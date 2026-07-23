package webutils

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), `"key":"value"`)
}

func TestErrorJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	ErrorJSON(rr, errors.New("something went wrong"), http.StatusBadRequest)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "something went wrong")
}

func TestRawJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	RawJSON(rr, http.StatusOK, []byte(`{"cached":true}`))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Equal(t, `{"cached":true}`, rr.Body.String())
}

func TestReadJSON_Valid(t *testing.T) {
	body := strings.NewReader(`{"name":"go-cart","price":99.9}`)
	req, _ := http.NewRequest(http.MethodPost, "/", body)

	var dst struct {
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}
	require.NoError(t, ReadJSON(req, &dst))
	assert.Equal(t, "go-cart", dst.Name)
	assert.Equal(t, 99.9, dst.Price)
}

func TestReadJSON_Invalid(t *testing.T) {
	body := strings.NewReader(`{invalid json}`)
	req, _ := http.NewRequest(http.MethodPost, "/", body)

	var dst map[string]any
	assert.Error(t, ReadJSON(req, &dst))
}
