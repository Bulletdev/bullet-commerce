package handlers_test

import (
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/shipping"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newShippingRouter() *mux.Router {
	provider := shipping.NewTableProvider("01000000", shipping.DefaultBrazilRules())
	h := handlers.NewShippingHandler(provider)
	r := mux.NewRouter()
	r.HandleFunc("/api/shipping/calculate", h.Calculate).Methods(http.MethodPost)
	return r
}

func postShipping(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/shipping/calculate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	newShippingRouter().ServeHTTP(rr, req)
	return rr
}

// Given a valid CEP, When calculating, Then 200 with cost_cents/estimated_days.
func TestShippingHandler_Calculate_OK(t *testing.T) {
	rr := postShipping(t, `{"dest_cep":"01310100","weight_grams":500,"subtotal_cents":9990}`)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"cost_cents":1500`)
	assert.Contains(t, rr.Body.String(), `"estimated_days":3`)
	assert.Contains(t, rr.Body.String(), `"method":"table-sudeste"`)
}

// Given a malformed CEP, When calculating, Then 400.
func TestShippingHandler_Calculate_InvalidCEP(t *testing.T) {
	rr := postShipping(t, `{"dest_cep":"123"}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid CEP format")
}

// Given a well-formed CEP outside every rule, When calculating, Then 422.
func TestShippingHandler_Calculate_Unavailable(t *testing.T) {
	rr := postShipping(t, `{"dest_cep":"00000000"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "destination unavailable")
}

func TestShippingHandler_Calculate_BadJSON(t *testing.T) {
	rr := postShipping(t, `{"dest_cep":}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}
