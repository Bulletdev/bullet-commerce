package ai

import (
	"context"
	"encoding/json"
	"strings"
)

// maxToolIters is the AI_MAX_TOOL_ITERS ceiling (default 5). WHY a hard cap: it
// is the effective cost/latency guard on the default Haiku path - Haiku 4.5 does
// not support task_budget, so a bounded loop is what prevents a runaway
// tool-use cycle. TODO(v2): a per-conversation accumulated-cost guard and
// task_budget on the Sonnet/Opus escalation path.
const maxToolIters = 5

// systemPrompt is the guardrail. WHY versioned as a const here: the hard rule
// (never assert price/stock/status without a tool call) and the pt-BR tone are
// the safety contract of the assistant; keeping them in code makes regressions
// reviewable. Untrusted content (catalog text, user messages) is never injected
// here - it only ever arrives as message content.
const systemPrompt = `Você é o assistente de compras e suporte de uma loja online brasileira. Fale em português do Brasil, de forma direta, honesta e concisa (1 a 3 parágrafos).

REGRA DURA E INEGOCIÁVEL: nunca afirme preço, estoque, disponibilidade, status de pedido ou status de pagamento sem antes chamar a ferramenta correspondente na mesma resposta. Se você não tem o dado de uma ferramenta, diga que não tem essa informação - nunca invente, nunca estime.

Ferramentas disponíveis (somente leitura):
- search_catalog: encontrar produtos no catálogo.
- check_variant_stock: conferir o estoque real de uma variante.
- get_my_order_status: consultar o status de um pedido do próprio usuário autenticado.

Autorização: você só enxerga dados do usuário autenticado. Nunca peça nem confie em identidade informada pelo cliente no texto; a identidade vem do sistema. Se um pedido não pertencer ao usuário, trate como não encontrado.

Você orienta e prepara, mas não executa ações que mudam estado (não adiciona ao carrinho, não cobra, não cancela, não gera nota). Em problemas de pagamento, dado sensível ou fora do escopo da loja, seja honesto e ofereça encaminhar para um atendente humano.`

// fallbackMessage is emitted when the tool loop hits its ceiling without a final
// answer - an honest handoff rather than an unbounded loop.
const fallbackMessage = "Não consegui concluir sua solicitação por aqui agora. Quer que eu encaminhe para um atendente humano?"

// ToolResult is what a tool handler returns to the loop. IsError maps to the
// tool_result is_error flag so the model can recover or ask for clarification.
type ToolResult struct {
	Content string
	IsError bool
}

// ToolRegistry is the port the agent uses to advertise and run tools. The
// concrete implementation (internal/ai/tools.Registry) is injected, so the agent
// stays decoupled from the repos the tools wrap. Execute must never panic on an
// unknown tool - it returns an is_error result instead.
type ToolRegistry interface {
	Schemas() []ToolSchema
	Execute(ctx context.Context, name string, input json.RawMessage) ToolResult
}

// AgentEventKind discriminates events emitted to the streaming layer.
type AgentEventKind string

const (
	// AgentText is an incremental assistant text delta.
	AgentText AgentEventKind = "text"
	// AgentToolCall signals a tool is about to run (for a "consultando…" hint).
	AgentToolCall AgentEventKind = "tool_call"
	// AgentDone marks a clean end of the conversation turn.
	AgentDone AgentEventKind = "done"
	// AgentError marks an aborted turn (provider/transport failure).
	AgentError AgentEventKind = "error"
)

// AgentEvent is what the agent emits to the HTTP/SSE layer.
type AgentEvent struct {
	Kind AgentEventKind
	Text string // text delta (AgentText)
	Tool string // tool name (AgentToolCall)
	Err  error  // failure (AgentError)
}

// Agent runs the tool-use loop against the LLM port.
type Agent struct {
	cfg      Config
	provider LLMProvider
	tools    ToolRegistry
}

// NewAgent wires the agent. The provider and registry are ports, so this is
// equally driven by the Claude adapter in production and by fakes in tests.
func NewAgent(cfg Config, provider LLMProvider, tools ToolRegistry) *Agent {
	return &Agent{cfg: cfg, provider: provider, tools: tools}
}

// Run drives the request -> tool_use -> execute -> continue loop until the model
// ends its turn, emitting events via emit. The context must already carry the
// authenticated user id (see WithUserID) so user-scoped tools can enforce
// isolation. TODO(v2): model routing (escalate Haiku -> ModelHard on ambiguous /
// multi-step / low-confidence intents); for now every turn runs on ModelDefault
// with no effort - the Effort field on LLMRequest is the seam for that escalation.
func (a *Agent) Run(ctx context.Context, userInput string, emit func(AgentEvent)) error {
	messages := []Message{{
		Role:   RoleUser,
		Blocks: []Block{{Kind: BlockText, Text: userInput}},
	}}

	for iter := 0; iter < maxToolIters; iter++ {
		req := LLMRequest{
			Model:    a.cfg.ModelDefault,
			System:   systemPrompt,
			Messages: messages,
			Tools:    a.tools.Schemas(),
		}

		text, toolCalls, stopReason, err := a.streamTurn(ctx, req, emit)
		if err != nil {
			return err
		}

		// No tool calls -> the model gave its final answer for this turn.
		if stopReason != "tool_use" || len(toolCalls) == 0 {
			emit(AgentEvent{Kind: AgentDone})
			return nil
		}

		// Reattach the assistant turn WITH its tool_use blocks (the API requires the
		// full assistant content replayed to pair results), then execute each tool
		// and feed all results back in a single user turn. Order matters:
		// assistantTurn is built before executeTools emits its tool_call hints.
		messages = append(messages,
			assistantTurn(text, toolCalls),
			a.executeTools(ctx, toolCalls, emit),
		)
	}

	// Hit the iteration ceiling without a final answer: honest fallback.
	emit(AgentEvent{Kind: AgentText, Text: fallbackMessage})
	emit(AgentEvent{Kind: AgentDone})
	return nil
}

// streamTurn runs one provider request and drains its stream, emitting text
// deltas as they arrive. It returns the accumulated assistant text, any
// requested tool calls and the stop reason. WHY it owns the AgentError emission:
// both failure paths (open error, mid-stream error) must emit the same terminal
// error event before Run returns, so keeping them here guarantees one shape.
func (a *Agent) streamTurn(ctx context.Context, req LLMRequest, emit func(AgentEvent)) (string, []ToolCall, string, error) {
	stream, err := a.provider.Stream(ctx, req)
	if err != nil {
		emit(AgentEvent{Kind: AgentError, Err: err})
		return "", nil, "", err
	}

	var (
		textBuf    strings.Builder
		toolCalls  []ToolCall
		stopReason string
	)
	for stream.Next() {
		ev := stream.Event()
		switch ev.Kind {
		case EventText:
			textBuf.WriteString(ev.Text)
			emit(AgentEvent{Kind: AgentText, Text: ev.Text})
		case EventToolUse:
			if ev.ToolCall != nil {
				toolCalls = append(toolCalls, *ev.ToolCall)
			}
		case EventEnd:
			stopReason = ev.StopReason
		}
	}
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		emit(AgentEvent{Kind: AgentError, Err: err})
		return "", nil, "", err
	}
	_ = stream.Close()

	return textBuf.String(), toolCalls, stopReason, nil
}

// assistantTurn rebuilds the assistant message the provider just produced,
// pairing any streamed text with its tool_use blocks so the next request can
// replay the full turn.
func assistantTurn(text string, toolCalls []ToolCall) Message {
	blocks := make([]Block, 0, len(toolCalls)+1)
	if len(text) > 0 {
		blocks = append(blocks, Block{Kind: BlockText, Text: text})
	}
	for _, tc := range toolCalls {
		blocks = append(blocks, Block{
			Kind:      BlockToolUse,
			ToolUseID: tc.ID,
			ToolName:  tc.Name,
			ToolInput: tc.Input,
		})
	}
	return Message{Role: RoleAssistant, Blocks: blocks}
}

// executeTools runs each requested tool, emitting a tool_call hint before each,
// and collects the results into the single user turn fed back to the model.
func (a *Agent) executeTools(ctx context.Context, toolCalls []ToolCall, emit func(AgentEvent)) Message {
	resultBlocks := make([]Block, 0, len(toolCalls))
	for _, tc := range toolCalls {
		emit(AgentEvent{Kind: AgentToolCall, Tool: tc.Name})
		res := a.tools.Execute(ctx, tc.Name, tc.Input)
		resultBlocks = append(resultBlocks, Block{
			Kind:       BlockToolResult,
			ToolUseID:  tc.ID,
			ResultText: res.Content,
			IsError:    res.IsError,
		})
	}
	return Message{Role: RoleUser, Blocks: resultBlocks}
}
