package handlers

import (
	"bullet-commerce/internal/addresses"
	"bullet-commerce/internal/cart"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/webutils"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// pixChargeTTL bounds how long a started PIX charge stays payable. Kept inside ProPay's
// accepted 300..86400s window; the shopper polls the flow until it settles or expires.
const pixChargeTTL = time.Hour

type OrderHandler struct {
	OrderRepo   orders.OrderRepository
	CartRepo    cart.CartRepository
	AddressRepo addresses.AddressRepository
	// PaymentRegistry + PaymentProvider select the configured PSP for the pay flow. The
	// handler never imports a concrete provider - it resolves one by config name.
	PaymentRegistry *payment.Registry
	PaymentProvider payment.Name
}

func NewOrderHandler(orderRepo orders.OrderRepository, cartRepo cart.CartRepository, addressRepo addresses.AddressRepository, paymentRegistry *payment.Registry, paymentProvider payment.Name) *OrderHandler {
	return &OrderHandler{
		OrderRepo:       orderRepo,
		CartRepo:        cartRepo,
		AddressRepo:     addressRepo,
		PaymentRegistry: paymentRegistry,
		PaymentProvider: paymentProvider,
	}
}

type CreateOrderRequest struct {
	ShippingAddressID uuid.UUID `json:"shipping_address_id"`
	// ShippingCostCents/ShippingMethod are supplied by the client after quoting via
	// POST /api/shipping/calculate. WHY accept (vs recompute here): the freight quote is
	// a separate bounded step; the order records the chosen cost and adds it to the total.
	ShippingCostCents int64   `json:"shipping_cost_cents"`
	ShippingMethod    *string `json:"shipping_method,omitempty"`
}

type OrderResponse struct {
	Order models.Order       `json:"order"`
	Items []models.OrderItem `json:"items"`
}

type UpdateTrackingRequest struct {
	TrackingNumber string `json:"tracking_number"`
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	var req CreateOrderRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	// Idempotency is OPT-IN via the Idempotency-Key header. When present, a repeated request
	// with the same key (the classic double-click / client retry) replays the original
	// response instead of creating a second order or reserving stock again. Absent the header,
	// behavior is unchanged. Fast path: replay a finalized key (or 409 an in-flight one) before
	// doing any work, so a replay succeeds even if the cart has since changed.
	idempKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempKey != "" {
		rec, err := h.OrderRepo.LookupIdempotencyKey(r.Context(), authUserID, idempKey)
		if err != nil {
			webutils.ErrorJSON(w, errors.New("failed to check idempotency key"), http.StatusInternalServerError)
			return
		}
		if rec != nil {
			if rec.Completed {
				webutils.RawJSON(w, rec.ResponseStatus, rec.ResponseBody)
				return
			}
			// An identical request is still being processed - do not start a second one.
			webutils.ErrorJSON(w, errors.New("a request with this Idempotency-Key is already in progress"), http.StatusConflict)
			return
		}
	}

	if _, err := h.AddressRepo.FindByUserAndID(r.Context(), authUserID, req.ShippingAddressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, errors.New("shipping address not found or does not belong to user"), http.StatusBadRequest)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate shipping address"), http.StatusInternalServerError)
		}
		return
	}

	userCart, err := h.CartRepo.GetOrCreateCartByUserID(r.Context(), authUserID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart"), http.StatusInternalServerError)
		return
	}

	cartItems, err := h.CartRepo.GetCartItems(r.Context(), userCart.ID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart items"), http.StatusInternalServerError)
		return
	}

	if len(cartItems) == 0 {
		webutils.ErrorJSON(w, errors.New("cannot create order from empty cart"), http.StatusBadRequest)
		return
	}

	if req.ShippingCostCents < 0 {
		webutils.ErrorJSON(w, errors.New("shipping_cost_cents cannot be negative"), http.StatusBadRequest)
		return
	}

	// Reserve the idempotency key immediately before creating the order - this INSERT is the
	// serialization point that makes the double-click reserve stock exactly once. WHY here (not
	// at the top): the read-only validations above may legitimately fail and be retried, so we
	// only gate the state-changing step. A lost race (claimed == false) means a concurrent
	// request beat us to it: replay if it already finished, else 409.
	if idempKey != "" {
		claimed, err := h.OrderRepo.ClaimIdempotencyKey(r.Context(), authUserID, idempKey, hashOrderRequest(req))
		if err != nil {
			webutils.ErrorJSON(w, errors.New("failed to reserve idempotency key"), http.StatusInternalServerError)
			return
		}
		if !claimed {
			if rec, lerr := h.OrderRepo.LookupIdempotencyKey(r.Context(), authUserID, idempKey); lerr == nil && rec != nil && rec.Completed {
				webutils.RawJSON(w, rec.ResponseStatus, rec.ResponseBody)
				return
			}
			webutils.ErrorJSON(w, errors.New("a request with this Idempotency-Key is already in progress"), http.StatusConflict)
			return
		}
	}

	newOrder, err := h.OrderRepo.CreateOrderFromCart(r.Context(), authUserID, userCart.ID, req.ShippingAddressID, cartItems, req.ShippingCostCents, req.ShippingMethod)
	if err != nil {
		// Creation rolled back (no stock reserved): drop the in-flight reservation so the
		// client may retry with the same key.
		if idempKey != "" {
			if rerr := h.OrderRepo.ReleaseIdempotencyKey(r.Context(), authUserID, idempKey); rerr != nil {
				slog.Error("failed to release idempotency key after order failure", "user_id", authUserID, "error", rerr)
			}
		}
		slog.Error("failed to create order", "user_id", authUserID, "error", err)
		if errors.Is(err, orders.ErrInsufficientStock) {
			webutils.ErrorJSON(w, errors.New("insufficient stock for one or more items"), http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to create order"), http.StatusInternalServerError)
		}
		return
	}

	// Build the response body once so the same bytes are both returned now and stored for
	// replay under the idempotency key.
	var body []byte
	if createdOrder, createdItems, ferr := h.OrderRepo.FindOrderByID(r.Context(), newOrder.ID); ferr == nil {
		body, _ = json.Marshal(OrderResponse{Order: *createdOrder, Items: createdItems})
	} else {
		body, _ = json.Marshal(newOrder)
	}

	if idempKey != "" {
		if err := h.OrderRepo.SaveIdempotencyKey(r.Context(), authUserID, idempKey, http.StatusCreated, body, newOrder.ID); err != nil {
			slog.Error("failed to persist idempotency key", "order_id", newOrder.ID, "error", err)
		}
	}

	webutils.RawJSON(w, http.StatusCreated, body)
}

// hashOrderRequest fingerprints the create-order request so a reused Idempotency-Key can be
// recorded alongside the payload it was first used with (stored for auditing/debugging).
func hashOrderRequest(req CreateOrderRequest) string {
	b, _ := json.Marshal(req)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	limit, offset := parsePagination(r)
	list, err := h.OrderRepo.FindUserOrders(r.Context(), authUserID, limit, offset)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve orders"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	orderID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order ID format"), http.StatusBadRequest)
		return
	}

	order, items, err := h.OrderRepo.FindOrderByID(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve order"), http.StatusInternalServerError)
		}
		return
	}

	if order.UserID != authUserID {
		webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, OrderResponse{Order: *order, Items: items})
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	orderID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order ID format"), http.StatusBadRequest)
		return
	}

	order, _, err := h.OrderRepo.FindOrderByID(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve order"), http.StatusInternalServerError)
		}
		return
	}

	if order.UserID != authUserID {
		webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	// CancelOrder atomically flips status to cancelled and releases the reservation for
	// not-yet-paid orders (physical stock untouched).
	if err := h.OrderRepo.CancelOrder(r.Context(), orderID); err != nil {
		if errors.Is(err, orders.ErrInvalidStatusTransition) {
			webutils.ErrorJSON(w, errors.New("order cannot be cancelled in its current state"), http.StatusConflict)
		} else if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to cancel order"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Pay starts a payment flow for an order: it resolves the configured PSP, asks it to
// StartFlow for the order total, records the order as pending_payment with the PSP
// reference, and returns the FlowStatus (PIX QR / redirect action) for the client to act on.
func (h *OrderHandler) Pay(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	orderID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order ID format"), http.StatusBadRequest)
		return
	}

	order, _, err := h.OrderRepo.FindOrderByID(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve order"), http.StatusInternalServerError)
		}
		return
	}

	if order.UserID != authUserID {
		webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	// Only a not-yet-settled order can start (or restart) a payment flow.
	if order.PaymentStatus != models.PaymentUnpaid && order.PaymentStatus != models.PaymentPending {
		webutils.ErrorJSON(w, errors.New("order is not payable in its current state"), http.StatusConflict)
		return
	}

	provider, err := h.PaymentRegistry.Get(h.PaymentProvider)
	if err != nil {
		slog.Error("payment provider not configured", "provider", h.PaymentProvider, "error", err)
		webutils.ErrorJSON(w, errors.New("payment provider not available"), http.StatusServiceUnavailable)
		return
	}

	currency := order.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	charge, flow, err := provider.StartFlow(r.Context(), payment.PixChargeRequest{
		ReferenceID: order.ID.String(),
		Amount:      payment.Money(order.TotalCents),
		Currency:    payment.Currency(currency),
		ExpiresIn:   pixChargeTTL,
	})
	if err != nil {
		slog.Error("failed to start payment flow", "order_id", orderID, "error", err)
		webutils.ErrorJSON(w, errors.New("failed to start payment"), http.StatusBadGateway)
		return
	}

	// Record the order as pending_payment with the PSP reference so the webhook can be
	// reconciled back to this order.
	reference := charge.ProviderID
	if reference == "" {
		reference = charge.TxID
	}
	if err := h.OrderRepo.MarkPendingPayment(r.Context(), orderID, reference); err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, errors.New("order is not payable in its current state"), http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to record payment"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, flow)
}

func (h *OrderHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid order ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateTrackingRequest
	if err := webutils.ReadJSON(r, &req); err != nil || req.TrackingNumber == "" {
		webutils.ErrorJSON(w, errors.New("tracking_number is required"), http.StatusBadRequest)
		return
	}

	if err := h.OrderRepo.UpdateOrderTracking(r.Context(), orderID, req.TrackingNumber); err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update tracking"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *OrderHandler) TrackOrder(w http.ResponseWriter, r *http.Request) {
	webutils.ErrorJSON(w, errors.New("not implemented"), http.StatusNotImplemented)
}

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

	// An empty body is valid: it means a full refund. Only a malformed (non-empty) body is 400.
	var req RefundOrderRequest
	if err := webutils.ReadJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.AmountCents < 0 {
		webutils.ErrorJSON(w, errors.New("amount_cents cannot be negative"), http.StatusBadRequest)
		return
	}

	provider, err := h.PaymentRegistry.Get(h.PaymentProvider)
	if err != nil {
		slog.Error("payment provider not configured", "provider", h.PaymentProvider, "error", err)
		webutils.ErrorJSON(w, errors.New("payment provider not available"), http.StatusServiceUnavailable)
		return
	}
	// Refunder is an optional PSP capability - reach it by type assertion, never redefine it.
	refunder, ok := provider.(payment.Refunder)
	if !ok {
		webutils.ErrorJSON(w, errors.New("payment provider does not support refunds"), http.StatusNotImplemented)
		return
	}

	items := make([]orders.RefundItem, 0, len(req.Items))
	for _, it := range req.Items {
		if it.Restock && it.Qty <= 0 {
			webutils.ErrorJSON(w, errors.New("qty must be positive for a restocked item"), http.StatusBadRequest)
			return
		}
		items = append(items, orders.RefundItem{VariantID: it.VariantID, Qty: it.Qty, Restock: it.Restock})
	}

	if err := h.OrderRepo.RefundOrder(r.Context(), refunder, orderID, items, req.AmountCents); err != nil {
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
			// PSP refund call or persistence failed - treat as an upstream/gateway error.
			slog.Error("failed to refund order", "order_id", orderID, "error", err)
			webutils.ErrorJSON(w, errors.New("failed to refund order"), http.StatusBadGateway)
		}
		return
	}

	// Return the refreshed order so the caller sees the new payment_status / refund totals.
	if order, refreshedItems, ferr := h.OrderRepo.FindOrderByID(r.Context(), orderID); ferr == nil {
		webutils.WriteJSON(w, http.StatusOK, OrderResponse{Order: *order, Items: refreshedItems})
		return
	}
	w.WriteHeader(http.StatusOK)
}
