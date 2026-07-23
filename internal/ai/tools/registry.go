// Package tools holds the read-only tool handlers the AI assistant may call.
// Each handler wraps an existing domain package (search, variants, orders)
// through a NARROW interface - never the concrete repository constructor - so
// the tool layer stays decoupled and easy to fake in tests.
//
// The registry enforces a strict allowlist: only the three registered tools can
// run, and an unknown tool name is rejected as an is_error result rather than
// executed. User-scoped tools derive the user id from the request context
// (injected from the JWT), never from model-supplied input.
//
// TODO(v2): state-mutating tools (add_to_cart, initiate_return, create_pix_charge)
// gated behind human confirmation - they arrive as new handlers, not a rewrite.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"bullet-commerce/internal/ai"
)

// Handler binds a tool schema (advertised to the model) to its executor.
type Handler struct {
	Schema  ai.ToolSchema
	Execute func(ctx context.Context, input json.RawMessage) ai.ToolResult
}

// Registry is the allowlist of callable tools. Only names registered here can
// execute; the map lookup in Execute is the allowlist enforcement point.
type Registry struct {
	handlers map[string]Handler
	// order preserves a stable schema ordering so the cached tool prefix (and
	// thus the prompt cache) does not churn between requests.
	order []string
}

// Compile-time assertion that Registry satisfies the agent's port.
var _ ai.ToolRegistry = (*Registry)(nil)

// NewRegistry wires the read-only tools over narrow repo interfaces.
func NewRegistry(catalog CatalogSearcher, variants VariantReader, orders OrderReader) *Registry {
	r := &Registry{handlers: map[string]Handler{}}
	r.register(searchCatalogTool(catalog))
	r.register(checkVariantStockTool(variants))
	r.register(getMyOrderStatusTool(orders))
	return r
}

func (r *Registry) register(h Handler) {
	r.handlers[h.Schema.Name] = h
	r.order = append(r.order, h.Schema.Name)
}

// Schemas returns the advertised tool schemas in a stable order.
func (r *Registry) Schemas() []ai.ToolSchema {
	out := make([]ai.ToolSchema, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.handlers[name].Schema)
	}
	return out
}

// Execute runs a tool by name. An unregistered name is rejected here (the
// allowlist) as an is_error result - it never runs and never panics.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) ai.ToolResult {
	h, ok := r.handlers[name]
	if !ok {
		return ai.ToolResult{
			Content: fmt.Sprintf("ferramenta desconhecida: %q", name),
			IsError: true,
		}
	}
	return h.Execute(ctx, input)
}

// errResult is a small helper for handler-level failures.
func errResult(msg string) ai.ToolResult {
	return ai.ToolResult{Content: msg, IsError: true}
}

// okJSON marshals a successful tool payload; marshalling never realistically
// fails for these flat maps, but we degrade gracefully if it does.
func okJSON(v any) ai.ToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return errResult("erro ao serializar resultado")
	}
	return ai.ToolResult{Content: string(data)}
}
