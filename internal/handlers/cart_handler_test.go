package handlers_test

import (
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/cart"
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/promotions"
	"bullet-commerce/internal/users"
	"bullet-commerce/internal/variants"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestCartHandler_GetCart tests the GET /api/cart endpoint
func TestCartHandler_GetCart(t *testing.T) {
	testUserID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}
	testItems := []models.CartItem{
		{CartID: testCart.ID, ProductID: uuid.New(), Quantity: 2, PriceCents: 1050},
		{CartID: testCart.ID, ProductID: uuid.New(), Quantity: 1, PriceCents: 2500},
	}

	tests := []struct {
		name                 string
		mocksSetup           func(mockCartRepo *MockCartRepository) // Simplified mock setup
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name: "Success - Existing Cart with Items",
			mocksSetup: func(mockCartRepo *MockCartRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockGetCartItemsSuccess(mockCartRepo, testCart.ID, testItems)
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart":{"id":"%s","user_id":"%s","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},"items":[{"id":"00000000-0000-0000-0000-000000000000","cart_id":"%s","product_id":"%s","quantity":%d,"price_cents":%d,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},{"id":"00000000-0000-0000-0000-000000000000","cart_id":"%s","product_id":"%s","quantity":%d,"price_cents":%d,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}],"total_cents":%d}`, testCart.ID, testUserID, testItems[0].CartID, testItems[0].ProductID, testItems[0].Quantity, testItems[0].PriceCents, testItems[1].CartID, testItems[1].ProductID, testItems[1].Quantity, testItems[1].PriceCents, (testItems[0].PriceCents*int64(testItems[0].Quantity))+(testItems[1].PriceCents*int64(testItems[1].Quantity))),
		},
		{
			name: "Success - New Cart (Empty)",
			mocksSetup: func(mockCartRepo *MockCartRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockGetCartItemsSuccess(mockCartRepo, testCart.ID, []models.CartItem{})
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart": {"id":"%s", "user_id":"%s", "created_at":"0001-01-01T00:00:00Z", "updated_at":"0001-01-01T00:00:00Z"}, "items": [], "total_cents": 0.00}`, testCart.ID, testUserID),
		},
		{
			name: "Error - GetOrCreateCart Fails",
			mocksSetup: func(mockCartRepo *MockCartRepository) {
				mockGetOrCreateCartError(mockCartRepo, testUserID)
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to get or create cart"}`,
		},
		{
			name: "Error - GetCartItems Fails",
			mocksSetup: func(mockCartRepo *MockCartRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockGetCartItemsError(mockCartRepo, testCart.ID)
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to retrieve cart items"}`,
		},
		// Add test case for middleware failure
		{
			name:                 "Error - Middleware User Check Fails",
			mocksSetup:           func(mockCartRepo *MockCartRepository) { /* Cart repo not called */ },
			expectedStatus:       http.StatusUnauthorized,
			expectedBodyContains: `{"error":"user associated with token not found"}`,
		},
		// Add test case for missing token
		{
			name:                 "Error - No Auth Token",
			mocksSetup:           func(mockCartRepo *MockCartRepository) { /* Cart repo not called */ },
			expectedStatus:       http.StatusUnauthorized,
			expectedBodyContains: `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Setup inside t.Run for isolation
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			mockProductRepo := new(MockProductRepository) // Needed for handler instantiation
			cartHandler := handlers.NewCartHandler(mockCartRepo, mockProductRepo, new(variants.MockVariantRepository), promotions.NoopVoucherHandler{})
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			router := mux.NewRouter()
			router.Handle("/api/cart", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.GetCart))).Methods("GET")

			// Conditionally setup user mock for middleware
			if tc.name == "Error - Middleware User Check Fails" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(nil, users.ErrUserNotFound).Once()
			} else if tc.name != "Error - No Auth Token" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe() // Use Maybe for non-error cases
			}

			// Setup CartRepo mocks specific to this test case
			tc.mocksSetup(mockCartRepo)

			// Generate token (or not for the specific test case)
			var currentToken string
			if tc.name != "Error - No Auth Token" {
				var err error
				currentToken, err = generateTestToken(testUserID)
				require.NoError(t, err)
			}

			req, _ := http.NewRequest("GET", "/api/cart", nil)
			if currentToken != "" {
				req.Header.Set("Authorization", "Bearer "+currentToken)
			}

			executeRequestAndAssert(t, router, req, tc.expectedStatus, tc.expectedBodyContains)

			// Assert mocks
			mockCartRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
		})
	}
}

// TestCartHandler_AddItem tests the POST /api/cart/items endpoint
func TestCartHandler_AddItem(t *testing.T) {
	testUserID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}
	productID := uuid.New()
	testProduct := &models.Product{ID: productID, Name: "Test Item", PriceCents: 1999}
	testQuantity := 2
	// The default variant resolves the sellable unit. Price is materialized on the variant
	// (NOT NULL), so the line takes variant.PriceCents directly — here equal to the product's.
	testVariant := models.ProductVariant{ID: uuid.New(), ProductID: productID, SKU: "default-" + productID.String(), PriceCents: testProduct.PriceCents}
	testCartItem := &models.CartItem{CartID: testCart.ID, ProductID: productID, VariantID: testVariant.ID, Quantity: testQuantity, PriceCents: testProduct.PriceCents}

	tests := []struct {
		name                 string
		body                 string
		mocksSetup           func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository)
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name: "Success - Add New Item (default variant)",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockFindProductSuccess(mockProductRepo, testProduct)
				mockVariantRepo.On("FindByProductID", mock.Anything, productID).Return([]models.ProductVariant{testVariant}, nil).Once()
				mockAddItemSuccess(mockCartRepo, testCart.ID, productID, testVariant.ID, testQuantity, testProduct.PriceCents, testCartItem)
			},
			expectedStatus:       http.StatusCreated,
			expectedBodyContains: fmt.Sprintf(`"variant_id":"%s"`, testVariant.ID),
		},
		{
			name: "Success - Add New Item (explicit variant)",
			body: fmt.Sprintf(`{"product_id":"%s", "variant_id":"%s", "quantity":%d}`, productID, testVariant.ID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockFindProductSuccess(mockProductRepo, testProduct)
				mockVariantRepo.On("FindByID", mock.Anything, testVariant.ID).Return(&testVariant, nil).Once()
				mockAddItemSuccess(mockCartRepo, testCart.ID, productID, testVariant.ID, testQuantity, testProduct.PriceCents, testCartItem)
			},
			expectedStatus:       http.StatusCreated,
			expectedBodyContains: fmt.Sprintf(`"variant_id":"%s"`, testVariant.ID),
		},
		{
			name: "Error - Invalid Quantity (Zero)",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":0}`, productID),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				// No product or add item mock needed
			},
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"quantity must be positive"}`,
		},
		{
			name: "Error - Invalid Quantity (Negative)",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":-1}`, productID),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
			},
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"quantity must be positive"}`,
		},
		{
			name: "Error - Product Not Found",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockFindProductNotFound(mockProductRepo, productID)
			},
			expectedStatus:       http.StatusNotFound,
			expectedBodyContains: `{"error":"product not found"}`,
		},
		{
			name: "Error - FindProductByID Fails (Internal Error)",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockFindProductError(mockProductRepo, productID)
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to validate product"}`,
		},
		{
			name: "Error - AddItem Fails",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				mockGetOrCreateCartSuccess(mockCartRepo, testUserID, testCart)
				mockFindProductSuccess(mockProductRepo, testProduct)
				mockVariantRepo.On("FindByProductID", mock.Anything, productID).Return([]models.ProductVariant{testVariant}, nil).Once()
				mockAddItemError(mockCartRepo, testCart.ID, productID, testVariant.ID, testQuantity, testProduct.PriceCents)
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to add item to cart"}`,
		},
		{
			name: "Error - Invalid JSON Body",
			body: `{"product_id": invalid}`, // Malformed JSON
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) {
				// May or may not call GetOrCreateCart depending on when body is parsed
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Maybe()
			},
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"invalid request body"}`,
		},
		{
			name: "Error - Middleware User Check Fails",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) { /* No cart/product mocks needed */
			},
			expectedStatus:       http.StatusUnauthorized,
			expectedBodyContains: `{"error":"user associated with token not found"}`,
		},
		{
			name: "Error - No Auth Token",
			body: fmt.Sprintf(`{"product_id":"%s", "quantity":%d}`, productID, testQuantity),
			mocksSetup: func(mockCartRepo *MockCartRepository, mockProductRepo *MockProductRepository, mockVariantRepo *variants.MockVariantRepository) { /* No cart/product mocks needed */
			},
			expectedStatus:       http.StatusUnauthorized,
			expectedBodyContains: `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Setup inside t.Run
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			mockProductRepo := new(MockProductRepository)
			mockVariantRepo := new(variants.MockVariantRepository)
			cartHandler := handlers.NewCartHandler(mockCartRepo, mockProductRepo, mockVariantRepo, promotions.NoopVoucherHandler{})
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			router := mux.NewRouter()
			router.Handle("/api/cart/items", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.AddItem))).Methods("POST")

			// Conditionally setup user mock for middleware
			if tc.name == "Error - Middleware User Check Fails" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(nil, users.ErrUserNotFound).Once()
			} else if tc.name != "Error - No Auth Token" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()
			}

			// Setup CartRepo, ProductRepo and VariantRepo mocks
			tc.mocksSetup(mockCartRepo, mockProductRepo, mockVariantRepo)

			// Generate token (or not)
			var currentToken string
			if tc.name != "Error - No Auth Token" {
				var err error
				currentToken, err = generateTestToken(testUserID)
				require.NoError(t, err)
			}

			req, _ := http.NewRequest("POST", "/api/cart/items", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if currentToken != "" {
				req.Header.Set("Authorization", "Bearer "+currentToken)
			}

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")

			// Assert body based on expected status
			if tc.expectedStatus == http.StatusCreated {
				assert.Contains(t, rr.Body.String(), tc.expectedBodyContains) // Check if created item is in response
			} else {
				require.JSONEq(t, tc.expectedBodyContains, rr.Body.String()) // Exact match for errors
			}

			// Assert mocks
			mockCartRepo.AssertExpectations(t)
			mockProductRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
		})
	}
}

// TestCartHandler_DeleteItem tests the DELETE /api/cart/items/{variantId} endpoint
func TestCartHandler_DeleteItem(t *testing.T) {
	// Explicitly capture all 9 return values
	_, _, router, baseMockUserRepo, baseMockProductRepo, _, _, baseMockCartRepo, token := setupBaseTest(t)

	// Handler created once
	cartHandler := handlers.NewCartHandler(baseMockCartRepo, baseMockProductRepo, new(variants.MockVariantRepository), promotions.NoopVoucherHandler{})

	claims, err := auth.ValidateToken(token, testJwtSecret)
	require.NoError(t, err)
	testUserID := claims.UserID

	productID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}

	// Route registered once
	router.Handle("/api/cart/items/{variantId}", auth.NewMiddleware(testJwtSecret, baseMockUserRepo).Authenticate(http.HandlerFunc(cartHandler.DeleteItem))).Methods("DELETE")

	tests := []struct {
		name                 string
		productIDParam       string
		mockGetOrCreateCart  func(*MockCartRepository)
		mockRemoveItem       func(*MockCartRepository)
		mockGetCartItems     func(*MockCartRepository) // For the final GetCart call
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name:           "Success",
			productIDParam: productID.String(),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				// Expect two calls because handler calls it, then calls GetCart which calls it again
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Twice()
			},
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(nil).Once()
			},
			mockGetCartItems: func(mockCartRepo *MockCartRepository) {
				// Simulate cart being empty after removal
				mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return([]models.CartItem{}, nil).Once()
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart":{"id":"%s","user_id":"%s","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},"items":[],"total_cents":0}`, testCart.ID, testUserID),
		},
		{
			name:           "Product Not Found in Cart",
			productIDParam: productID.String(),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(cart.ErrProductNotInCart).Once()
			},
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusNotFound,
			expectedBodyContains: `{"error":"product not found in cart"}`,
		},
		{
			name:           "Invalid Variant ID Format",
			productIDParam: "invalid-uuid",
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"invalid variant ID format"}`,
		},
		{
			name:           "GetOrCreateCart Fails",
			productIDParam: productID.String(),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(nil, assert.AnError).Once()
			},
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to get or create cart"}`,
		},
		{
			name:           "RemoveItem Fails (Internal Error)",
			productIDParam: productID.String(),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(assert.AnError).Once()
			},
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to remove cart item"}`,
		},
		{
			name:           "GetCartItems Fails After Successful Remove",
			productIDParam: productID.String(),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				// Expect two calls, same as success case for RemoveItem
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Twice()
			},
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(nil).Once()
			},
			mockGetCartItems: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return(nil, assert.AnError).Once()
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to retrieve cart items"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Fresh mocks for subtest
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			cartHandler.CartRepo = mockCartRepo // Update handler repo

			// New middleware
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()

			// Setup specific mocks
			tc.mockGetOrCreateCart(mockCartRepo)
			tc.mockRemoveItem(mockCartRepo)
			tc.mockGetCartItems(mockCartRepo)

			url := fmt.Sprintf("/api/cart/items/%s", tc.productIDParam)
			req, _ := http.NewRequest("DELETE", url, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			// Re-register route with new middleware
			subRouter := mux.NewRouter()
			subRouter.Handle("/api/cart/items/{variantId}", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.DeleteItem))).Methods("DELETE")

			executeRequestAndAssert(t, subRouter, req, tc.expectedStatus, tc.expectedBodyContains)

			mockCartRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
		})
	}
}

// TestCartHandler_UpdateItem tests the PUT /api/cart/items/{variantId} endpoint
func TestCartHandler_UpdateItem(t *testing.T) {
	// Explicitly capture all 9 return values
	_, _, router, baseMockUserRepo, baseMockProductRepo, _, _, baseMockCartRepo, token := setupBaseTest(t)

	// Handler created once
	cartHandler := handlers.NewCartHandler(baseMockCartRepo, baseMockProductRepo, new(variants.MockVariantRepository), promotions.NoopVoucherHandler{})

	claims, err := auth.ValidateToken(token, testJwtSecret)
	require.NoError(t, err)
	testUserID := claims.UserID

	productID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}
	updatedQuantity := 5
	updatedItem := &models.CartItem{CartID: testCart.ID, ProductID: productID, Quantity: updatedQuantity, PriceCents: 1500}

	// Route registered once
	router.Handle("/api/cart/items/{variantId}", auth.NewMiddleware(testJwtSecret, baseMockUserRepo).Authenticate(http.HandlerFunc(cartHandler.UpdateItem))).Methods("PUT")

	tests := []struct {
		name                 string
		productIDParam       string
		body                 string
		mockGetOrCreateCart  func(*MockCartRepository)
		mockUpdateQuantity   func(*MockCartRepository)
		mockRemoveItem       func(*MockCartRepository) // For quantity <= 0 case
		mockGetCartItems     func(*MockCartRepository) // For the final GetCart call
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name:           "Success",
			productIDParam: productID.String(),
			body:           fmt.Sprintf(`{"quantity": %d}`, updatedQuantity),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				// Expect two calls: one at the start, one inside the final GetCart call
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Twice()
			},
			mockUpdateQuantity: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("UpdateItemQuantity", mock.Anything, testCart.ID, productID, updatedQuantity).Return(updatedItem, nil).Once()
			},
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {}, // Not called
			mockGetCartItems: func(mockCartRepo *MockCartRepository) {
				// Simulate cart having the updated item
				mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return([]models.CartItem{*updatedItem}, nil).Once()
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart":{"id":"%s","user_id":"%s","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},"items":[%s],"total_cents":%d}`, testCart.ID, testUserID, fmt.Sprintf(`{"id":"00000000-0000-0000-0000-000000000000","cart_id":"%s","product_id":"%s","quantity":%d,"price_cents":%d,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}`, updatedItem.CartID, updatedItem.ProductID, updatedItem.Quantity, updatedItem.PriceCents), int64(updatedItem.Quantity)*updatedItem.PriceCents),
		},
		{
			name:           "Quantity Zero (Triggers Delete)",
			productIDParam: productID.String(),
			body:           `{"quantity": 0}`,
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				// Expect THREE calls: UpdateItem -> DeleteItem -> GetCart
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Times(3)
			},
			mockUpdateQuantity: func(mockCartRepo *MockCartRepository) {}, // Not called directly
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(nil).Once()
			},
			mockGetCartItems: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return([]models.CartItem{}, nil).Once()
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart":{"id":"%s","user_id":"%s","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},"items":[],"total_cents":0}`, testCart.ID, testUserID),
		},
		{
			name:           "Product Not Found in Cart",
			productIDParam: productID.String(),
			body:           fmt.Sprintf(`{"quantity": %d}`, updatedQuantity),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockUpdateQuantity: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("UpdateItemQuantity", mock.Anything, testCart.ID, productID, updatedQuantity).Return(nil, cart.ErrProductNotInCart).Once()
			},
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) {}, // Not called
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) {}, // Not called
			expectedStatus:       http.StatusNotFound,
			expectedBodyContains: `{"error":"product not found in cart"}`,
		},
		{
			name:           "Invalid Quantity (Negative Triggers Delete)",
			productIDParam: productID.String(),
			body:           `{"quantity": -1}`,
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				// Expect THREE calls: UpdateItem -> DeleteItem -> GetCart
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Times(3)
			},
			mockUpdateQuantity: func(mockCartRepo *MockCartRepository) {}, // Not called directly
			mockRemoveItem: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("RemoveItem", mock.Anything, testCart.ID, productID).Return(nil).Once()
			},
			mockGetCartItems: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return([]models.CartItem{}, nil).Once()
			},
			expectedStatus:       http.StatusOK,
			expectedBodyContains: fmt.Sprintf(`{"cart":{"id":"%s","user_id":"%s","created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},"items":[],"total_cents":0}`, testCart.ID, testUserID),
		},
		{
			name:           "Invalid JSON Body",
			productIDParam: productID.String(),
			body:           `{"quantity": "not-a-number"}`,
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Maybe()
			},
			mockUpdateQuantity:   func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"invalid request body"}`,
		},
		{
			name:           "Invalid Variant ID Format",
			productIDParam: "invalid-uuid",
			body:           fmt.Sprintf(`{"quantity": %d}`, updatedQuantity),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockUpdateQuantity:   func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusBadRequest,
			expectedBodyContains: `{"error":"invalid variant ID format"}`,
		},
		{
			name:           "GetOrCreateCart Fails",
			productIDParam: productID.String(),
			body:           fmt.Sprintf(`{"quantity": %d}`, updatedQuantity),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(nil, assert.AnError).Once()
			},
			mockUpdateQuantity:   func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) { /* Not called */ },
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to get or create cart"}`,
		},
		{
			name:           "UpdateItemQuantity Fails (Internal Error)",
			productIDParam: productID.String(),
			body:           fmt.Sprintf(`{"quantity": %d}`, updatedQuantity),
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockUpdateQuantity: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("UpdateItemQuantity", mock.Anything, testCart.ID, productID, updatedQuantity).Return(nil, assert.AnError).Once()
			},
			mockRemoveItem:       func(mockCartRepo *MockCartRepository) {}, // Not called
			mockGetCartItems:     func(mockCartRepo *MockCartRepository) {}, // Not called
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to update cart item"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Fresh mocks
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			cartHandler.CartRepo = mockCartRepo // Update handler repo

			// New middleware
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()

			// Setup specific mocks
			tc.mockGetOrCreateCart(mockCartRepo)
			tc.mockUpdateQuantity(mockCartRepo)
			tc.mockRemoveItem(mockCartRepo)
			tc.mockGetCartItems(mockCartRepo)

			url := fmt.Sprintf("/api/cart/items/%s", tc.productIDParam)
			req, _ := http.NewRequest("PUT", url, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			// Re-register route
			subRouter := mux.NewRouter()
			subRouter.Handle("/api/cart/items/{variantId}", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.UpdateItem))).Methods("PUT")

			executeRequestAndAssert(t, subRouter, req, tc.expectedStatus, tc.expectedBodyContains)

			mockCartRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
		})
	}
}

// TestCartHandler_ClearCart tests the DELETE /api/cart endpoint
func TestCartHandler_ClearCart(t *testing.T) {
	// Explicitly capture all 9 return values
	_, _, router, baseMockUserRepo, baseMockProductRepo, _, _, baseMockCartRepo, token := setupBaseTest(t)

	// Handler created once
	cartHandler := handlers.NewCartHandler(baseMockCartRepo, baseMockProductRepo, new(variants.MockVariantRepository), promotions.NoopVoucherHandler{})

	claims, err := auth.ValidateToken(token, testJwtSecret)
	require.NoError(t, err)
	testUserID := claims.UserID
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}

	// Route registered once
	router.Handle("/api/cart", auth.NewMiddleware(testJwtSecret, baseMockUserRepo).Authenticate(http.HandlerFunc(cartHandler.ClearCart))).Methods("DELETE")

	tests := []struct {
		name                 string
		mockGetOrCreateCart  func(*MockCartRepository)
		mockClearCart        func(*MockCartRepository)
		expectedStatus       int
		expectedBodyContains string // Should be empty for 204
	}{
		{
			name: "Success",
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockClearCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("ClearCart", mock.Anything, testCart.ID).Return(nil).Once()
			},
			expectedStatus:       http.StatusNoContent,
			expectedBodyContains: "",
		},
		{
			name: "GetOrCreateCart Fails",
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(nil, assert.AnError).Once()
			},
			mockClearCart:        func(mockCartRepo *MockCartRepository) { /* Not called */ },
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to get or create cart"}`,
		},
		{
			name: "ClearCart Fails",
			mockGetOrCreateCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
			},
			mockClearCart: func(mockCartRepo *MockCartRepository) {
				mockCartRepo.On("ClearCart", mock.Anything, testCart.ID).Return(assert.AnError).Once()
			},
			expectedStatus:       http.StatusInternalServerError,
			expectedBodyContains: `{"error":"failed to clear cart"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Fresh mocks
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			cartHandler.CartRepo = mockCartRepo // Update handler repo

			// New middleware
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()

			// Setup specific mocks
			tc.mockGetOrCreateCart(mockCartRepo)
			tc.mockClearCart(mockCartRepo)

			req, _ := http.NewRequest("DELETE", "/api/cart", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			// Re-register route
			subRouter := mux.NewRouter()
			subRouter.Handle("/api/cart", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.ClearCart))).Methods("DELETE")

			executeRequestAndAssert(t, subRouter, req, tc.expectedStatus, tc.expectedBodyContains)

			mockCartRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
		})
	}
}

// --- Coupon endpoints & discount pricing ---

// couponVoucher builds a real CouponHandler backed by a mock coupon repo returning the
// given coupon for the given code, so the handler exercises the real pricing path.
func couponVoucher(code string, coupon *models.Coupon) *promotions.CouponHandler {
	repo := &coupons.MockCouponRepository{}
	if coupon != nil {
		repo.On("FindByCode", mock.Anything, code).Return(coupon, nil)
	} else {
		repo.On("FindByCode", mock.Anything, code).Return(nil, coupons.ErrCouponNotFound)
	}
	return promotions.NewCouponHandler(repo)
}

// TestCartHandler_GetCart_WithDiscount pins the accept criterion: a 10%-off coupon on a
// 10000-cent cart yields discount_cents 1000 and total_cents 9000.
func TestCartHandler_GetCart_WithDiscount(t *testing.T) {
	testUserID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID, AppliedCouponCodes: []string{"SAVE10"}}
	items := []models.CartItem{{CartID: testCart.ID, ProductID: uuid.New(), Quantity: 1, PriceCents: 10000}}
	coupon := &models.Coupon{ID: uuid.New(), Code: "SAVE10", DiscountType: models.CouponPercent, Value: 10, Active: true}

	mockUserRepo := new(MockUserRepository)
	mockCartRepo := new(MockCartRepository)
	cartHandler := handlers.NewCartHandler(mockCartRepo, new(MockProductRepository), new(variants.MockVariantRepository), couponVoucher("SAVE10", coupon))
	authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
	router := mux.NewRouter()
	router.Handle("/api/cart", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.GetCart))).Methods("GET")

	mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()
	mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil).Once()
	mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return(items, nil).Once()

	token, err := generateTestToken(testUserID)
	require.NoError(t, err)
	req, _ := http.NewRequest("GET", "/api/cart", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := executeRequestAndAssert(t, router, req, http.StatusOK, `{"discount_cents":1000,"total_cents":9000}`)
	assert.Contains(t, rr.Body.String(), `"discount_cents":1000`)
	mockCartRepo.AssertExpectations(t)
}

func TestCartHandler_AddCoupon(t *testing.T) {
	testUserID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID}
	items := []models.CartItem{{CartID: testCart.ID, ProductID: uuid.New(), Quantity: 1, PriceCents: 10000}}

	tests := []struct {
		name           string
		body           string
		coupon         *models.Coupon
		code           string
		expectPersist  bool
		expectedStatus int
	}{
		{
			name:           "Success",
			body:           `{"code":"SAVE10"}`,
			coupon:         &models.Coupon{ID: uuid.New(), Code: "SAVE10", DiscountType: models.CouponPercent, Value: 10, Active: true},
			code:           "SAVE10",
			expectPersist:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid coupon rejected",
			body:           `{"code":"NOPE"}`,
			coupon:         nil, // FindByCode → ErrCouponNotFound
			code:           "NOPE",
			expectPersist:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty code",
			body:           `{"code":""}`,
			coupon:         nil,
			code:           "",
			expectPersist:  false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo := new(MockUserRepository)
			mockCartRepo := new(MockCartRepository)
			cartHandler := handlers.NewCartHandler(mockCartRepo, new(MockProductRepository), new(variants.MockVariantRepository), couponVoucher(tc.code, tc.coupon))
			authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
			router := mux.NewRouter()
			router.Handle("/api/cart/coupon", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.AddCoupon))).Methods("POST")

			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()
			// GetOrCreateCart is hit once by AddCoupon, and again by GetCart on success.
			mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil)
			// AddCoupon reads items to price the code; GetCart reads them again on success.
			mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return(items, nil)
			if tc.expectPersist {
				updated := &models.Cart{ID: testCart.ID, UserID: testUserID, AppliedCouponCodes: []string{tc.code}}
				mockCartRepo.On("AddCouponCode", mock.Anything, testCart.ID, tc.code).Return(updated, nil).Once()
			}

			token, err := generateTestToken(testUserID)
			require.NoError(t, err)
			req, _ := http.NewRequest("POST", "/api/cart/coupon", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			executeRequestAndAssert(t, router, req, tc.expectedStatus, "")
			if tc.expectPersist {
				mockCartRepo.AssertCalled(t, "AddCouponCode", mock.Anything, testCart.ID, tc.code)
			} else {
				mockCartRepo.AssertNotCalled(t, "AddCouponCode", mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestCartHandler_RemoveCoupon(t *testing.T) {
	testUserID := uuid.New()
	testCart := &models.Cart{ID: uuid.New(), UserID: testUserID, AppliedCouponCodes: []string{"SAVE10"}}

	mockUserRepo := new(MockUserRepository)
	mockCartRepo := new(MockCartRepository)
	cartHandler := handlers.NewCartHandler(mockCartRepo, new(MockProductRepository), new(variants.MockVariantRepository), promotions.NoopVoucherHandler{})
	authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)
	router := mux.NewRouter()
	router.Handle("/api/cart/coupon/{code}", authMiddleware.Authenticate(http.HandlerFunc(cartHandler.RemoveCoupon))).Methods("DELETE")

	mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(&models.User{ID: testUserID}, nil).Maybe()
	mockCartRepo.On("GetOrCreateCartByUserID", mock.Anything, testUserID).Return(testCart, nil)
	emptied := &models.Cart{ID: testCart.ID, UserID: testUserID}
	mockCartRepo.On("RemoveCouponCode", mock.Anything, testCart.ID, "SAVE10").Return(emptied, nil).Once()
	mockCartRepo.On("GetCartItems", mock.Anything, testCart.ID).Return([]models.CartItem{}, nil)

	token, err := generateTestToken(testUserID)
	require.NoError(t, err)
	req, _ := http.NewRequest("DELETE", "/api/cart/coupon/SAVE10", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	executeRequestAndAssert(t, router, req, http.StatusOK, "")
	mockCartRepo.AssertCalled(t, "RemoveCouponCode", mock.Anything, testCart.ID, "SAVE10")
}
