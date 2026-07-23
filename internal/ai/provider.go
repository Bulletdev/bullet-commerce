package ai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// maxOutputTokens caps a single assistant reply. WHY modest: assistant answers
// are concise (1–3 paragraphs, per the PRD tone), and a small cap keeps latency
// and cost predictable on the high-volume Haiku path.
const maxOutputTokens = 1024

// ErrMissingAPIKey is returned by the constructor when no key is configured, so
// the failure surfaces at init/wiring time rather than on the first request.
var ErrMissingAPIKey = errors.New("ai: ANTHROPIC_API_KEY is required to build the Claude provider")

// --- Port & domain types (no SDK types leak across this boundary) ---

// Role is the author of a conversation turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockKind discriminates the content blocks inside a Message.
type BlockKind string

const (
	BlockText       BlockKind = "text"
	BlockToolUse    BlockKind = "tool_use"
	BlockToolResult BlockKind = "tool_result"
)

// Block is one heterogeneous piece of a turn: assistant text, a model tool call,
// or a tool result fed back on the next user turn. Modelling all three lets the
// agent replay the exact assistant content (preserving tool_use) that a correct
// tool-use loop requires.
type Block struct {
	Kind BlockKind

	// Text is set for BlockText.
	Text string

	// ToolUseID is the tool-call id: assigned by the model on BlockToolUse and
	// echoed by BlockToolResult so the API can pair result to call.
	ToolUseID string
	// ToolName / ToolInput are set for BlockToolUse.
	ToolName  string
	ToolInput json.RawMessage

	// ResultText / IsError are set for BlockToolResult.
	ResultText string
	IsError    bool
}

// Message is a single conversational turn.
type Message struct {
	Role   Role
	Blocks []Block
}

// ToolSchema advertises a callable tool to the model. Properties/Required are
// the JSON-schema pieces; the adapter enforces additionalProperties:false and
// strict validation so tool inputs are guaranteed to match.
type ToolSchema struct {
	Name        string
	Description string
	Properties  map[string]any
	Required    []string
}

// Effort levels for the hard tier only. The default Haiku path sends none.
const (
	EffortLow    = "low"
	EffortMedium = "medium"
)

// LLMRequest is the provider-agnostic request. Effort == "" means the default
// tier (Haiku): no effort/thinking is sent - Haiku 4.5 rejects effort with a 400,
// so omitting it on the highest-volume path is deliberate.
type LLMRequest struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolSchema
	Effort   string
}

// Usage reports token accounting for one turn, including cache hits/writes.
type Usage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
}

// ToolCall is a fully-assembled model tool invocation surfaced by the stream.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// EventKind discriminates a StreamEvent.
type EventKind string

const (
	EventText    EventKind = "text"     // Text carries an incremental delta.
	EventToolUse EventKind = "tool_use" // ToolCall is a complete tool invocation.
	EventEnd     EventKind = "end"      // StopReason + Usage finalize the turn.
)

// StreamEvent is one item from an LLMStream.
type StreamEvent struct {
	Kind       EventKind
	Text       string
	ToolCall   *ToolCall
	StopReason string
	Usage      Usage
}

// LLMStream is a pull iterator over a single streamed turn. Text deltas arrive
// live for SSE UX; completed tool_use blocks and the terminal end event arrive
// after the model finishes the turn.
type LLMStream interface {
	Next() bool
	Event() StreamEvent
	Err() error
	Close() error
}

// LLMProvider is the port between the agent and the LLM. Mirrors payment.Provider
// / shipping.Provider: the agent never sees the vendor SDK.
type LLMProvider interface {
	Stream(ctx context.Context, req LLMRequest) (LLMStream, error)
}

// --- Claude adapter over anthropic-sdk-go ---

type claudeProvider struct {
	client anthropic.Client
}

// Compile-time assertion that the adapter satisfies the port.
var _ LLMProvider = (*claudeProvider)(nil)

// NewClaudeProvider builds the Claude-backed provider. It fails fast when no key
// is configured (init-time, not request-time). No network call is made here - a
// missing key never reaches the API, and building/testing needs no key because
// the fakeProvider is used in tests.
func NewClaudeProvider(cfg Config) (LLMProvider, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	return &claudeProvider{
		client: anthropic.NewClient(option.WithAPIKey(cfg.APIKey)),
	}, nil
}

func (p *claudeProvider) Stream(ctx context.Context, req LLMRequest) (LLMStream, error) {
	params := buildParams(req)
	ctx, cancel := context.WithCancel(ctx)
	sdkStream := p.client.Messages.NewStreaming(ctx, params)
	s := &claudeStream{ch: make(chan StreamEvent), cancel: cancel}
	go s.pump(ctx, sdkStream)
	return s, nil
}

// buildParams translates the domain request into SDK params. Prompt caching:
// one cache_control breakpoint on the system block. WHY there: render order is
// tools -> system -> messages, so a breakpoint on system caches the stable
// prefix (tool schemas + system prompt) together - the meta the PRD leans on for
// cost (M7) and TTFT. Thinking/effort are sent ONLY when req.Effort is set (hard
// tier); the default Haiku path omits both to avoid a 400.
func buildParams(req LLMRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxOutputTokens,
		Messages:  toSDKMessages(req.Messages),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{
			Text:         req.System,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}}
	}

	if len(req.Tools) > 0 {
		params.Tools = toSDKTools(req.Tools)
	}

	if req.Effort != "" {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(req.Effort),
		}
	}

	return params
}

func toSDKMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			switch b.Kind {
			case BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case BlockToolUse:
				var input any
				if len(b.ToolInput) > 0 {
					input = json.RawMessage(b.ToolInput)
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ToolUseID, input, b.ToolName))
			case BlockToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.ResultText, b.IsError))
			}
		}
		if m.Role == RoleAssistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		} else {
			out = append(out, anthropic.NewUserMessage(blocks...))
		}
	}
	return out
}

func toSDKTools(tools []ToolSchema) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		tp := anthropic.ToolParam{
			Name: t.Name,
			// strict + additionalProperties:false so tool_use.input is guaranteed
			// to validate against the schema (no surprise fields).
			Strict: anthropic.Bool(true),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties:  t.Properties,
				Required:    t.Required,
				ExtraFields: map[string]any{"additionalProperties": false},
			},
		}
		if t.Description != "" {
			tp.Description = anthropic.String(t.Description)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return out
}

func toUsage(u anthropic.Usage) Usage {
	return Usage{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
	}
}

// claudeStream adapts the SDK's SSE stream to the LLMStream port. A goroutine
// pumps SDK events into a channel: text deltas pass through live, and the
// accumulated message's tool_use blocks + a terminal end event are emitted once
// the turn completes. WHY a goroutine+channel: it lets one SDK event fan out
// into several domain events and defers tool_use emission to end-of-turn cleanly.
type claudeStream struct {
	ch     chan StreamEvent
	cancel context.CancelFunc
	cur    StreamEvent
	err    error
}

func (s *claudeStream) pump(ctx context.Context, sdk *ssestream.Stream[anthropic.MessageStreamEventUnion]) {
	// Order matters: set err before closing ch. A receiver only observes err
	// after Next() sees the closed channel, so the close provides the
	// happens-before for a race-free read.
	defer close(s.ch)
	defer sdk.Close()

	var acc anthropic.Message
	for sdk.Next() {
		ev := sdk.Current()
		if err := acc.Accumulate(ev); err != nil {
			s.err = err
			return
		}
		if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			if !s.send(ctx, StreamEvent{Kind: EventText, Text: ev.Delta.Text}) {
				return
			}
		}
	}
	if err := sdk.Err(); err != nil {
		s.err = err
		return
	}

	for _, blk := range acc.Content {
		if blk.Type == "tool_use" {
			tc := ToolCall{ID: blk.ID, Name: blk.Name, Input: json.RawMessage(blk.Input)}
			if !s.send(ctx, StreamEvent{Kind: EventToolUse, ToolCall: &tc}) {
				return
			}
		}
	}
	s.send(ctx, StreamEvent{Kind: EventEnd, StopReason: string(acc.StopReason), Usage: toUsage(acc.Usage)})
}

// send delivers one event, bailing out if the consumer cancelled (Close) so the
// pump goroutine never leaks blocked on a send nobody reads.
func (s *claudeStream) send(ctx context.Context, e StreamEvent) bool {
	select {
	case s.ch <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *claudeStream) Next() bool {
	ev, ok := <-s.ch
	if !ok {
		return false
	}
	s.cur = ev
	return true
}

func (s *claudeStream) Event() StreamEvent { return s.cur }
func (s *claudeStream) Err() error         { return s.err }

func (s *claudeStream) Close() error {
	s.cancel()
	return nil
}
