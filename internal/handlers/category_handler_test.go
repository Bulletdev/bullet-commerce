package handlers_test

import (
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/categories"
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// setupCategoryTest creates mock repositories, handler, middleware, and router for category tests.
func setupCategoryTest(t *testing.T) (*categories.MockCategoryRepository, *MockUserRepository, *handlers.CategoryHandler, *auth.Middleware, *mux.Router) {
	mockCategoryRepo := new(categories.MockCategoryRepository)
	// Call the base setup - Capture necessary mocks and router, ignore others
	_, _, router, mockUserRepo, _, _, _, _, _ := setupBaseTest(t)

	categoryHandler := handlers.NewCategoryHandler(mockCategoryRepo)

	// Need authMiddleware instance for protected routes
	authMiddleware := auth.NewMiddleware(testJwtSecret, mockUserRepo)

	apiV1 := router.PathPrefix("/api").Subrouter()

	// Public category routes
	apiV1.HandleFunc("/categories", categoryHandler.GetAllCategories).Methods("GET")
	apiV1.HandleFunc("/categories/{id:[0-9a-fA-F-]+}", categoryHandler.GetCategory).Methods("GET")

	// Protected category routes
	protectedCategoryRoutes := apiV1.PathPrefix("/categories").Subrouter()
	protectedCategoryRoutes.Use(authMiddleware.Authenticate) // Apply middleware
	protectedCategoryRoutes.HandleFunc("", categoryHandler.CreateCategory).Methods("POST")
	protectedCategoryRoutes.HandleFunc("/{id:[0-9a-fA-F-]+}", categoryHandler.UpdateCategory).Methods("PUT")
	protectedCategoryRoutes.HandleFunc("/{id:[0-9a-fA-F-]+}", categoryHandler.DeleteCategory).Methods("DELETE")

	return mockCategoryRepo, mockUserRepo, categoryHandler, authMiddleware, router
}

// --- Tests for Public Routes ---

func TestCategoryHandler_GetAllCategories(t *testing.T) {
	mockRepo, _, _, _, router := setupCategoryTest(t)

	tests := []struct {
		name           string
		mockReturn     []models.Category
		mockError      error
		expectedStatus int
		expectedBody   string // Expect JSON string or partial match
	}{
		{
			name: "Success - Multiple Categories",
			mockReturn: []models.Category{
				{ID: uuid.New(), Name: "Electronics", CreatedAt: time.Now(), UpdatedAt: time.Now()},
				{ID: uuid.New(), Name: "Books", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody:   "Electronics", // Just check if category names are present
		},
		{
			name:           "Success - No Categories",
			mockReturn:     []models.Category{}, // Empty slice
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody:   `[]`,
		},
		{
			name:           "Failure - Repository Error",
			mockReturn:     nil,
			mockError:      errors.New("db error"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to retrieve categories"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			mockRepo.On("FindAll", mock.Anything).Return(tc.mockReturn, tc.mockError).Once()

			req := httptest.NewRequest(http.MethodGet, "/api/categories", nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.expectedBody)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestCategoryHandler_GetCategory(t *testing.T) {
	testID := uuid.New()
	foundCategory := models.Category{ID: testID, Name: "Specific Category", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	tests := []struct {
		name           string
		categoryID     string
		mockReturn     *models.Category
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Success - Category Found",
			categoryID:     testID.String(),
			mockReturn:     &foundCategory,
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody:   "Specific Category",
		},
		{
			name:           "Failure - Category Not Found",
			categoryID:     uuid.New().String(), // Different ID
			mockReturn:     nil,
			mockError:      categories.ErrCategoryNotFound,
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error":"category not found"}`,
		},
		{
			name:           "Failure - Invalid UUID",
			categoryID:     "not-a-uuid",
			mockReturn:     nil, // Mock won't be called
			mockError:      nil,
			expectedStatus: http.StatusNotFound,  // <<< CORRECTION: Expect 404 from router
			expectedBody:   "404 page not found", // <<< CORRECTION: Expect router's 404 message
		},
		{
			name:           "Failure - Repository Error",
			categoryID:     testID.String(),
			mockReturn:     nil, // <<< CORRECTION: Ensure mock returns nil object on error
			mockError:      errors.New("db error"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to retrieve category"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Moved setup inside t.Run for isolation
			mockRepo, _, _, _, router := setupCategoryTest(t)

			// Setup mock expectation only if the UUID is valid
			if tc.categoryID != "not-a-uuid" {
				parsedID, _ := uuid.Parse(tc.categoryID)
				mockRepo.On("FindByID", mock.Anything, parsedID).Return(tc.mockReturn, tc.mockError).Once()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/categories/"+tc.categoryID, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.expectedBody)
			mockRepo.AssertExpectations(t)
		})
	}
}

// --- Tests for Protected Routes (Require Authentication) ---

func TestCategoryHandler_CreateCategory(t *testing.T) {
	testUserID := uuid.New()
	// Corrected generateTestToken call
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err, "Failed to generate test token")

	tests := []struct {
		name           string
		body           string
		mockUserReturn *models.User // For middleware check
		mockUserErr    error
		mockCreateErr  error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Success",
			body:           `{"name":"New Category"}`,
			mockUserReturn: &models.User{ID: testUserID}, // Simulate user exists
			mockUserErr:    nil,
			mockCreateErr:  nil,
			expectedStatus: http.StatusCreated,
			expectedBody:   "New Category",
		},
		{
			name:           "Failure - Invalid JSON",
			body:           `{"name":"Category",}`, // Invalid JSON
			mockUserReturn: &models.User{ID: testUserID},
			mockUserErr:    nil,
			mockCreateErr:  nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"invalid request body"}`,
		},
		{
			name:           "Failure - Missing Name",
			body:           `{}`, // Empty body
			mockUserReturn: &models.User{ID: testUserID},
			mockUserErr:    nil,
			mockCreateErr:  nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"category name is required"}`,
		},
		{
			name:           "Failure - Name Already Exists",
			body:           `{"name":"Existing Category"}`,
			mockUserReturn: &models.User{ID: testUserID},
			mockUserErr:    nil,
			mockCreateErr:  categories.ErrCategoryNameExists,
			expectedStatus: http.StatusConflict,
			expectedBody:   `{"error":"category name already exists"}`,
		},
		{
			name:           "Failure - Repo Create Error",
			body:           `{"name":"Good Category"}`,
			mockUserReturn: &models.User{ID: testUserID},
			mockUserErr:    nil,
			mockCreateErr:  errors.New("db create failed"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"error":"failed to create category"}`,
		},
		{
			name:           "Failure - Middleware User Check Fails",
			body:           `{"name":"Another Category"}`,
			mockUserReturn: nil, // Simulate user not found by middleware
			mockUserErr:    users.ErrUserNotFound,
			mockCreateErr:  nil,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"error":"user associated with token not found"}`,
		},
		{
			name:           "Failure - No Auth Token",
			body:           `{"name":"No Token Category"}`,
			mockUserReturn: nil, // Won't be called
			mockUserErr:    nil,
			mockCreateErr:  nil,
			expectedStatus: http.StatusUnauthorized,                     // Expect unauthorized if no token provided
			expectedBody:   `{"error":"authorization header required"}`, // <<< CORRECTION: Actual middleware message
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Moved setup inside t.Run for isolation
			mockCategoryRepo, mockUserRepo, _, _, router := setupCategoryTest(t)

			// Mock middleware user check (only if token is expected)
			if tc.expectedStatus != http.StatusUnauthorized || tc.expectedBody == `{"error":"user associated with token not found"}` {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturn, tc.mockUserErr).Once()
			}

			// Mock category repo create (only if middleware check passes and validation passes)
			if tc.mockUserErr == nil && tc.expectedStatus != http.StatusBadRequest && tc.expectedStatus != http.StatusUnauthorized {
				mockCategoryRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Category")).
					Return(func(ctx context.Context, c *models.Category) *models.Category {
						if tc.mockCreateErr != nil {
							return nil
						}
						c.ID = uuid.New()
						c.CreatedAt = time.Now()
						c.UpdatedAt = c.CreatedAt
						return c
					}, tc.mockCreateErr).Once()
			}

			req := httptest.NewRequest(http.MethodPost, "/api/categories", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			// Only add token if the test case expects it (i.e., not the 'No Auth Token' case)
			if tc.expectedStatus != http.StatusUnauthorized || tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tc.expectedBody)
			mockUserRepo.AssertExpectations(t)
			mockCategoryRepo.AssertExpectations(t)
		})
	}
}

func TestCategoryHandler_UpdateCategory(t *testing.T) {
	testUserID := uuid.New()
	categoryToUpdateID := uuid.New()
	// Corrected generateTestToken call
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err, "Failed to generate test token")
	userForToken := &models.User{ID: testUserID} // Define user for token once

	tests := []struct {
		name              string
		categoryID        string
		body              string
		mockUserReturnMid *models.User // Renamed for clarity (middleware check)
		mockUserErrMid    error        // Renamed for clarity (middleware check)
		mockUpdateErr     error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Updated Category Name"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     nil,
			expectedStatus:    http.StatusOK,
			expectedBody:      "Updated Category Name",
		},
		{
			name:              "Failure - Invalid UUID",
			categoryID:        "not-a-uuid",
			body:              `{"name":"Update Attempt"}`,
			mockUserReturnMid: userForToken, // Mock won't be called if UUID is invalid before middleware
			mockUserErrMid:    nil,
			mockUpdateErr:     nil,
			expectedStatus:    http.StatusNotFound, // Router handles invalid path param
			expectedBody:      "404 page not found",
		},
		{
			name:              "Failure - Invalid JSON",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":}`, // Invalid JSON
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      `{"error":"invalid request body"}`,
		},
		{
			name:              "Failure - Missing Name",
			categoryID:        categoryToUpdateID.String(),
			body:              `{}`, // Missing name field
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     nil,
			expectedStatus:    http.StatusBadRequest,
			expectedBody:      `{"error":"category name is required"}`,
		},
		{
			name:              "Failure - Category Not Found",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Update Attempt"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     categories.ErrCategoryNotFound,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      `{"error":"category not found"}`,
		},
		{
			name:              "Failure - Name Already Exists",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Existing Name"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     categories.ErrCategoryNameExists,
			expectedStatus:    http.StatusConflict,
			expectedBody:      `{"error":"category name already exists"}`,
		},
		{
			name:              "Failure - Repo Update Error",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Update Attempt"}`,
			mockUserReturnMid: userForToken,
			mockUserErrMid:    nil,
			mockUpdateErr:     errors.New("db update failed"),
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to update category"}`,
		},
		{
			name:              "Failure - Middleware User Check Fails",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Update Attempt"}`,
			mockUserReturnMid: nil, // Simulate user not found by middleware
			mockUserErrMid:    users.ErrUserNotFound,
			mockUpdateErr:     nil, // Update won't be called
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"user associated with token not found"}`,
		},
		{
			name:              "Failure - No Auth Token",
			categoryID:        categoryToUpdateID.String(),
			body:              `{"name":"Update Attempt"}`,
			mockUserReturnMid: nil, // Mock won't be called
			mockUserErrMid:    nil,
			mockUpdateErr:     nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Moved setup inside t.Run for isolation
			mockCategoryRepo, mockUserRepo, _, _, router := setupCategoryTest(t)

			// --- Mock Setup ---
			isValidUUID := true
			var parsedID uuid.UUID
			var err error
			if tc.categoryID != "not-a-uuid" {
				parsedID, err = uuid.Parse(tc.categoryID)
				if err != nil {
					isValidUUID = false // Should not happen with regex, but good practice
				}
			} else {
				isValidUUID = false
			}

			// 1. Mock middleware user check (UserRepo.FindByID)
			// Only setup if the categoryID in URL is valid AND we are not testing the "No Auth Token" case.
			shouldMockMiddleware := isValidUUID && tc.expectedBody != `{"error":"authorization header required"}`
			if shouldMockMiddleware {
				// *** USE CORRECTED VARIABLES FOR MIDDLEWARE MOCK ***
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once()
			}

			// 2. Mock category repo update (CategoryRepo.Update)
			// Only setup if:
			// - categoryID in URL is valid
			// - Middleware check is expected to pass (tc.mockUserErrMid == nil)
			// - Request body validation is expected to pass (tc.expectedStatus != 400)
			// - We are not expecting an auth error (middleware passed or no token was needed)
			shouldMockUpdate := isValidUUID && tc.mockUserErrMid == nil && tc.expectedStatus != http.StatusBadRequest && tc.expectedStatus != http.StatusUnauthorized
			if shouldMockUpdate {
				// *** USE CORRECTED LOGIC FOR UPDATE MOCK ***
				mockCategoryRepo.On("Update", mock.Anything, parsedID, mock.MatchedBy(func(cat *models.Category) bool {
					// Optionally add more specific matching for the category payload here if needed
					return cat != nil && cat.Name != ""
				})).Return(func(ctx context.Context, id uuid.UUID, c *models.Category) *models.Category {
					// Return nil object if there's an error to mimic real behavior
					if tc.mockUpdateErr != nil {
						return nil
					}
					// Simulate successful update by returning the input category with updated timestamp
					c.ID = id
					c.UpdatedAt = time.Now()
					return c
				}, tc.mockUpdateErr).Once()
			}

			// --- Request Execution ---
			req := httptest.NewRequest(http.MethodPut, "/api/categories/"+tc.categoryID, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")

			// Add token only if not testing the "No Auth Token" case.
			if tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			// --- Assertions ---
			assert.Equal(t, tc.expectedStatus, rr.Code, "Status code mismatch")
			assert.Contains(t, rr.Body.String(), tc.expectedBody, "Response body mismatch")

			// Verify that mocks were called as expected
			mockUserRepo.AssertExpectations(t)
			mockCategoryRepo.AssertExpectations(t)
		})
	}
}

func TestCategoryHandler_DeleteCategory(t *testing.T) {
	testUserID := uuid.New()
	categoryToDeleteID := uuid.New()
	// Corrected generateTestToken call
	testToken, err := generateTestToken(testUserID)
	require.NoError(t, err, "Failed to generate test token")
	userForToken := &models.User{ID: testUserID} // Define user for token once

	tests := []struct {
		name              string
		categoryID        string
		mockUserReturnMid *models.User // Renamed
		mockUserErrMid    error        // Renamed
		mockDeleteErr     error
		expectedStatus    int
		expectedBody      string
	}{
		{
			name:              "Success",
			categoryID:        categoryToDeleteID.String(),
			mockUserReturnMid: userForToken, // Corrected
			mockUserErrMid:    nil,          // Corrected
			mockDeleteErr:     nil,
			expectedStatus:    http.StatusNoContent,
			expectedBody:      "",
		},
		{
			name:              "Failure - Invalid UUID",
			categoryID:        "not-a-uuid",
			mockUserReturnMid: userForToken, // Corrected
			mockUserErrMid:    nil,          // Corrected
			mockDeleteErr:     nil,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      "404 page not found",
		},
		{
			name:              "Failure - Category Not Found",
			categoryID:        categoryToDeleteID.String(),
			mockUserReturnMid: userForToken, // Corrected
			mockUserErrMid:    nil,          // Corrected
			mockDeleteErr:     categories.ErrCategoryNotFound,
			expectedStatus:    http.StatusNotFound,
			expectedBody:      `{"error":"category not found"}`,
		},
		{
			name:              "Failure - Repo Delete Error",
			categoryID:        categoryToDeleteID.String(),
			mockUserReturnMid: userForToken, // Corrected
			mockUserErrMid:    nil,          // Corrected
			mockDeleteErr:     errors.New("db delete failed"),
			expectedStatus:    http.StatusInternalServerError,
			expectedBody:      `{"error":"failed to delete category"}`,
		},
		{
			name:              "Failure - Middleware User Check Fails",
			categoryID:        categoryToDeleteID.String(),
			mockUserReturnMid: nil,                   // Corrected
			mockUserErrMid:    users.ErrUserNotFound, // Corrected
			mockDeleteErr:     nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"user associated with token not found"}`,
		},
		{
			name:              "Failure - No Auth Token",
			categoryID:        categoryToDeleteID.String(),
			mockUserReturnMid: nil, // Corrected
			mockUserErrMid:    nil, // Corrected
			mockDeleteErr:     nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedBody:      `{"error":"authorization header required"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Moved setup inside t.Run for isolation
			mockCategoryRepo, mockUserRepo, _, _, router := setupCategoryTest(t)

			// Mock middleware user check only if the UUID is valid AND we are not testing the "No Auth Token" case directly
			if tc.categoryID != "not-a-uuid" && tc.expectedBody != `{"error":"authorization header required"}` {
				mockUserRepo.On("FindByID", mock.Anything, testUserID).Return(tc.mockUserReturnMid, tc.mockUserErrMid).Once() // Corrected
			}

			// Mock category repo delete (only if middleware/parsing passes)
			if tc.categoryID != "not-a-uuid" && tc.mockUserErrMid == nil && tc.expectedStatus != http.StatusBadRequest && tc.expectedStatus != http.StatusUnauthorized { // Corrected
				parsedID, _ := uuid.Parse(tc.categoryID)
				mockCategoryRepo.On("Delete", mock.Anything, parsedID).Return(tc.mockDeleteErr).Once()
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/categories/"+tc.categoryID, nil)
			if tc.expectedStatus != http.StatusUnauthorized || tc.expectedBody != `{"error":"authorization header required"}` {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			if tc.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			} else {
				assert.Empty(t, rr.Body.String())
			}
			mockUserRepo.AssertExpectations(t)
			mockCategoryRepo.AssertExpectations(t)
		})
	}
}

// Test functions will go here...
