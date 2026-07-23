package handlers

import (
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/webutils"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// WebhookHandler receives PSP payment callbacks. It lives OUTSIDE the authenticated
// subrouter: the PSP has no JWT, so authenticity is proven by the provider's signature
// over the raw body, not by the auth middleware.
type WebhookHandler struct {
	OrderRepo       orders.OrderRepository
	PaymentRegistry *payment.Registry
	PaymentProvider payment.Name
}

func NewWebhookHandler(orderRepo orders.OrderRepository, paymentRegistry *payment.Registry, paymentProvider payment.Name) *WebhookHandler {
	return &WebhookHandler{
		OrderRepo:       orderRepo,
		PaymentRegistry: paymentRegistry,
		PaymentProvider: paymentProvider,
	}
}

// HandlePayment verifies the raw webhook body against the provider signature and, on a
// paid event, confirms the referenced order. Response contract:
//   - invalid/failed signature verification -> 400 (never process an unverified body)
//   - EventUnknown / not-yet-handled kinds   -> 200 no-op
//   - paid -> ConfirmOrderPayment; an already-paid/unknown order is a 200 no-op (idempotent),
//     a transient error is 500 so the PSP retries.
func (h *WebhookHandler) HandlePayment(w http.ResponseWriter, r *http.Request) {
	provider, err := h.PaymentRegistry.Get(h.PaymentProvider)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("payment provider not available"), http.StatusServiceUnavailable)
		return
	}

	// Read the body raw: the signature is verified over the exact bytes before any parse.
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to read request body"), http.StatusBadRequest)
		return
	}

	evt, err := provider.VerifyWebhook(r.Context(), payment.RawWebhook{
		Headers: flattenHeaders(r.Header),
		Body:    raw,
		Query:   flattenQuery(r),
	})
	if err != nil {
		// Invalid signature or an undecodable body: refuse without processing.
		webutils.ErrorJSON(w, errors.New("invalid webhook"), http.StatusBadRequest)
		return
	}

	if evt.Type != payment.EventChargePaid {
		// Unknown/expired/refunded are not handled yet — acknowledge so the PSP stops retrying.
		w.WriteHeader(http.StatusOK)
		return
	}

	orderID, err := uuid.Parse(evt.ReferenceID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order reference"), http.StatusBadRequest)
		return
	}

	if err := h.OrderRepo.ConfirmOrderPayment(r.Context(), orderID); err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			// Already paid, cancelled, or unknown — a duplicate/late webhook is a no-op.
			w.WriteHeader(http.StatusOK)
			return
		}
		webutils.ErrorJSON(w, errors.New("failed to confirm payment"), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// flattenHeaders collapses the multi-valued http.Header to the single-value map the
// provider contract expects (headerValue lookups are case-insensitive).
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k := range h {
		out[k] = h.Get(k)
	}
	return out
}

// flattenQuery collapses the URL query to a single-value map for signature schemes that
// carry the digest as a query parameter.
func flattenQuery(r *http.Request) map[string]string {
	q := r.URL.Query()
	out := make(map[string]string, len(q))
	for k := range q {
		out[k] = q.Get(k)
	}
	return out
}
