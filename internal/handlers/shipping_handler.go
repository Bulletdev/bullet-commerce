package handlers

import (
	"bullet-commerce/internal/shipping"
	"bullet-commerce/internal/webutils"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// ShippingHandler exposes freight quoting over the shipping.Provider domain seam.
type ShippingHandler struct {
	Provider shipping.Provider
}

func NewShippingHandler(provider shipping.Provider) *ShippingHandler {
	return &ShippingHandler{Provider: provider}
}

type CalculateShippingRequest struct {
	DestCEP       string `json:"dest_cep"`
	WeightGrams   int    `json:"weight_grams,omitempty"`
	SubtotalCents int64  `json:"subtotal_cents,omitempty"`
}

type CalculateShippingResponse struct {
	CostCents     int64  `json:"cost_cents"`
	EstimatedDays int    `json:"estimated_days"`
	Method        string `json:"method"`
}

// Calculate handles POST /api/shipping/calculate (public). It maps domain errors to
// transport: malformed CEP -> 400, out-of-range CEP -> 422.
func (h *ShippingHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	var req CalculateShippingRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	quote, err := h.Provider.Quote(r.Context(), shipping.QuoteRequest{
		DestCEP:          req.DestCEP,
		TotalWeightGrams: req.WeightGrams,
		SubtotalCents:    req.SubtotalCents,
	})
	if err != nil {
		switch {
		case errors.Is(err, shipping.ErrInvalidCEP):
			webutils.ErrorJSON(w, errors.New("invalid CEP format"), http.StatusBadRequest)
		case errors.Is(err, shipping.ErrDestinationUnavailable):
			webutils.ErrorJSON(w, errors.New("destination unavailable"), http.StatusUnprocessableEntity)
		default:
			webutils.ErrorJSON(w, errors.New("failed to calculate shipping"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, CalculateShippingResponse{
		CostCents:     quote.CostCents,
		EstimatedDays: quote.EstimatedDays,
		Method:        quote.Method,
	})
}

var (
	cepRegex  = regexp.MustCompile(`^\d{8}$`)
	cepClient = &http.Client{Timeout: 5 * time.Second}
)

type CepResponse struct {
	Street       string `json:"street"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
}

// LookupCep handles GET /api/shipping/cep/{cep}
// Proxies ViaCEP so the frontend never calls external services directly.
func LookupCep(w http.ResponseWriter, r *http.Request) {
	raw := strings.ReplaceAll(mux.Vars(r)["cep"], "-", "")
	if !cepRegex.MatchString(raw) {
		webutils.ErrorJSON(w, errors.New("invalid CEP format"), http.StatusBadRequest)
		return
	}

	// raw is validated by cepRegex above (digits only) and the host is the fixed ViaCEP
	// endpoint, so the concatenation cannot redirect the request elsewhere.
	resp, err := cepClient.Get("https://viacep.com.br/ws/" + raw + "/json/") //nolint:gosec // raw is cepRegex-validated; host is fixed
	if err != nil {
		webutils.ErrorJSON(w, errors.New("CEP service unavailable"), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var via struct {
		Erro       bool   `json:"erro"`
		Logradouro string `json:"logradouro"`
		Bairro     string `json:"bairro"`
		Localidade string `json:"localidade"`
		UF         string `json:"uf"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&via); err != nil || via.Erro {
		webutils.ErrorJSON(w, errors.New("CEP not found"), http.StatusNotFound)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, CepResponse{
		Street:       via.Logradouro,
		Neighborhood: via.Bairro,
		City:         via.Localidade,
		State:        via.UF,
	})
}
