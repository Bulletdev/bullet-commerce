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
	// behavior is unchanged.
	idempKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if h.replayIdempotentOrder(w, r, authUserID, idempKey) {
		return
	}

	userCart, cartItems, ok := h.loadOrderPrerequisites(w, r, authUserID, req)
	if !ok {
		return
	}

	if !h.claimOrderIdempotencyKey(w, r, authUserID, idempKey, req) {
		return
	}

	newOrder, ok := h.createOrderFromCart(w, r, authUserID, idempKey, userCart.ID, cartItems, req)
	if !ok {
		return
	}

	h.writeCreatedOrder(w, r, authUserID, idempKey, newOrder)
}

// replayIdempotentOrder is the fast path for an opt-in Idempotency-Key: replay a finalized key
// (or 409 an in-flight one) before doing any work, so a replay succeeds even if the cart has
// since changed. Returns true when it has already written the response (caller must stop).
func (h *OrderHandler) replayIdempotentOrder(w http.ResponseWriter, r *http.Request, userID uuid.UUID, idempKey string) bool {
	if idempKey == "" {
		return false
	}
	rec, err := h.OrderRepo.LookupIdempotencyKey(r.Context(), userID, idempKey)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to check idempotency key"), http.StatusInternalServerError)
		return true
	}
	if rec == nil {
		return false
	}
	if rec.Completed {
		webutils.RawJSON(w, rec.ResponseStatus, rec.ResponseBody)
		return true
	}
	// An identical request is still being processed - do not start a second one.
	webutils.ErrorJSON(w, errors.New("a request with this Idempotency-Key is already in progress"), http.StatusConflict)
	return true
}

// loadOrderPrerequisites validates the shipping address, loads the cart and its items and rejects
// an empty cart or a negative shipping cost. Returns ok=false after having written the response.
func (h *OrderHandler) loadOrderPrerequisites(w http.ResponseWriter, r *http.Request, userID uuid.UUID, req CreateOrderRequest) (*models.Cart, []models.CartItem, bool) {
	if _, err := h.AddressRepo.FindByUserAndID(r.Context(), userID, req.ShippingAddressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, errors.New("shipping address not found or does not belong to user"), http.StatusBadRequest)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate shipping address"), http.StatusInternalServerError)
		}
		return nil, nil, false
	}

	userCart, err := h.CartRepo.GetOrCreateCartByUserID(r.Context(), userID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart"), http.StatusInternalServerError)
		return nil, nil, false
	}

	cartItems, err := h.CartRepo.GetCartItems(r.Context(), userCart.ID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart items"), http.StatusInternalServerError)
		return nil, nil, false
	}

	if len(cartItems) == 0 {
		webutils.ErrorJSON(w, errors.New("cannot create order from empty cart"), http.StatusBadRequest)
		return nil, nil, false
	}

	if req.ShippingCostCents < 0 {
		webutils.ErrorJSON(w, errors.New("shipping_cost_cents cannot be negative"), http.StatusBadRequest)
		return nil, nil, false
	}

	return userCart, cartItems, true
}

// claimOrderIdempotencyKey reserves the key immediately before creating the order - this INSERT is
// the serialization point that makes the double-click reserve stock exactly once. WHY here (not at
// the top): the read-only validations above may legitimately fail and be retried, so we only gate
// the state-changing step. A lost race (claimed == false) means a concurrent request beat us to it:
// replay if it already finished, else 409. Returns false after having written the response.
func (h *OrderHandler) claimOrderIdempotencyKey(w http.ResponseWriter, r *http.Request, userID uuid.UUID, idempKey string, req CreateOrderRequest) bool {
	if idempKey == "" {
		return true
	}
	claimed, err := h.OrderRepo.ClaimIdempotencyKey(r.Context(), userID, idempKey, hashOrderRequest(req))
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to reserve idempotency key"), http.StatusInternalServerError)
		return false
	}
	if !claimed {
		if rec, lerr := h.OrderRepo.LookupIdempotencyKey(r.Context(), userID, idempKey); lerr == nil && rec != nil && rec.Completed {
			webutils.RawJSON(w, rec.ResponseStatus, rec.ResponseBody)
			return false
		}
		webutils.ErrorJSON(w, errors.New("a request with this Idempotency-Key is already in progress"), http.StatusConflict)
		return false
	}
	return true
}

// createOrderFromCart runs the state-changing order creation and, on failure, drops the in-flight
// idempotency reservation so the client may retry with the same key. Returns ok=false after having
// written the response.
func (h *OrderHandler) createOrderFromCart(w http.ResponseWriter, r *http.Request, userID uuid.UUID, idempKey string, cartID uuid.UUID, cartItems []models.CartItem, req CreateOrderRequest) (*models.Order, bool) {
	newOrder, err := h.OrderRepo.CreateOrderFromCart(r.Context(), userID, cartID, req.ShippingAddressID, cartItems, req.ShippingCostCents, req.ShippingMethod)
	if err != nil {
		// Creation rolled back (no stock reserved): drop the in-flight reservation so the
		// client may retry with the same key.
		if idempKey != "" {
			if rerr := h.OrderRepo.ReleaseIdempotencyKey(r.Context(), userID, idempKey); rerr != nil {
				slog.Error("failed to release idempotency key after order failure", "user_id", userID, "error", rerr)
			}
		}
		slog.Error("failed to create order", "user_id", userID, "error", err)
		if errors.Is(err, orders.ErrInsufficientStock) {
			webutils.ErrorJSON(w, errors.New("insufficient stock for one or more items"), http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to create order"), http.StatusInternalServerError)
		}
		return nil, false
	}
	return newOrder, true
}

// writeCreatedOrder builds the response body once so the same bytes are both returned now and
// stored for replay under the idempotency key.
func (h *OrderHandler) writeCreatedOrder(w http.ResponseWriter, r *http.Request, userID uuid.UUID, idempKey string, newOrder *models.Order) {
	var body []byte
	if createdOrder, createdItems, ferr := h.OrderRepo.FindOrderByID(r.Context(), newOrder.ID); ferr == nil {
		body, _ = json.Marshal(OrderResponse{Order: *createdOrder, Items: createdItems})
	} else {
		body, _ = json.Marshal(newOrder)
	}

	if idempKey != "" {
		if err := h.OrderRepo.SaveIdempotencyKey(r.Context(), userID, idempKey, http.StatusCreated, body, newOrder.ID); err != nil {
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

	if _, ok := h.loadOwnedOrder(w, r, authUserID, orderID); !ok {
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

	order, ok := h.loadOwnedOrder(w, r, authUserID, orderID)
	if !ok {
		return
	}

	// Only a not-yet-settled order can start (or restart) a payment flow.
	if order.PaymentStatus != models.PaymentUnpaid && order.PaymentStatus != models.PaymentPending {
		webutils.ErrorJSON(w, errors.New("order is not payable in its current state"), http.StatusConflict)
		return
	}

	flow, ok := h.startOrderPayment(w, r, order)
	if !ok {
		return
	}

	webutils.WriteJSON(w, http.StatusOK, flow)
}

// loadOwnedOrder loads the order and enforces that the authenticated user owns it, writing the
// response on any failure. Returns ok=false once it has responded.
func (h *OrderHandler) loadOwnedOrder(w http.ResponseWriter, r *http.Request, userID, orderID uuid.UUID) (*models.Order, bool) {
	order, _, err := h.OrderRepo.FindOrderByID(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve order"), http.StatusInternalServerError)
		}
		return nil, false
	}
	if order.UserID != userID {
		webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
		return nil, false
	}
	return order, true
}

// startOrderPayment resolves the configured PSP, asks it to StartFlow for the order total and
// records the order as pending_payment with the PSP reference so the webhook can be reconciled back
// to this order. Returns the FlowStatus for the client to act on, or ok=false after responding.
func (h *OrderHandler) startOrderPayment(w http.ResponseWriter, r *http.Request, order *models.Order) (payment.FlowStatus, bool) {
	provider, err := h.PaymentRegistry.Get(h.PaymentProvider)
	if err != nil {
		slog.Error("payment provider not configured", "provider", h.PaymentProvider, "error", err)
		webutils.ErrorJSON(w, errors.New("payment provider not available"), http.StatusServiceUnavailable)
		return payment.FlowStatus{}, false
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
		slog.Error("failed to start payment flow", "order_id", order.ID, "error", err)
		webutils.ErrorJSON(w, errors.New("failed to start payment"), http.StatusBadGateway)
		return payment.FlowStatus{}, false
	}

	// Record the order as pending_payment with the PSP reference so the webhook can be
	// reconciled back to this order.
	reference := charge.ProviderID
	if reference == "" {
		reference = charge.TxID
	}
	if err := h.OrderRepo.MarkPendingPayment(r.Context(), order.ID, reference); err != nil {
		if errors.Is(err, orders.ErrOrderNotFound) {
			webutils.ErrorJSON(w, errors.New("order is not payable in its current state"), http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to record payment"), http.StatusInternalServerError)
		}
		return payment.FlowStatus{}, false
	}

	return flow, true
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
