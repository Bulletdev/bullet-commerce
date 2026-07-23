package handlers_test

import (
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/products"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockOrderRepository struct{ mock.Mock }

func (m *MockOrderRepository) CreateOrderFromCart(ctx context.Context, userID, cartID, addrID uuid.UUID, items []models.CartItem, shippingCostCents int64, shippingMethod *string) (*models.Order, error) {
	args := m.Called(ctx, userID, cartID, addrID, items, shippingCostCents, shippingMethod)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}
func (m *MockOrderRepository) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	return m.Called(ctx, orderID).Error(0)
}
func (m *MockOrderRepository) ConfirmOrderPayment(ctx context.Context, orderID uuid.UUID) error {
	return m.Called(ctx, orderID).Error(0)
}
func (m *MockOrderRepository) MarkPendingPayment(ctx context.Context, orderID uuid.UUID, reference string) error {
	return m.Called(ctx, orderID, reference).Error(0)
}
func (m *MockOrderRepository) ExpireUnpaidOrders(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockOrderRepository) FindUserOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Order, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Order), args.Error(1)
}
func (m *MockOrderRepository) FindOrderByID(ctx context.Context, orderID uuid.UUID) (*models.Order, []models.OrderItem, error) {
	args := m.Called(ctx, orderID)
	var order *models.Order
	var items []models.OrderItem
	if args.Get(0) != nil {
		order = args.Get(0).(*models.Order)
	}
	if args.Get(1) != nil {
		items = args.Get(1).([]models.OrderItem)
	}
	return order, items, args.Error(2)
}
func (m *MockOrderRepository) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, current, next models.OrderStatus) error {
	return m.Called(ctx, orderID, current, next).Error(0)
}
func (m *MockOrderRepository) UpdateOrderTracking(ctx context.Context, orderID uuid.UUID, tracking string) error {
	return m.Called(ctx, orderID, tracking).Error(0)
}
func (m *MockOrderRepository) ExpireOrphanedOrders(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockOrderRepository) RefundOrder(ctx context.Context, refunder payment.Refunder, orderID uuid.UUID, items []orders.RefundItem, amountCents int64) error {
	return m.Called(ctx, refunder, orderID, items, amountCents).Error(0)
}
func (m *MockOrderRepository) LookupIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*orders.IdempotencyRecord, error) {
	args := m.Called(ctx, userID, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*orders.IdempotencyRecord), args.Error(1)
}
func (m *MockOrderRepository) ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, requestHash string) (bool, error) {
	args := m.Called(ctx, userID, key, requestHash)
	return args.Bool(0), args.Error(1)
}
func (m *MockOrderRepository) SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, status int, body []byte, orderID uuid.UUID) error {
	return m.Called(ctx, userID, key, status, body, orderID).Error(0)
}
func (m *MockOrderRepository) ReleaseIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) error {
	return m.Called(ctx, userID, key).Error(0)
}

const testJwtSecret = "um-segredo-super-secreto-para-testes-123"

type MockUserRepository struct{ mock.Mock }

func (m *MockUserRepository) Create(ctx context.Context, name, email, passwordHash string) (*models.User, error) {
	args := m.Called(ctx, name, email, passwordHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *MockUserRepository) Update(ctx context.Context, id uuid.UUID, name, email string, cpf *string) (*models.User, error) {
	args := m.Called(ctx, id, name, email, cpf)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

type MockProductRepository struct{ mock.Mock }

func (m *MockProductRepository) FindAll(ctx context.Context, limit, offset int) ([]models.Product, error) {
	args := m.Called(ctx, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Product), args.Error(1)
}
func (m *MockProductRepository) FindFeatured(ctx context.Context) ([]models.Product, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Product), args.Error(1)
}
func (m *MockProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepository) FindByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepository) FindByCategoryID(ctx context.Context, categoryID uuid.UUID, limit, offset int) ([]models.Product, error) {
	args := m.Called(ctx, categoryID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Product), args.Error(1)
}
func (m *MockProductRepository) Create(ctx context.Context, product *models.Product) (*models.Product, error) {
	args := m.Called(ctx, product)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepository) Update(ctx context.Context, id uuid.UUID, product *models.Product) (*models.Product, error) {
	args := m.Called(ctx, id, product)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Product), args.Error(1)
}
func (m *MockProductRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *MockProductRepository) Search(ctx context.Context, query string, limit, offset int) ([]models.Product, error) {
	args := m.Called(ctx, query, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Product), args.Error(1)
}
func (m *MockProductRepository) UpdateStock(ctx context.Context, id uuid.UUID, stock int) error {
	args := m.Called(ctx, id, stock)
	return args.Error(0)
}
func (m *MockProductRepository) SetCategories(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	return m.Called(ctx, productID, categoryIDs).Error(0)
}

// MockSourceRepository satisfies sourcing.SourceRepository for handler tests that resolve a
// stock source (default or by id).
type MockSourceRepository struct{ mock.Mock }

func (m *MockSourceRepository) GetDefault(ctx context.Context) (*models.Source, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Source), args.Error(1)
}
func (m *MockSourceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Source, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Source), args.Error(1)
}
func (m *MockProductRepository) FindCategoryIDs(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	args := m.Called(ctx, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]uuid.UUID), args.Error(1)
}

type MockCategoryRepository struct{ mock.Mock }

func (m *MockCategoryRepository) FindAll(ctx context.Context) ([]models.Category, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Category), args.Error(1)
}
func (m *MockCategoryRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Category), args.Error(1)
}
func (m *MockCategoryRepository) Create(ctx context.Context, category *models.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}
func (m *MockCategoryRepository) Update(ctx context.Context, category *models.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}
func (m *MockCategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockAddressRepository struct{ mock.Mock }

func (m *MockAddressRepository) Create(ctx context.Context, address *models.Address) (*models.Address, error) {
	args := m.Called(ctx, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Address), args.Error(1)
}
func (m *MockAddressRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]models.Address, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Address), args.Error(1)
}
func (m *MockAddressRepository) FindByUserAndID(ctx context.Context, userID, addressID uuid.UUID) (*models.Address, error) {
	args := m.Called(ctx, userID, addressID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Address), args.Error(1)
}
func (m *MockAddressRepository) Update(ctx context.Context, userID, addressID uuid.UUID, address *models.Address) (*models.Address, error) {
	args := m.Called(ctx, userID, addressID, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Address), args.Error(1)
}
func (m *MockAddressRepository) Delete(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}
func (m *MockAddressRepository) SetDefault(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}
func (m *MockAddressRepository) SetDefaultBilling(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}
func (m *MockAddressRepository) SetDefaultShipping(ctx context.Context, userID, addressID uuid.UUID) error {
	args := m.Called(ctx, userID, addressID)
	return args.Error(0)
}

type MockCartRepository struct{ mock.Mock }

func (m *MockCartRepository) GetOrCreateCartByUserID(ctx context.Context, userID uuid.UUID) (*models.Cart, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Cart), args.Error(1)
}
func (m *MockCartRepository) GetCartItems(ctx context.Context, cartID uuid.UUID) ([]models.CartItem, error) {
	args := m.Called(ctx, cartID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.CartItem), args.Error(1)
}
func (m *MockCartRepository) AddItem(ctx context.Context, cartID, productID, variantID uuid.UUID, quantity int, price int64) (*models.CartItem, error) {
	args := m.Called(ctx, cartID, productID, variantID, quantity, price)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.CartItem), args.Error(1)
}
func (m *MockCartRepository) UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, quantity int) (*models.CartItem, error) {
	args := m.Called(ctx, cartID, productID, quantity)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.CartItem), args.Error(1)
}
func (m *MockCartRepository) RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error {
	args := m.Called(ctx, cartID, productID)
	return args.Error(0)
}
func (m *MockCartRepository) FindCartItem(ctx context.Context, cartID, productID uuid.UUID) (*models.CartItem, error) {
	args := m.Called(ctx, cartID, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.CartItem), args.Error(1)
}
func (m *MockCartRepository) ClearCart(ctx context.Context, cartID uuid.UUID) error {
	args := m.Called(ctx, cartID)
	return args.Error(0)
}
func (m *MockCartRepository) AddCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	args := m.Called(ctx, cartID, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Cart), args.Error(1)
}
func (m *MockCartRepository) RemoveCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	args := m.Called(ctx, cartID, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Cart), args.Error(1)
}

type MockPasswordHasher struct{ mock.Mock }

func (m *MockPasswordHasher) HashPassword(password string) (string, error) {
	args := m.Called(password)
	return args.String(0), args.Error(1)
}
func (m *MockPasswordHasher) CheckPassword(hashedPassword, password string) error {
	args := m.Called(hashedPassword, password)
	return args.Error(0)
}

func setupBaseTest(t *testing.T) (context.Context, *httptest.ResponseRecorder, *mux.Router, *MockUserRepository, *MockProductRepository, *MockCategoryRepository, *MockAddressRepository, *MockCartRepository, string) {
	t.Helper()
	ctx := context.Background()
	rr := httptest.NewRecorder()
	router := mux.NewRouter()

	mockUserRepo := new(MockUserRepository)
	mockProductRepo := new(MockProductRepository)
	mockCategoryRepo := new(MockCategoryRepository)
	mockAddressRepo := new(MockAddressRepository)
	mockCartRepo := new(MockCartRepository)

	testUserID := uuid.New()
	testToken, err := auth.GenerateToken(testUserID, testJwtSecret, time.Hour)
	require.NoError(t, err)

	mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()

	return ctx, rr, router, mockUserRepo, mockProductRepo, mockCategoryRepo, mockAddressRepo, mockCartRepo, testToken
}

func generateTestToken(userID uuid.UUID) (string, error) {
	return auth.GenerateToken(userID, testJwtSecret, time.Hour)
}

func executeRequestAndAssert(t *testing.T, router *mux.Router, req *http.Request, expectedStatus int, expectedBody string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, expectedStatus, rr.Code, "handler returned wrong status code")

	if expectedBody != "" {
		assertJSONContains(t, strings.TrimSpace(expectedBody), strings.TrimSpace(rr.Body.String()))
	}

	return rr
}

// assertJSONContains checks that all fields in expectedJSON exist and match in actualJSON.
// The actual response may contain additional fields — useful when models gain new fields.
func assertJSONContains(t *testing.T, expectedJSON, actualJSON string) {
	t.Helper()

	var expected, actual any
	if err := json.Unmarshal([]byte(expectedJSON), &expected); err != nil {
		// Not JSON — fall back to plain string contains.
		require.Contains(t, actualJSON, expectedJSON)
		return
	}
	require.NoError(t, json.Unmarshal([]byte(actualJSON), &actual), "response is not valid JSON: %s", actualJSON)
	assertJSONSubset(t, expected, actual, "$")
}

func assertJSONSubset(t *testing.T, expected, actual any, path string) {
	t.Helper()
	switch e := expected.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		require.True(t, ok, "%s: expected object, got %T", path, actual)
		for k, ev := range e {
			av, exists := a[k]
			require.True(t, exists, "%s.%s: key not found in actual", path, k)
			assertJSONSubset(t, ev, av, path+"."+k)
		}
	case []any:
		a, ok := actual.([]any)
		require.True(t, ok, "%s: expected array, got %T", path, actual)
		require.Equal(t, len(e), len(a), "%s: array length mismatch", path)
		for i := range e {
			assertJSONSubset(t, e[i], a[i], path+"."+string(rune('0'+i)))
		}
	default:
		// Normalize via JSON round-trip to handle float formatting (10.50 == 10.5).
		eb, _ := json.Marshal(expected)
		ab, _ := json.Marshal(actual)
		require.Equal(t, string(eb), string(ab), "%s: value mismatch", path)
	}
}

func setupDummyDbPool(t *testing.T) *pgxpool.Pool {
	t.Skip("Skipping test requiring DB pool setup")
	return nil
}

func mockReturn[T any](args mock.Arguments, index int) T {
	if args.Get(index) == nil {
		var zero T
		return zero
	}
	return args.Get(index).(T)
}

func mockGetOrCreateCartSuccess(m *MockCartRepository, userID uuid.UUID, cart *models.Cart) {
	m.On("GetOrCreateCartByUserID", mock.Anything, userID).Return(cart, nil).Once()
}
func mockGetOrCreateCartError(m *MockCartRepository, userID uuid.UUID) {
	m.On("GetOrCreateCartByUserID", mock.Anything, userID).Return(nil, assert.AnError).Once()
}
func mockGetCartItemsSuccess(m *MockCartRepository, cartID uuid.UUID, items []models.CartItem) {
	m.On("GetCartItems", mock.Anything, cartID).Return(items, nil).Once()
}
func mockGetCartItemsError(m *MockCartRepository, cartID uuid.UUID) {
	m.On("GetCartItems", mock.Anything, cartID).Return(nil, assert.AnError).Once()
}
func mockFindProductSuccess(m *MockProductRepository, product *models.Product) {
	m.On("FindByID", mock.Anything, product.ID).Return(product, nil).Once()
}
func mockFindProductNotFound(m *MockProductRepository, productID uuid.UUID) {
	m.On("FindByID", mock.Anything, productID).Return(nil, products.ErrProductNotFound).Once()
}
func mockFindProductError(m *MockProductRepository, productID uuid.UUID) {
	m.On("FindByID", mock.Anything, productID).Return(nil, assert.AnError).Once()
}
func mockAddItemSuccess(m *MockCartRepository, cartID, productID, variantID uuid.UUID, quantity int, price int64, item *models.CartItem) {
	m.On("AddItem", mock.Anything, cartID, productID, variantID, quantity, price).Return(item, nil).Once()
}
func mockAddItemError(m *MockCartRepository, cartID, productID, variantID uuid.UUID, quantity int, price int64) {
	m.On("AddItem", mock.Anything, cartID, productID, variantID, quantity, price).Return(nil, assert.AnError).Once()
}
func mockRemoveItemSuccess(m *MockCartRepository, cartID, productID uuid.UUID) {
	m.On("RemoveItem", mock.Anything, cartID, productID).Return(nil).Once()
}
func mockRemoveItemNotFound(m *MockCartRepository, cartID, productID uuid.UUID) {
	m.On("RemoveItem", mock.Anything, cartID, productID).Return(assert.AnError).Once()
}
func mockRemoveItemError(m *MockCartRepository, cartID, productID uuid.UUID) {
	m.On("RemoveItem", mock.Anything, cartID, productID).Return(assert.AnError).Once()
}
func mockUpdateItemQuantitySuccess(m *MockCartRepository, cartID, productID uuid.UUID, quantity int, item *models.CartItem) {
	m.On("UpdateItemQuantity", mock.Anything, cartID, productID, quantity).Return(item, nil).Once()
}
func mockUpdateItemQuantityNotFound(m *MockCartRepository, cartID, productID uuid.UUID, quantity int) {
	m.On("UpdateItemQuantity", mock.Anything, cartID, productID, quantity).Return(nil, assert.AnError).Once()
}
func mockUpdateItemQuantityError(m *MockCartRepository, cartID, productID uuid.UUID, quantity int) {
	m.On("UpdateItemQuantity", mock.Anything, cartID, productID, quantity).Return(nil, assert.AnError).Once()
}
func mockClearCartSuccess(m *MockCartRepository, cartID uuid.UUID) {
	m.On("ClearCart", mock.Anything, cartID).Return(nil).Once()
}
func mockClearCartError(m *MockCartRepository, cartID uuid.UUID) {
	m.On("ClearCart", mock.Anything, cartID).Return(assert.AnError).Once()
}
