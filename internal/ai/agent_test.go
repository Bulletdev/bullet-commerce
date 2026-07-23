package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeRegistry is an in-memory ToolRegistry recording calls and returning
// scripted results.
type fakeRegistry struct {
	schemas []ToolSchema
	results map[string]ToolResult
	calls   []toolCall
}

type toolCall struct {
	name  string
	input json.RawMessage
}

func (r *fakeRegistry) Schemas() []ToolSchema { return r.schemas }

func (r *fakeRegistry) Execute(_ context.Context, name string, input json.RawMessage) ToolResult {
	r.calls = append(r.calls, toolCall{name: name, input: input})
	if res, ok := r.results[name]; ok {
		return res
	}
	return ToolResult{Content: "sem handler", IsError: true}
}

func collect(t *testing.T, provider LLMProvider, reg ToolRegistry, input string) []AgentEvent {
	t.Helper()
	agent := NewAgent(Config{ModelDefault: "claude-haiku-4-5"}, provider, reg)
	var events []AgentEvent
	if err := agent.Run(context.Background(), input, func(ev AgentEvent) {
		events = append(events, ev)
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return events
}

func joinText(events []AgentEvent) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Kind == AgentText {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

// The loop calls the right tool, feeds the result back, and produces a grounded
// final answer - without inventing anything.
func TestAgentToolLoopGrounded(t *testing.T) {
	provider := &fakeProvider{turns: []scriptedTurn{
		{toolCalls: []ToolCall{{ID: "t1", Name: "check_variant_stock", Input: rawInput(map[string]any{"variant_id": "v-1"})}}},
		{text: "Temos essa variante em estoque."},
	}}
	reg := &fakeRegistry{results: map[string]ToolResult{
		"check_variant_stock": {Content: `{"available":3,"in_stock":true}`},
	}}

	events := collect(t, provider, reg, "tem em estoque?")

	if len(reg.calls) != 1 || reg.calls[0].name != "check_variant_stock" {
		t.Fatalf("expected one check_variant_stock call, got %+v", reg.calls)
	}
	if got := joinText(events); !strings.Contains(got, "em estoque") {
		t.Fatalf("expected grounded final answer, got %q", got)
	}
	if events[len(events)-1].Kind != AgentDone {
		t.Fatal("expected the turn to end with AgentDone")
	}

	// The second request must replay the tool_result so the answer is grounded.
	second := provider.requests[1]
	last := second.Messages[len(second.Messages)-1]
	if last.Role != RoleUser || len(last.Blocks) != 1 || last.Blocks[0].Kind != BlockToolResult {
		t.Fatalf("expected the follow-up turn to carry a tool_result, got %+v", last)
	}
	if last.Blocks[0].ToolUseID != "t1" {
		t.Fatalf("tool_result must reference the tool_use id, got %q", last.Blocks[0].ToolUseID)
	}
}

// A turn with no tool calls ends immediately.
func TestAgentDirectAnswer(t *testing.T) {
	provider := &fakeProvider{turns: []scriptedTurn{
		{text: "Olá! Como posso ajudar?"},
	}}
	events := collect(t, provider, &fakeRegistry{}, "oi")
	if got := joinText(events); !strings.Contains(got, "ajudar") {
		t.Fatalf("unexpected answer: %q", got)
	}
}

// Hitting the tool-iteration ceiling ends with the honest fallback, not a loop.
func TestAgentMaxIterationsFallback(t *testing.T) {
	turns := make([]scriptedTurn, maxToolIters)
	for i := range turns {
		turns[i] = scriptedTurn{toolCalls: []ToolCall{{ID: "t", Name: "search_catalog", Input: rawInput(map[string]any{"query": "x"})}}}
	}
	provider := &fakeProvider{turns: turns}
	reg := &fakeRegistry{results: map[string]ToolResult{"search_catalog": {Content: `{"num_results":0}`}}}

	events := collect(t, provider, reg, "loop")
	if got := joinText(events); !strings.Contains(got, "atendente humano") {
		t.Fatalf("expected fallback handoff, got %q", got)
	}
	if len(reg.calls) != maxToolIters {
		t.Fatalf("expected %d tool calls, got %d", maxToolIters, len(reg.calls))
	}
}

// A provider/transport error aborts the turn with an AgentError.
func TestAgentProviderError(t *testing.T) {
	provider := &fakeProvider{turns: []scriptedTurn{{err: errors.New("boom")}}}
	agent := NewAgent(Config{ModelDefault: "claude-haiku-4-5"}, provider, &fakeRegistry{})

	var sawError bool
	err := agent.Run(context.Background(), "oi", func(ev AgentEvent) {
		if ev.Kind == AgentError {
			sawError = true
		}
	})
	if err == nil || !sawError {
		t.Fatalf("expected an error and an AgentError event, err=%v sawError=%v", err, sawError)
	}
}
