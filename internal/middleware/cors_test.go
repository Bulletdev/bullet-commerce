package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func next(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func TestCORS_AllowedOrigin(t *testing.T) {
	handler := CORS("http://localhost:8888,http://localhost:3000")(http.HandlerFunc(next))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:8888")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, "http://localhost:8888", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	handler := CORS("http://localhost:8888")(http.HandlerFunc(next))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_Preflight(t *testing.T) {
	handler := CORS("http://localhost:8888")(http.HandlerFunc(next))

	req := httptest.NewRequest(http.MethodOptions, "/api/products", nil)
	req.Header.Set("Origin", "http://localhost:8888")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Headers"), "Authorization")
}

func TestCORS_NoOriginHeader(t *testing.T) {
	handler := CORS("http://localhost:8888")(http.HandlerFunc(next))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rr.Code)
}
