package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestID_Generated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(RequestIDKey)
		assert.NotEmpty(t, id)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.NotEmpty(t, rr.Header().Get("X-Request-ID"))
}

func TestRequestID_Propagated(t *testing.T) {
	const existing = "my-trace-id-123"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(RequestIDKey).(string)
		assert.Equal(t, existing, id)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existing)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, existing, rr.Header().Get("X-Request-ID"))
}
