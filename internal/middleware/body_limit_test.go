package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBodyLimit_UnderLimit(t *testing.T) {
	handler := BodyLimit(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "hello", string(body))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestBodyLimit_OverLimit(t *testing.T) {
	handler := BodyLimit(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		assert.Error(t, err, "reading beyond limit should fail")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	}))

	big := bytes.Repeat([]byte("x"), 20)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(big))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}
