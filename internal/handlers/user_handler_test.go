package handlers_test

import (
	"bullet-commerce/internal/addresses"
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"bullet-commerce/internal/auth"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// setupUserHandlerTest creates mocks, handler, middleware, and router for user/address tests.
func setupUserHandlerTest(t *testing.T) (*MockUserRepository, *MockAddressRepository, *handlers.UserHandler, *mux.Router) {
	t.Helper()
	_, _, router, mockUserRepo, _, _, mockAddressRepo, _, _ := setupBaseTest(t)

	userHandler := handlers.NewUserHandler(mockUserRepo, mockAddressRepo)

	// Need to instantiate authMiddleware here as it's used for route protection
	authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)

	apiV1 := router.PathPrefix("/api").Subrouter()
	userRoutes := apiV1.PathPrefix("/users").Subrouter()
	userRoutes.Use(authMiddleware.Authenticate)

	userRoutes.HandleFunc("/me", userHandler.GetMe).Methods("GET")

	addressRoutes := userRoutes.PathPrefix("/{userId:[0-9a-fA-F-]+}/addresses").Subrouter()
	addressRoutes.HandleFunc("", userHandler.ListAddresses).Methods("GET")
	addressRoutes.HandleFunc("", userHandler.AddAddress).Methods("POST")
	addressRoutes.HandleFunc("/{addressId:[0-9a-fA-F-]+}", userHandler.UpdateAddress).Methods("PUT")
	addressRoutes.HandleFunc("/{addressId:[0-9a-fA-F-]+}", userHandler.DeleteAddress).Methods("DELETE")
	addressRoutes.HandleFunc("/{addressId:[0-9a-fA-F-]+}/default", userHandler.SetDefaultAddress).Methods("POST")
	addressRoutes.HandleFunc("/{addressId:[0-9a-fA-F-]+}/default/billing", userHandler.SetDefaultBillingAddress).Methods("POST")
	addressRoutes.HandleFunc("/{addressId:[0-9a-fA-F-]+}/default/shipping", userHandler.SetDefaultShippingAddress).Methods("POST")

	return mockUserRepo, mockAddressRepo, userHandler, router
}

func TestUserHandler_GetMe(t *testing.T) {
	testUserID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err, "Failed to generate test token")
	foundUser := &models.User{ID: testUserID, Name: "Test User", Email: "test@example.com", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	tests := []struct {
		name              string
		mockUserReturnMid *models.User
		mockUserErrMid    error
		mockUserReturnHnd *models.User
		mockUserErrHnd    error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success",
			mockUserReturnMid: foundUser,
			mockUserErrMid:    nil,
			mockUserReturnHnd: foundUser,
			mockUserErrHnd:    nil,
			expectedStatus:    http.StatusOK,
			expectedBody:      fmt.Sprintf(`{"id":"%s","name":"%s","email":"%s","created_at":"%s","updated_at":"%s"}`, foundUser.ID, foundUser.Name, foundUser.Email, foundUser.CreatedAt.Format(time.RFC3339Nano), foundUser.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name:              "Failure - Handler Repo Error",
			mockUserReturnMid: foundUser,
			mockUserErrMid:    nil,
			mockUserReturnHnd: nil,
			mockUserErrHnd:    assert.AnError,
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to retrieve user data"}`,
		},
		{
			name:              "Failure - Middleware User Check Fails",
			mockUserReturnMid: nil,
			mockUserErrMid:    users.ErrUserNotFound,
			mockUserReturnHnd: nil,
			mockUserErrHnd:    nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"user associated with token not found"}`,
		},
		{
			name:              "Failure - No Auth Token",
			mockUserReturnMid: nil,
			mockUserErrMid:    nil,
			mockUserReturnHnd: nil,
			mockUserErrHnd:    nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, _, _, router := setupUserHandlerTest(t)

			if tc.expectedBody != `{"error":"authorization header required"}` {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			if tc.mockUserErrMid == nil && tc.expectedBody != `{"error":"authorization header required"}` {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnHnd, tc.mockUserErrHnd).Once()
			}

			req, _ := http.NewRequest(http.MethodGet, "/api/users/me", nil)
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			executeRequestAndAssert(t, router, req, tc.expectedStatus, tc.expectedBody)

			mockUserRepo.AssertExpectations(t)
		})
	}
}

func TestUserHandler_ListAddresses(t *testing.T) { // NOSONAR table-driven test
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err, "Failed to generate test token")
	userForToken := &models.User{ID: testUserID}

	addresses := []models.Address{
		{ID: uuid.New(), UserID: testUserID, Street: "123 Main St", City: "Anytown", PostalCode: "12345", IsDefault: true},
		{ID: uuid.New(), UserID: testUserID, Street: "456 Side St", City: "Anytown", PostalCode: "67890", IsDefault: false},
	}

	tests := []struct {
		name              string
		targetUserIDStr   string
		mockUserReturnMid *models.User
		mockUserErrMid    error
		mockAddrReturn    []models.Address
		mockAddrErr       error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success - List Own Addresses",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrReturn:    addresses,
			mockAddrErr:       nil,
			expectedStatus:    http.StatusOK,
			expectedBody:      fmt.Sprintf(`[{"id":"%s","user_id":"%s","street":"%s","city":"%s","state":"","postal_code":"%s","country":"","is_default":%t,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"},{"id":"%s","user_id":"%s","street":"%s","city":"%s","state":"","postal_code":"%s","country":"","is_default":%t,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}]`, addresses[0].ID, addresses[0].UserID, addresses[0].Street, addresses[0].City, addresses[0].PostalCode, addresses[0].IsDefault, addresses[1].ID, addresses[1].UserID, addresses[1].Street, addresses[1].City, addresses[1].PostalCode, addresses[1].IsDefault),
		},
		{
			name:              "Success - List Own Addresses (Empty)",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrReturn:    []models.Address{},
			mockAddrErr:       nil,
			expectedStatus:    http.StatusOK,
			expectedBody:      `[]`,
		},
		{
			name:              "Failure - Repository Error",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrReturn:    nil,
			mockAddrErr:       assert.AnError,
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to retrieve addresses"}`,
		},
		{
			name:              "Failure - Unauthorized (Different User ID)",
			targetUserIDStr:   anotherUserID.String(),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrReturn:    nil,
			mockAddrErr:       nil,
			expectedStatus:    http.StatusForbidden,
			expectedBody:      `{"error":"forbidden"}`,
		},
		{
			name:              "Failure - Invalid User ID in URL",
			targetUserIDStr:   "not-a-uuid",
			mockUserReturnMid: nil,
			mockUserErrMid:    nil,
			mockAddrReturn:    nil,
			mockAddrErr:       nil,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      "404 page not found",
		},
		{
			name:              "Failure - Middleware User Check Fails",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: nil,
			mockUserErrMid:    users.ErrUserNotFound,
			mockAddrReturn:    nil,
			mockAddrErr:       nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"user associated with token not found"}`,
		},
		{
			name:              "Failure - No Auth Token",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: nil,
			mockUserErrMid:    nil,
			mockAddrReturn:    nil,
			mockAddrErr:       nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			if tc.expectedBody != `{"error":"authorization header required"}` && tc.name != "Failure - Invalid User ID in URL" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			if tc.mockUserErrMid == nil && tc.expectedStatus == http.StatusOK || (tc.expectedStatus == http.StatusInternalServerError && tc.mockAddrErr != nil) {
				targetUUID, err := uuid.Parse(tc.targetUserIDStr)
				if err == nil && targetUUID == testUserID {
					mockAddressRepo.On("FindByUserID", mock.Anything, targetUUID).Return(tc.mockAddrReturn, tc.mockAddrErr).Once()
				}
			}

			url := fmt.Sprintf("/api/users/%s/addresses", tc.targetUserIDStr)
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			executeRequestAndAssert(t, router, req, tc.expectedStatus, tc.expectedBody)

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

func TestUserHandler_AddAddress(t *testing.T) { // NOSONAR table-driven test
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	newAddress := models.Address{
		Street:     "789 New Ave",
		City:       "Newville",
		State:      "NS",
		PostalCode: "98765",
		Country:    "NC",
	}
	bodyJSON, _ := json.Marshal(newAddress)

	// Define the expected created address, simulating repo response
	createdAddress := &models.Address{
		ID:         uuid.New(), // Simulate generated ID
		UserID:     testUserID, // Should match the authorized user
		Street:     newAddress.Street,
		City:       newAddress.City,
		State:      newAddress.State,
		PostalCode: newAddress.PostalCode,
		Country:    newAddress.Country,
		IsDefault:  false,      // Assuming default is false unless specified
		CreatedAt:  time.Now(), // Simulate timestamp
		UpdatedAt:  time.Now(), // Simulate timestamp
	}

	tests := []struct {
		name              string
		targetUserIDStr   string
		body              string
		mockUserReturnMid *models.User
		mockUserErrMid    error
		mockAddrCreateErr error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success - Add Address for Self",
			targetUserIDStr:   testUserID.String(),
			body:              string(bodyJSON),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusCreated,
			expectedBody:      fmt.Sprintf(`{"id":"%s","user_id":"%s","street":"%s","city":"%s","state":"%s","postal_code":"%s","country":"%s","is_default":%t,"created_at":"%s","updated_at":"%s"}`, createdAddress.ID, createdAddress.UserID, createdAddress.Street, createdAddress.City, createdAddress.State, createdAddress.PostalCode, createdAddress.Country, createdAddress.IsDefault, createdAddress.CreatedAt.Format(time.RFC3339Nano), createdAddress.UpdatedAt.Format(time.RFC3339Nano)),
		},
		{
			name:              "Failure - Add Address for Another User",
			targetUserIDStr:   anotherUserID.String(),
			body:              string(bodyJSON),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusForbidden,
			expectedBody:      `{"error":"forbidden"}`,
		},
		{
			name:              "Failure - Invalid User ID in URL",
			targetUserIDStr:   "not-a-uuid",
			body:              string(bodyJSON),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      "404 page not found",
		},
		{
			name:              "Failure - Invalid JSON Body",
			targetUserIDStr:   testUserID.String(),
			body:              `{"street":"bad json"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      `{"error":"all address fields are required"}`,
		},
		{
			name:              "Failure - Missing Required Field (Street)",
			targetUserIDStr:   testUserID.String(),
			body:              `{"city":"Newville","state":"NS","postal_code":"98765","country":"NC"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      `{"error":"all address fields are required"}`,
		},
		{
			name:              "Failure - Repository Create Error",
			targetUserIDStr:   testUserID.String(),
			body:              string(bodyJSON),
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockAddrCreateErr: assert.AnError,
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to create address"}`,
		},
		{
			name:              "Failure - Middleware User Check Fails",
			targetUserIDStr:   testUserID.String(),
			body:              string(bodyJSON),
			mockUserReturnMid: nil,
			mockUserErrMid:    users.ErrUserNotFound,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"user associated with token not found"}`,
		},
		{
			name:              "Failure - No Auth Token",
			targetUserIDStr:   testUserID.String(),
			body:              string(bodyJSON),
			mockUserReturnMid: nil,
			mockUserErrMid:    nil,
			mockAddrCreateErr: nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			if tc.expectedBody != `{"error":"authorization header required"}` && tc.targetUserIDStr != "not-a-uuid" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			// Conditionally mock address repo create call
			// Skip if middleware fails, forbidden, bad request, invalid URL, or no auth token
			shouldMockCreate := tc.mockUserErrMid == nil &&
				tc.expectedStatus != http.StatusForbidden &&
				tc.expectedStatus != http.StatusBadRequest &&
				tc.targetUserIDStr != "not-a-uuid" &&
				tc.expectedBody != `{"error":"authorization header required"}`

			if shouldMockCreate {
				mockAddressRepo.On("Create", mock.Anything, mock.MatchedBy(func(addr *models.Address) bool {
					return addr.Street == newAddress.Street && addr.City == newAddress.City && addr.UserID == testUserID && addr.PostalCode == newAddress.PostalCode
				})).Return(createdAddress, tc.mockAddrCreateErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses", tc.targetUserIDStr)
			req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(tc.body))
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")

			if tc.expectedStatus == http.StatusCreated {
				var createdAddr models.Address
				err := json.Unmarshal(rr.Body.Bytes(), &createdAddr)
				require.NoError(t, err, "Failed to unmarshal created address")
				assert.Equal(t, createdAddress.ID, createdAddr.ID)
				assert.Equal(t, newAddress.Street, createdAddr.Street)
				assert.Equal(t, newAddress.City, createdAddr.City)
				assert.Equal(t, newAddress.State, createdAddr.State)
				assert.Equal(t, newAddress.PostalCode, createdAddr.PostalCode)
				assert.Equal(t, newAddress.Country, createdAddr.Country)
				assert.Equal(t, testUserID, createdAddr.UserID)
				assert.NotEqual(t, uuid.Nil, createdAddr.ID)
				assert.False(t, createdAddr.CreatedAt.IsZero())
				assert.False(t, createdAddr.UpdatedAt.IsZero())
			} else if tc.targetUserIDStr == "not-a-uuid" {
				// For router 404, check plain text body
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			} else {
				// For other errors, expect JSON
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

func TestUserHandler_UpdateAddress(t *testing.T) { // NOSONAR table-driven test
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	addressID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	updateData := models.Address{
		Street:     "987 Updated Ln",
		City:       "Updateville",
		State:      "UP",
		PostalCode: "54321",
		Country:    "UC",
	}
	bodyJSON, _ := json.Marshal(updateData)

	tests := []struct {
		name               string
		targetUserIDStr    string
		targetAddressIDStr string
		body               string
		mockUserReturnMid  *models.User
		mockUserErrMid     error
		mockAddrUpdateErr  error
		expectedStatus     int
		expectedBody       string
	}{
		{
			name:               "Success - Update Own Address",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusOK,
			expectedBody:       `{"street":"987 Updated Ln","city":"Updateville","state":"UP","postal_code":"54321","country":"UC"}`,
		},
		{
			name:               "Failure - Update Another User's Address",
			targetUserIDStr:    anotherUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusForbidden,
			expectedBody:       `{"error":"forbidden"}`,
		},
		{
			name:               "Failure - Invalid User ID in URL",
			targetUserIDStr:    "not-a-uuid",
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Invalid Address ID in URL",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: "not-a-uuid",
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Invalid JSON Body",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               `{"street:"bad json"}`,
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusBadRequest,
			expectedBody:       `{"error":"invalid request body"}`,
		},
		{
			name:               "Failure - Missing Required Field (City)",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               `{"street":"987 Updated Ln","state":"UP","postal_code":"54321","country":"UC"}`,
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusBadRequest,
			expectedBody:       `{"error":"all address fields are required"}`,
		},
		{
			name:               "Failure - Address Not Found",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  addresses.ErrAddressNotFound,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       `{"error":"address not found"}`,
		},
		{
			name:               "Failure - Repository Update Error",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  assert.AnError,
			expectedStatus:     http.StatusInternalServerError,
			expectedBody:       `{"error":"failed to update address"}`,
		},
		{
			name:               "Failure - Middleware User Check Fails",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  nil,
			mockUserErrMid:     users.ErrUserNotFound,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"user associated with token not found"}`,
		},
		{
			name:               "Failure - No Auth Token",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			body:               string(bodyJSON),
			mockUserReturnMid:  nil,
			mockUserErrMid:     nil,
			mockAddrUpdateErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			// Conditionally mock middleware user check
			// Skip mocking if token is absent OR if URL IDs are invalid
			if tc.expectedBody != `{"error":"authorization header required"}` && tc.targetUserIDStr != "not-a-uuid" && tc.targetAddressIDStr != "not-a-uuid" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			shouldMockUpdate := tc.mockUserErrMid == nil &&
				tc.expectedStatus != http.StatusForbidden &&
				tc.expectedStatus != http.StatusBadRequest &&
				tc.targetUserIDStr != "not-a-uuid" &&
				tc.targetAddressIDStr != "not-a-uuid" &&
				tc.expectedBody != `{"error":"authorization header required"}`

			// Define updatedAddress just before the mock setup that uses it
			var updatedAddress *models.Address
			if shouldMockUpdate {
				targetAddrUUID, _ := uuid.Parse(tc.targetAddressIDStr)
				updatedAddress = &models.Address{
					ID:         targetAddrUUID,
					UserID:     testUserID,
					Street:     updateData.Street,
					City:       updateData.City,
					State:      updateData.State,
					PostalCode: updateData.PostalCode,
					Country:    updateData.Country,
					IsDefault:  false,
					UpdatedAt:  time.Now(), // Simulate update time
				}

				mockAddressRepo.On("Update", mock.Anything, testUserID, targetAddrUUID, mock.MatchedBy(func(addr *models.Address) bool {
					return addr.Street == updateData.Street &&
						addr.City == updateData.City &&
						addr.State == updateData.State &&
						addr.PostalCode == updateData.PostalCode &&
						addr.Country == updateData.Country
				})).Return(updatedAddress, tc.mockAddrUpdateErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses/%s", tc.targetUserIDStr, tc.targetAddressIDStr)
			req, _ := http.NewRequest(http.MethodPut, url, bytes.NewBufferString(tc.body))
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")

			if tc.expectedStatus == http.StatusOK {
				var updatedAddr models.Address
				err := json.Unmarshal(rr.Body.Bytes(), &updatedAddr)
				require.NoError(t, err)
				assert.Equal(t, updateData.Street, updatedAddr.Street)
				assert.Equal(t, updateData.City, updatedAddr.City)
				assert.Equal(t, updateData.State, updatedAddr.State)
				assert.Equal(t, updateData.PostalCode, updatedAddr.PostalCode)
				assert.Equal(t, updateData.Country, updatedAddr.Country)
				assert.Equal(t, addressID, updatedAddr.ID)
				assert.Equal(t, testUserID, updatedAddr.UserID)
				assert.False(t, updatedAddr.UpdatedAt.IsZero())
			} else if tc.targetUserIDStr == "not-a-uuid" || tc.targetAddressIDStr == "not-a-uuid" {
				// For router 404s, check plain text body
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			} else {
				// For other errors, expect JSON
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

func TestUserHandler_DeleteAddress(t *testing.T) { // NOSONAR table-driven test
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	addressID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	tests := []struct {
		name               string
		targetUserIDStr    string
		targetAddressIDStr string
		mockUserReturnMid  *models.User
		mockUserErrMid     error
		mockAddrDeleteErr  error
		expectedStatus     int
		expectedBody       string
	}{
		{
			name:               "Success - Delete Own Address",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusNoContent,
			expectedBody:       "",
		},
		{
			name:               "Failure - Delete Another User's Address",
			targetUserIDStr:    anotherUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusForbidden,
			expectedBody:       `{"error":"forbidden"}`,
		},
		{
			name:               "Failure - Invalid User ID in URL",
			targetUserIDStr:    "not-a-uuid",
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Invalid Address ID in URL",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: "not-a-uuid",
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Address Not Found",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  addresses.ErrAddressNotFound,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       `{"error":"address not found"}`,
		},
		{
			name:               "Failure - Repository Delete Error",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  assert.AnError,
			expectedStatus:     http.StatusInternalServerError,
			expectedBody:       `{"error":"failed to delete address"}`,
		},
		{
			name:               "Failure - Middleware User Check Fails",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  nil,
			mockUserErrMid:     users.ErrUserNotFound,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"user associated with token not found"}`,
		},
		{
			name:               "Failure - No Auth Token",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  nil,
			mockUserErrMid:     nil,
			mockAddrDeleteErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			// Conditionally mock middleware user check
			// Skip mocking if token is absent OR if URL IDs are invalid
			if tc.expectedBody != `{"error":"authorization header required"}` && tc.targetUserIDStr != "not-a-uuid" && tc.targetAddressIDStr != "not-a-uuid" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			shouldMockDelete := tc.mockUserErrMid == nil &&
				tc.expectedStatus != http.StatusForbidden &&
				tc.expectedStatus != http.StatusBadRequest &&
				tc.targetUserIDStr != "not-a-uuid" &&
				tc.targetAddressIDStr != "not-a-uuid" &&
				tc.expectedBody != `{"error":"authorization header required"}`

			if shouldMockDelete {
				targetAddrUUID, _ := uuid.Parse(tc.targetAddressIDStr)
				mockAddressRepo.On("Delete", mock.Anything, testUserID, targetAddrUUID).Return(tc.mockAddrDeleteErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses/%s", tc.targetUserIDStr, tc.targetAddressIDStr)
			req, _ := http.NewRequest(http.MethodDelete, url, nil)
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")

			if tc.expectedStatus == http.StatusNoContent {
				// No body expected for this status
			} else if tc.targetUserIDStr == "not-a-uuid" || tc.targetAddressIDStr == "not-a-uuid" {
				// For router 404s, check plain text body
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			} else {
				// For other errors, expect JSON
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

// TestUserHandler_SetDefaultBillingAddress covers the billing-default endpoint. It shares the
// same auth/validation flow as SetDefaultAddress; the key point is that it drives the
// independent SetDefaultBilling repository method.
func TestUserHandler_SetDefaultBillingAddress(t *testing.T) {
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	addressID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	tests := []struct {
		name              string
		targetUserIDStr   string
		mockUserReturnMid *models.User
		mockUserErrMid    error
		mockRepoErr       error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success - Set Billing Default for Own Address",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			expectedStatus:    http.StatusOK,
		},
		{
			name:              "Failure - Another User's Address",
			targetUserIDStr:   anotherUserID.String(),
			mockUserReturnMid: userForToken,
			expectedStatus:    http.StatusForbidden,
			expectedBody:      `{"error":"forbidden"}`,
		},
		{
			name:              "Failure - Address Not Found",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockRepoErr:       addresses.ErrAddressNotFound,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      `{"error":"address not found"}`,
		},
		{
			name:              "Failure - Repository Error",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockRepoErr:       assert.AnError,
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to set default billing address"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()

			if tc.expectedStatus != http.StatusForbidden {
				mockAddressRepo.On("SetDefaultBilling", mock.Anything, testUserID, addressID).Return(tc.mockRepoErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses/%s/default/billing", tc.targetUserIDStr, addressID.String())
			req, _ := http.NewRequest(http.MethodPost, url, nil)
			req.Header.Set("Authorization", "Bearer "+testToken)

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

// TestUserHandler_SetDefaultShippingAddress is the shipping mirror of the billing test.
func TestUserHandler_SetDefaultShippingAddress(t *testing.T) {
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	addressID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	tests := []struct {
		name              string
		targetUserIDStr   string
		mockUserReturnMid *models.User
		mockUserErrMid    error
		mockRepoErr       error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success - Set Shipping Default for Own Address",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			expectedStatus:    http.StatusOK,
		},
		{
			name:              "Failure - Another User's Address",
			targetUserIDStr:   anotherUserID.String(),
			mockUserReturnMid: userForToken,
			expectedStatus:    http.StatusForbidden,
			expectedBody:      `{"error":"forbidden"}`,
		},
		{
			name:              "Failure - Address Not Found",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockRepoErr:       addresses.ErrAddressNotFound,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      `{"error":"address not found"}`,
		},
		{
			name:              "Failure - Repository Error",
			targetUserIDStr:   testUserID.String(),
			mockUserReturnMid: userForToken,
			mockRepoErr:       assert.AnError,
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to set default shipping address"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()

			if tc.expectedStatus != http.StatusForbidden {
				mockAddressRepo.On("SetDefaultShipping", mock.Anything, testUserID, addressID).Return(tc.mockRepoErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses/%s/default/shipping", tc.targetUserIDStr, addressID.String())
			req, _ := http.NewRequest(http.MethodPost, url, nil)
			req.Header.Set("Authorization", "Bearer "+testToken)

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}

func TestUserHandler_SetDefaultAddress(t *testing.T) { // NOSONAR table-driven test
	testUserID := uuid.New()
	anotherUserID := uuid.New()
	addressID := uuid.New()
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err)
	userForToken := &models.User{ID: testUserID}

	tests := []struct {
		name               string
		targetUserIDStr    string
		targetAddressIDStr string
		mockUserReturnMid  *models.User
		mockUserErrMid     error
		mockAddrSetDefErr  error
		expectedStatus     int
		expectedBody       string
	}{
		{
			name:               "Success - Set Default for Own Address",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusOK,
			expectedBody:       "",
		},
		{
			name:               "Failure - Set Default for Another User's Address",
			targetUserIDStr:    anotherUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusForbidden,
			expectedBody:       `{"error":"forbidden"}`,
		},
		{
			name:               "Failure - Invalid User ID in URL",
			targetUserIDStr:    "not-a-uuid",
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Invalid Address ID in URL",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: "not-a-uuid",
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       "404 page not found",
		},
		{
			name:               "Failure - Address Not Found (Repo Error)",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  addresses.ErrAddressNotFound,
			expectedStatus:     http.StatusNotFound,
			expectedBody:       `{"error":"address not found"}`,
		},
		{
			name:               "Failure - Repository SetDefault Error",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  userForToken,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  assert.AnError,
			expectedStatus:     http.StatusInternalServerError,
			expectedBody:       `{"error":"failed to set default address"}`,
		},
		{
			name:               "Failure - Middleware User Check Fails",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  nil,
			mockUserErrMid:     users.ErrUserNotFound,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"user associated with token not found"}`,
		},
		{
			name:               "Failure - No Auth Token",
			targetUserIDStr:    testUserID.String(),
			targetAddressIDStr: addressID.String(),
			mockUserReturnMid:  nil,
			mockUserErrMid:     nil,
			mockAddrSetDefErr:  nil,
			expectedStatus:     http.StatusUnauthorized,
			expectedBody:       `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mockUserRepo, mockAddressRepo, _, router := setupUserHandlerTest(t)

			// Conditionally mock middleware user check
			// Skip mocking if token is absent OR if URL IDs are invalid
			if tc.expectedBody != `{"error":"authorization header required"}` && tc.targetUserIDStr != "not-a-uuid" && tc.targetAddressIDStr != "not-a-uuid" {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			shouldMockSetDefault := tc.mockUserErrMid == nil &&
				tc.expectedStatus != http.StatusForbidden &&
				tc.expectedStatus != http.StatusBadRequest &&
				tc.targetUserIDStr != "not-a-uuid" &&
				tc.targetAddressIDStr != "not-a-uuid" &&
				tc.expectedBody != `{"error":"authorization header required"}`

			if shouldMockSetDefault {
				targetAddrUUID, _ := uuid.Parse(tc.targetAddressIDStr)
				mockAddressRepo.On("SetDefault", mock.Anything, testUserID, targetAddrUUID).Return(tc.mockAddrSetDefErr).Once()
			}

			url := fmt.Sprintf("/api/users/%s/addresses/%s/default", tc.targetUserIDStr, tc.targetAddressIDStr)
			req, _ := http.NewRequest(http.MethodPost, url, nil)
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "")

			if tc.expectedStatus == http.StatusOK {
				// No body expected for this status
			} else if tc.targetUserIDStr == "not-a-uuid" || tc.targetAddressIDStr == "not-a-uuid" {
				// Check plain text body for router 404s
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			} else {
				// Check JSON body for other errors (like 403 Forbidden)
				require.JSONEq(t, tc.expectedBody, rr.Body.String())
			}

			mockUserRepo.AssertExpectations(t)
			mockAddressRepo.AssertExpectations(t)
		})
	}
}
