package handlers

import (
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/webutils"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// RefundItemRequest flags one order line for refund. Restock (opt-in) returns that line's
// physical stock; omit it to refund money without touching inventory.
type RefundItemRequest struct {
	VariantID uuid.UUID `json:"variant_id"`
	Qty       int       `json:"qty"`
	Restock   bool      `json:"restock"`
}

// RefundOrderRequest is the admin refund body. With Items omitted it is a full refund of the
// remaining balance (no restock). AmountCents == 0 means "refund the full remaining balance".
type RefundOrderRequest struct {
	Items       []RefundItemRequest `json:"items"`
	AmountCents int64               `json:"amount_cents"`
}

// RefundOrder refunds a paid order (ADMIN ONLY - wired on the admin subrouter). It resolves
// the configured PSP and requires it to implement payment.Refunder; a provider without that
// capability yields 501. The financial refund, the payment_status flip and any per-item
// restock all happen in one transaction inside the order repository.
func (h *OrderHandler) RefundOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order ID format"), http.StatusBadRequest)
		return
	}

	req, ok := parseRefundRequest(w, r)
	if !ok {
		return
	}

	refunder, ok := h.resolveRefunder(w)
	if !ok {
		return
	}

	items, ok := buildRefundItems(w, req)
	if !ok {
		return
	}

	if err := h.OrderRepo.RefundOrder(r.Context(), refunder, orderID, items, req.AmountCents); err != nil {
		writeRefundError(w, orderID, err)
		return
	}

	// Return the refreshed order so the caller sees the new payment_status / refund totals.
	if order, refreshedItems, ferr := h.OrderRepo.FindOrderByID(r.Context(), orderID); ferr == nil {
		webutils.WriteJSON(w, http.StatusOK, OrderResponse{Order: *order, Items: refreshedItems})
		return
	}
	w.WriteHeader(http.StatusOK)
}

// parseRefundRequest decodes the admin refund body. An empty body is valid: it means a full refund.
// Only a malformed (non-empty) body is 400. Returns ok=false after having written the response.
func parseRefundRequest(w http.ResponseWriter, r *http.Request) (RefundOrderRequest, bool) {
	var req RefundOrderRequest
	if err := webutils.ReadJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return RefundOrderRequest{}, false
	}
	if req.AmountCents < 0 {
		webutils.ErrorJSON(w, errors.New("amount_cents cannot be negative"), http.StatusBadRequest)
		return RefundOrderRequest{}, false
	}
	return req, true
}

// resolveRefunder resolves the configured PSP and requires it to implement the optional
// payment.Refunder capability (reached by type assertion, never redefined). Returns ok=false after
// having written the response.
func (h *OrderHandler) resolveRefunder(w http.ResponseWriter) (payment.Refunder, bool) {
	provider, err := h.PaymentRegistry.Get(h.PaymentProvider)
	if err != nil {
		slog.Error("payment provider not configured", "provider", h.PaymentProvider, "error", err)
		webutils.ErrorJSON(w, errors.New("payment provider not available"), http.StatusServiceUnavailable)
		return nil, false
	}
	refunder, ok := provider.(payment.Refunder)
	if !ok {
		webutils.ErrorJSON(w, errors.New("payment provider does not support refunds"), http.StatusNotImplemented)
		return nil, false
	}
	return refunder, true
}

// buildRefundItems maps the request lines to repository refund items, rejecting a restocked line
// with a non-positive qty. Returns ok=false after having written the response.
func buildRefundItems(w http.ResponseWriter, req RefundOrderRequest) ([]orders.RefundItem, bool) {
	items := make([]orders.RefundItem, 0, len(req.Items))
	for _, it := range req.Items {
		if it.Restock && it.Qty <= 0 {
			webutils.ErrorJSON(w, errors.New("qty must be positive for a restocked item"), http.StatusBadRequest)
			return nil, false
		}
		items = append(items, orders.RefundItem{VariantID: it.VariantID, Qty: it.Qty, Restock: it.Restock})
	}
	return items, true
}

// writeRefundError maps a repository refund failure to its response. A default (unmatched) error is
// a PSP refund call or persistence failure and is treated as an upstream/gateway error.
func writeRefundError(w http.ResponseWriter, orderID uuid.UUID, err error) {
	switch {
	case errors.Is(err, orders.ErrOrderNotFound):
		webutils.ErrorJSON(w, err, http.StatusNotFound)
	case errors.Is(err, orders.ErrOrderNotRefundable):
		webutils.ErrorJSON(w, errors.New("order is not in a refundable state"), http.StatusConflict)
	case errors.Is(err, orders.ErrRefundNotSupported):
		webutils.ErrorJSON(w, err, http.StatusNotImplemented)
	case errors.Is(err, orders.ErrMissingPaymentReference), errors.Is(err, orders.ErrRefundAmountInvalid):
		webutils.ErrorJSON(w, err, http.StatusUnprocessableEntity)
	case errors.Is(err, orders.ErrRefundItemNotFound):
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
	default:
		slog.Error("failed to refund order", "order_id", orderID, "error", err)
		webutils.ErrorJSON(w, errors.New("failed to refund order"), http.StatusBadGateway)
	}
}
