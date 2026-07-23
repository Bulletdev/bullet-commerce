package ai

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The default (Haiku) path must NOT send effort/thinking — Haiku 4.5 rejects
// effort with a 400, so this is a correctness guard on the highest-volume path.
func TestBuildParamsDefaultOmitsEffortAndThinking(t *testing.T) {
	params := buildParams(LLMRequest{
		Model:    "claude-haiku-4-5",
		System:   "guardrails",
		Messages: []Message{{Role: RoleUser, Blocks: []Block{{Kind: BlockText, Text: "oi"}}}},
	})

	if params.OutputConfig.Effort != "" {
		t.Fatalf("default path must not set effort, got %q", params.OutputConfig.Effort)
	}
	if params.Thinking.OfAdaptive != nil || params.Thinking.OfEnabled != nil {
		t.Fatal("default path must not set thinking")
	}
	// System prompt should carry a cache_control breakpoint (stable prefix).
	if len(params.System) != 1 || params.System[0].CacheControl.Type != "ephemeral" {
		t.Fatal("expected a cache_control breakpoint on the system block")
	}
}

// The hard tier sends adaptive thinking + effort.
func TestBuildParamsHardTierSendsEffort(t *testing.T) {
	params := buildParams(LLMRequest{
		Model:    "claude-sonnet-5",
		System:   "guardrails",
		Effort:   EffortMedium,
		Messages: []Message{{Role: RoleUser, Blocks: []Block{{Kind: BlockText, Text: "oi"}}}},
	})

	if params.OutputConfig.Effort != anthropic.OutputConfigEffortMedium {
		t.Fatalf("hard tier must set effort=medium, got %q", params.OutputConfig.Effort)
	}
	if params.Thinking.OfAdaptive == nil {
		t.Fatal("hard tier must set adaptive thinking")
	}
}

// Tool schemas must be advertised with strict validation.
func TestBuildParamsToolsAreStrict(t *testing.T) {
	params := buildParams(LLMRequest{
		Model: "claude-haiku-4-5",
		Tools: []ToolSchema{{
			Name:       "search_catalog",
			Properties: map[string]any{"query": map[string]any{"type": "string"}},
			Required:   []string{"query"},
		}},
	})
	if len(params.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(params.Tools))
	}
	tp := params.Tools[0].OfTool
	if tp == nil {
		t.Fatal("expected a custom tool param")
	}
	if tp.Strict.Or(false) != true {
		t.Fatal("tool must be strict")
	}
}
