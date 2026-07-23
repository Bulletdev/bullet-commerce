package handlers_test

import (
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func buildOrderRouter(
	t *testing.T,
	userID uuid.UUID,
	userRepo *MockUserRepository,
	orderRepo *MockOrderRepository,
) (*mux.Router, string) {
	t.Helper()
	tok, err := generateTestToken(userID)
	require.NoError(t, err)

	authMW := auth.NewMiddleware(testJwtSecret, userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, new(MockCartRepository), new(MockAddressRepository), payment.NewRegistry(), "")

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	p := api.NewRoute().Subrouter()
	p.Use(authMW.Authenticate)
	p.HandleFunc("/orders", orderHandler.ListOrders).Methods(http.MethodGet)
	p.HandleFunc("/orders", orderHandler.CreateOrder).Methods(http.MethodPost)
	p.HandleFunc("/orders/{id:[0-9a-fA-F-]+}", orderHandler.GetOrder).Methods(http.MethodGet)
	p.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/cancel", orderHandler.CancelOrder).Methods(http.MethodPatch)
	p.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/tracking", orderHandler.UpdateTracking).Methods(http.MethodPatch)
	r.HandleFunc("/api/orders/tracking/{number}", orderHandler.TrackOrder).Methods(http.MethodGet)
	return r, tok
}

func TestOrderHandler_ListOrders(t *testing.T) {
	testUserID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindUserOrders", mock.Anything, testUserID, 20, 0).
		Return([]models.Order{{ID: uuid.New(), Status: "pending", PaymentStatus: "unpaid", TotalCents: 5000}}, nil).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestOrderHandler_ListOrders_DBError(t *testing.T) {
	testUserID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindUserOrders", mock.Anything, testUserID, 20, 0).Return(nil, assert.AnError).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestOrderHandler_GetOrder_Success(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: testUserID, Status: "pending", PaymentStatus: "unpaid"}, []models.OrderItem{}, nil).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/orders/%s", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestOrderHandler_GetOrder_NotFound(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).Return(nil, nil, orders.ErrOrderNotFound).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/orders/%s", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestOrderHandler_GetOrder_Forbidden(t *testing.T) {
	testUserID := uuid.New()
	otherUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: otherUserID, Status: "pending", PaymentStatus: "unpaid"}, []models.OrderItem{}, nil).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/orders/%s", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestOrderHandler_GetOrder_DBError(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).Return(nil, nil, assert.AnError).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/orders/%s", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestOrderHandler_CancelOrder_Success(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: testUserID, Status: models.StatusPending, PaymentStatus: models.PaymentUnpaid}, []models.OrderItem{}, nil).Once()
	orderRepo.On("CancelOrder", mock.Anything, orderID).Return(nil).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/orders/%s/cancel", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestOrderHandler_CancelOrder_InvalidTransition(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: testUserID, Status: models.StatusDelivered, PaymentStatus: models.PaymentPaid}, []models.OrderItem{}, nil).Once()
	orderRepo.On("CancelOrder", mock.Anything, orderID).
		Return(orders.ErrInvalidStatusTransition).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/orders/%s/cancel", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestOrderHandler_UpdateTracking_Success(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleAdmin}, nil)
	orderRepo.On("UpdateOrderTracking", mock.Anything, orderID, "BR123456789").Return(nil).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	body := bytes.NewBufferString(`{"tracking_number":"BR123456789"}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/orders/%s/tracking", orderID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestOrderHandler_UpdateTracking_EmptyBody(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleAdmin}, nil)

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/orders/%s/tracking", orderID), bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestOrderHandler_UpdateTracking_NotFound(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleAdmin}, nil)
	orderRepo.On("UpdateOrderTracking", mock.Anything, orderID, "BR123").Return(orders.ErrOrderNotFound).Once()

	r, tok := buildOrderRouter(t, testUserID, userRepo, orderRepo)

	body := bytes.NewBufferString(`{"tracking_number":"BR123"}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/orders/%s/tracking", orderID), body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestOrderHandler_TrackOrder(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	orderHandler := handlers.NewOrderHandler(orderRepo, new(MockCartRepository), new(MockAddressRepository), payment.NewRegistry(), "")

	r := mux.NewRouter()
	r.HandleFunc("/api/orders/tracking/{number}", orderHandler.TrackOrder).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/orders/tracking/BR123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotImplemented, rr.Code)
}

// CreateOrder passes the client-supplied shipping cost/method through to the repo and
// returns 201 with the created order.
func TestOrderHandler_CreateOrder_WithShipping(t *testing.T) {
	testUserID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)
	cartRepo := new(MockCartRepository)
	addrRepo := new(MockAddressRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)

	cartID, addrID, orderID := uuid.New(), uuid.New(), uuid.New()
	items := []models.CartItem{{ProductID: uuid.New(), VariantID: uuid.New(), Quantity: 1, PriceCents: 1000}}
	addrRepo.On("FindByUserAndID", mock.Anything, testUserID, addrID).Return(&models.Address{ID: addrID}, nil).Once()
	cartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(&models.Cart{ID: cartID}, nil).Once()
	cartRepo.On("GetCartItems", mock.Anything, cartID).Return(items, nil).Once()
	method := "table-sudeste"
	orderRepo.On("CreateOrderFromCart", mock.Anything, testUserID, cartID, addrID, items, int64(1500), &method).
		Return(&models.Order{ID: orderID, TotalCents: 2500, ShippingCostCents: 1500}, nil).Once()
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: testUserID, TotalCents: 2500, ShippingCostCents: 1500}, []models.OrderItem{}, nil).Once()

	tok, _ := generateTestToken(testUserID)
	authMW := auth.NewMiddleware(testJwtSecret, userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, cartRepo, addrRepo, payment.NewRegistry(), "")

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	p := api.NewRoute().Subrouter()
	p.Use(authMW.Authenticate)
	p.HandleFunc("/orders", orderHandler.CreateOrder).Methods(http.MethodPost)

	body := bytes.NewBufferString(fmt.Sprintf(`{"shipping_address_id":"%s","shipping_cost_cents":1500,"shipping_method":"table-sudeste"}`, addrID))
	req := httptest.NewRequest(http.MethodPost, "/api/orders", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	orderRepo.AssertExpectations(t)
}

// Insufficient variant stock aborts the order and maps to 409 Conflict.
func TestOrderHandler_CreateOrder_InsufficientStock(t *testing.T) {
	testUserID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)
	cartRepo := new(MockCartRepository)
	addrRepo := new(MockAddressRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)

	cartID, addrID := uuid.New(), uuid.New()
	items := []models.CartItem{{ProductID: uuid.New(), VariantID: uuid.New(), Quantity: 3, PriceCents: 1000}}
	addrRepo.On("FindByUserAndID", mock.Anything, testUserID, addrID).Return(&models.Address{ID: addrID}, nil).Once()
	cartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(&models.Cart{ID: cartID}, nil).Once()
	cartRepo.On("GetCartItems", mock.Anything, cartID).Return(items, nil).Once()
	orderRepo.On("CreateOrderFromCart", mock.Anything, testUserID, cartID, addrID, items, int64(0), (*string)(nil)).
		Return(nil, orders.ErrInsufficientStock).Once()

	tok, _ := generateTestToken(testUserID)
	authMW := auth.NewMiddleware(testJwtSecret, userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, cartRepo, addrRepo, payment.NewRegistry(), "")

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	p := api.NewRoute().Subrouter()
	p.Use(authMW.Authenticate)
	p.HandleFunc("/orders", orderHandler.CreateOrder).Methods(http.MethodPost)

	body := bytes.NewBufferString(fmt.Sprintf(`{"shipping_address_id":"%s"}`, addrID))
	req := httptest.NewRequest(http.MethodPost, "/api/orders", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	orderRepo.AssertExpectations(t)
}

func TestOrderHandler_CreateOrder_EmptyCart(t *testing.T) {
	testUserID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)
	cartRepo := new(MockCartRepository)
	addrRepo := new(MockAddressRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)

	cartID := uuid.New()
	addrID := uuid.New()
	addrRepo.On("FindByUserAndID", mock.Anything, testUserID, addrID).Return(&models.Address{ID: addrID}, nil).Once()
	cartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(&models.Cart{ID: cartID}, nil).Once()
	cartRepo.On("GetCartItems", mock.Anything, cartID).Return([]models.CartItem{}, nil).Once()

	tok, _ := generateTestToken(testUserID)
	authMW := auth.NewMiddleware(testJwtSecret, userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, cartRepo, addrRepo, payment.NewRegistry(), "")

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	p := api.NewRoute().Subrouter()
	p.Use(authMW.Authenticate)
	p.HandleFunc("/orders", orderHandler.CreateOrder).Methods(http.MethodPost)

	body := bytes.NewBufferString(fmt.Sprintf(`{"shipping_address_id":"%s"}`, addrID))
	req := httptest.NewRequest(http.MethodPost, "/api/orders", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// fakeProvider is a configurable payment.Provider stand-in for the pay/webhook flows.
// Only StartFlow and VerifyWebhook are exercised; the rest satisfy the interface.
type fakeProvider struct {
	startCharge *payment.PixCharge
	startFlow   payment.FlowStatus
	startErr    error

	webhookEvent *payment.WebhookEvent
	webhookErr   error
}

func (f *fakeProvider) Name() payment.Name { return payment.Name("fake") }
func (f *fakeProvider) CreatePixCharge(ctx context.Context, req payment.PixChargeRequest) (*payment.PixCharge, error) {
	return f.startCharge, f.startErr
}
func (f *fakeProvider) GetCharge(ctx context.Context, providerID string) (*payment.PixCharge, error) {
	return f.startCharge, nil
}
func (f *fakeProvider) StartFlow(ctx context.Context, req payment.PixChargeRequest) (*payment.PixCharge, payment.FlowStatus, error) {
	if f.startErr != nil {
		return nil, payment.FlowStatus{}, f.startErr
	}
	return f.startCharge, f.startFlow, nil
}
func (f *fakeProvider) FlowState(ctx context.Context, providerID string) (payment.FlowStatus, error) {
	return f.startFlow, nil
}
func (f *fakeProvider) CancelCharge(ctx context.Context, providerID, reason string) error { return nil }
func (f *fakeProvider) VerifyWebhook(ctx context.Context, raw payment.RawWebhook) (*payment.WebhookEvent, error) {
	return f.webhookEvent, f.webhookErr
}

func registryWith(p payment.Provider) *payment.Registry {
	reg := payment.NewRegistry()
	reg.Register(p)
	return reg
}

// Pay resolves the configured provider, starts a flow, marks the order pending_payment
// with the PSP reference, and returns the FlowStatus.
func TestOrderHandler_Pay_ReturnsFlowStatus(t *testing.T) {
	testUserID := uuid.New()
	orderID := uuid.New()
	userRepo := new(MockUserRepository)
	orderRepo := new(MockOrderRepository)

	userRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID, Role: models.RoleUser}, nil)
	orderRepo.On("FindOrderByID", mock.Anything, orderID).
		Return(&models.Order{ID: orderID, UserID: testUserID, PaymentStatus: models.PaymentUnpaid, TotalCents: 2500, Currency: "BRL"}, []models.OrderItem{}, nil).Once()
	orderRepo.On("MarkPendingPayment", mock.Anything, orderID, "propay-tx-9").Return(nil).Once()

	prov := &fakeProvider{
		startCharge: &payment.PixCharge{ProviderID: "propay-tx-9"},
		startFlow: payment.FlowStatus{
			Status:     payment.ChargePending,
			Action:     payment.ActionDisplayPix,
			ActionData: map[string]string{"copy_paste": "0002012..."},
		},
	}

	tok, _ := generateTestToken(testUserID)
	authMW := auth.NewMiddleware(testJwtSecret, userRepo)
	orderHandler := handlers.NewOrderHandler(orderRepo, new(MockCartRepository), new(MockAddressRepository), registryWith(prov), prov.Name())

	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	p := api.NewRoute().Subrouter()
	p.Use(authMW.Authenticate)
	p.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/pay", orderHandler.Pay).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/orders/%s/pay", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "display_pix")
	orderRepo.AssertExpectations(t)
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	prov := &fakeProvider{webhookErr: payment.ErrInvalidSignature}
	wh := handlers.NewWebhookHandler(orderRepo, registryWith(prov), prov.Name())

	r := mux.NewRouter()
	r.HandleFunc("/api/webhooks/payment", wh.HandlePayment).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/payment", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestWebhookHandler_PaidConfirmsOrder(t *testing.T) {
	orderID := uuid.New()
	orderRepo := new(MockOrderRepository)
	orderRepo.On("ConfirmOrderPayment", mock.Anything, orderID).Return(nil).Once()

	prov := &fakeProvider{webhookEvent: &payment.WebhookEvent{
		Type:        payment.EventChargePaid,
		ReferenceID: orderID.String(),
	}}
	wh := handlers.NewWebhookHandler(orderRepo, registryWith(prov), prov.Name())

	r := mux.NewRouter()
	r.HandleFunc("/api/webhooks/payment", wh.HandlePayment).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/payment", bytes.NewBufferString(`{"event":"charge.paid"}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	orderRepo.AssertExpectations(t)
}

func TestWebhookHandler_UnknownEventNoOp(t *testing.T) {
	orderRepo := new(MockOrderRepository)
	prov := &fakeProvider{webhookEvent: &payment.WebhookEvent{Type: payment.EventUnknown}}
	wh := handlers.NewWebhookHandler(orderRepo, registryWith(prov), prov.Name())

	r := mux.NewRouter()
	r.HandleFunc("/api/webhooks/payment", wh.HandlePayment).Methods(http.MethodPost)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/payment", bytes.NewBufferString(`{"event":"whatever"}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// No confirmation should have been attempted.
	orderRepo.AssertNotCalled(t, "ConfirmOrderPayment", mock.Anything, mock.Anything)
}
