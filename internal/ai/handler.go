package ai

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bullet-commerce/internal/auth"

	"github.com/google/uuid"
)

// ChatHandler serves POST /api/assistant/chat as an SSE stream. It is GATED: if
// the feature is not Active (flag off or no key) it 404s, so the endpoint is
// invisible when disabled. This handler is NOT self-registering - the route is
// wired in cmd/main.go by the composition layer.
type ChatHandler struct {
	cfg   Config
	agent *Agent
}

// NewChatHandler builds the handler from the config, the LLM provider port, and
// the tool registry port. The registry is constructed by the caller (with the
// real repos) and injected here.
func NewChatHandler(cfg Config, provider LLMProvider, tools ToolRegistry) *ChatHandler {
	return &ChatHandler{cfg: cfg, agent: NewAgent(cfg, provider, tools)}
}

type chatRequest struct {
	Message string `json:"message"`
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Gate first: when inactive the endpoint does not exist. 404 (not 403) avoids
	// confirming the feature to unauthorized probes.
	if !h.cfg.Active() {
		http.NotFound(w, r)
		return
	}

	// Identity comes from the validated JWT (injected by auth middleware), never
	// from the request body - the assistant's user scoping depends on this.
	userID, ok := r.Context().Value(auth.UserIDContextKey).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Scope the context to the authenticated user for the tool layer.
	ctx := WithUserID(r.Context(), userID)

	_ = h.agent.Run(ctx, req.Message, func(ev AgentEvent) {
		writeSSE(w, ev)
		flusher.Flush()
	})
}

// writeSSE serializes an agent event as a named SSE event with a JSON payload.
func writeSSE(w http.ResponseWriter, ev AgentEvent) {
	payload := map[string]any{}
	switch ev.Kind {
	case AgentText:
		payload["text"] = ev.Text
	case AgentToolCall:
		payload["tool"] = ev.Tool
	case AgentError:
		payload["error"] = ev.Err.Error()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, data)
}
