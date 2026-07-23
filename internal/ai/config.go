// Package ai is the OPTIONAL, feature-gated AI assistant capability for the
// bullet-commerce store. It is off by default and only becomes reachable when an
// ANTHROPIC_API_KEY is present AND FEATURE_AI_ASSISTANT=true - see Config.Active.
//
// The package follows the same ports & adapters shape as internal/payment and
// internal/shipping: one port per external capability (LLMProvider), a thin
// adapter over the vendor SDK (claudeProvider over anthropic-sdk-go), and an
// agent that only knows the port. Read-only tools reuse the existing domain
// packages (search, variants, orders) through narrow interfaces.
//
// Scope of this foundation: gated provider + tool-use loop with read-only tools.
// TODO(v2): semantic RAG retrieval, evals harness, observability/tracing, and
// state-mutating tools (add_to_cart, initiate_return, create_pix_charge) are
// intentionally left out - the ports here are designed so those arrive as new
// adapters, not rewrites.
package ai

// Config carries the effective AI-assistant settings. WHY injected (not read
// from internal/config here): this package must not depend on the app's config
// loader - the wiring layer resolves the env vars and hands values in, keeping
// ai/** free of infra coupling and trivially testable.
type Config struct {
	// Enabled mirrors the FEATURE_AI_ASSISTANT flag.
	Enabled bool
	// APIKey is the ANTHROPIC_API_KEY. Never hardcode a secret; it arrives by
	// injection from the environment at wiring time.
	APIKey string
	// ModelDefault is the high-volume default (AI_MODEL_DEFAULT, e.g.
	// claude-haiku-4-5). Haiku 4.5 does NOT accept the effort/thinking params.
	ModelDefault string
	// ModelHard is the escalation tier (AI_MODEL_HARD, e.g. claude-sonnet-5)
	// used for hard intents; only this tier sends effort/adaptive thinking.
	ModelHard string
}

// Active reports whether the assistant may serve requests. This is the single
// gate every entry point consults: the feature flag must be on AND a key must
// be configured. Kept as one method so the handler and wiring agree on the rule.
func (c Config) Active() bool {
	return c.Enabled && c.APIKey != ""
}
