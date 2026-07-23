package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bullet-commerce/internal/auth"

	"github.com/google/uuid"
)

func newRequest(userID *uuid.UUID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/chat", strings.NewReader(body))
	if userID != nil {
		ctx := context.WithValue(req.Context(), auth.UserIDContextKey, *userID)
		req = req.WithContext(ctx)
	}
	return req
}

// When inactive (flag off / no key) the endpoint must be invisible (404).
func TestChatHandlerGatedReturns404WhenInactive(t *testing.T) {
	h := NewChatHandler(Config{}, &fakeProvider{}, &fakeRegistry{})
	uid := uuid.New()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newRequest(&uid, `{"message":"oi"}`))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when inactive, got %d", rr.Code)
	}
}

// Active but no authenticated user -> 401.
func TestChatHandlerRequiresAuth(t *testing.T) {
	h := NewChatHandler(Config{Enabled: true, APIKey: "sk-test"}, &fakeProvider{}, &fakeRegistry{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newRequest(nil, `{"message":"oi"}`))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without user, got %d", rr.Code)
	}
}

// Active + authenticated -> streams SSE with the assistant's text.
func TestChatHandlerStreamsSSE(t *testing.T) {
	provider := &fakeProvider{turns: []scriptedTurn{{text: "Olá!"}}}
	h := NewChatHandler(Config{Enabled: true, APIKey: "sk-test", ModelDefault: "claude-haiku-4-5"}, provider, &fakeRegistry{})

	uid := uuid.New()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newRequest(&uid, `{"message":"oi"}`))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: text") || !strings.Contains(body, "Olá!") {
		t.Fatalf("expected SSE text event with content, got %q", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("expected a done event, got %q", body)
	}
}

// Malformed body -> 400.
func TestChatHandlerBadRequest(t *testing.T) {
	h := NewChatHandler(Config{Enabled: true, APIKey: "sk-test"}, &fakeProvider{}, &fakeRegistry{})
	uid := uuid.New()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newRequest(&uid, `{}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty message, got %d", rr.Code)
	}
}
