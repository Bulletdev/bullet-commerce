package handlers_test

import (
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// setupAuthTest sets up mocks, handler, and router for auth tests.
// func setupAuthTest(t *testing.T) (*users.MockUserRepository, *handlers.AuthHandler, *mux.Router) {
// 	// Use the base setup for common parts (user mock, middleware, router)
// 	mockUserRepo, _, router, _, _, _, _, _, _ := setupBaseTest(t) // Corrected assignment
//
// 	// Instantiate the real password hasher
// 	hasher := auth.NewBcryptPasswordHasher() // Use real hasher or MockPasswordHasher if preferred
//
// 	// Provide test JWT configuration
// 	testJwtExpiry := time.Hour * 1 // Example expiry for tests
//
// 	// Create the AuthHandler, now passing the hasher, secret and expiry
// 	authHandler := handlers.NewAuthHandler(mockUserRepo, hasher, testJwtSecret, testJwtExpiry)
//
// 	// Define routes handled by AuthHandler (no middleware needed for these)
// 	apiV1 := router.PathPrefix("/api/auth").Subrouter()
// 	apiV1.HandleFunc("/register", authHandler.Register).Methods("POST")
// 	apiV1.HandleFunc("/login", authHandler.Login).Methods("POST")
//
// 	return mockUserRepo, authHandler, router
// }
// NOTE: setupAuthTest is removed as setup now happens inside each test for better isolation
// and mock control.

func TestAuthHandler_Register(t *testing.T) {
	// User details
	userName := "Test User"
	userEmail := "test@example.com"
	userPassword := "password123"
	hashedPassword := "hashed_password_string"
	createdUser := &models.User{ID: uuid.New(), Name: userName, Email: userEmail}

	tests := []struct {
		name string
		body string
		// These functions configure the mocks created *inside* t.Run
		mockHashPassword    func(*MockPasswordHasher)
		mockUserCreate      func(*MockUserRepository)
		mockUserFindByEmail func(*MockUserRepository)
		expectedStatus      int
		expectedBodyRegexp  string // Use regexp for ID matching
	}{
		{
			name: "Success",
			body: fmt.Sprintf(`{"name":"%s", "email":"%s", "password":"%s"}`, userName, userEmail, userPassword),
			// Pass the subtest mockHasher to the setup function
			mockHashPassword: func(hasher *MockPasswordHasher) {
				hasher.On("HashPassword", userPassword).Return(hashedPassword, nil).Once()
			},
			// Pass the subtest mockUserRepo to the setup function
			mockUserCreate: func(repo *MockUserRepository) {
				repo.On("Create", mock.Anything, userName, userEmail, hashedPassword).Return(createdUser, nil).Once()
			},
			// Pass the subtest mockUserRepo to the setup function
			mockUserFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, users.ErrUserNotFound).Once()
			},
			expectedStatus:     http.StatusCreated,
			expectedBodyRegexp: fmt.Sprintf(`{"id":"%s","name":"%s","email":"%s","created_at":".*","updated_at":".*"}`, createdUser.ID, userName, userEmail),
		},
		{
			name: "Duplicate Email",
			body: fmt.Sprintf(`{"name":"%s", "email":"%s", "password":"%s"}`, userName, userEmail, userPassword),
			mockHashPassword: func(hasher *MockPasswordHasher) {
				// No HashPassword mock needed if FindByEmail finds the user first
			},
			mockUserCreate: func(repo *MockUserRepository) {
				// Create should not be called if email is found
			},
			mockUserFindByEmail: func(repo *MockUserRepository) {
				// Simulate finding an existing user
				repo.On("FindByEmail", mock.Anything, userEmail).Return(&models.User{ID: uuid.New(), Email: userEmail}, nil).Once()
			},
			expectedStatus:     http.StatusConflict,
			expectedBodyRegexp: `{"error":"email already registered"}`,
		},
		{
			name: "Hashing Error",
			body: fmt.Sprintf(`{"name":"%s", "email":"%s", "password":"%s"}`, userName, userEmail, userPassword),
			mockHashPassword: func(hasher *MockPasswordHasher) {
				hasher.On("HashPassword", userPassword).Return("", assert.AnError).Once()
			},
			mockUserCreate: func(repo *MockUserRepository) { /* Not called */ },
			mockUserFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, users.ErrUserNotFound).Once()
			},
			expectedStatus:     http.StatusInternalServerError,
			expectedBodyRegexp: `{"error":"failed to register user"}`,
		},
		{
			name: "Create User Error",
			body: fmt.Sprintf(`{"name":"%s", "email":"%s", "password":"%s"}`, userName, userEmail, userPassword),
			mockHashPassword: func(hasher *MockPasswordHasher) {
				hasher.On("HashPassword", userPassword).Return(hashedPassword, nil).Once()
			},
			mockUserCreate: func(repo *MockUserRepository) {
				repo.On("Create", mock.Anything, userName, userEmail, hashedPassword).Return(nil, assert.AnError).Once()
			},
			mockUserFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, users.ErrUserNotFound).Once()
			},
			expectedStatus:     http.StatusInternalServerError,
			expectedBodyRegexp: `{"error":"failed to register user"}`,
		},
		{
			name:                "Invalid JSON",
			body:                `{"email":"test@example.com",}`, // Malformed
			mockHashPassword:    func(hasher *MockPasswordHasher) { /* Not called */ },
			mockUserCreate:      func(repo *MockUserRepository) { /* Not called */ },
			mockUserFindByEmail: func(repo *MockUserRepository) { /* Not called */ },
			expectedStatus:      http.StatusBadRequest,
			expectedBodyRegexp:  `{"error":"invalid request body"}`,
		},
		{
			name:                "Missing Field",
			body:                fmt.Sprintf(`{"name":"%s", "email":"%s"}`, userName, userEmail), // Missing password
			mockHashPassword:    func(hasher *MockPasswordHasher) { /* Not called */ },
			mockUserCreate:      func(repo *MockUserRepository) { /* Not called */ },
			mockUserFindByEmail: func(repo *MockUserRepository) { /* Not called */ },
			expectedStatus:      http.StatusBadRequest,
			expectedBodyRegexp:  `{"error":"name, email, and password are required"}`,
		},
		{
			name:             "FindByEmail DB Error",
			body:             fmt.Sprintf(`{"name":"%s", "email":"%s", "password":"%s"}`, userName, userEmail, userPassword),
			mockHashPassword: func(hasher *MockPasswordHasher) { /* Not called */ },
			mockUserCreate:   func(repo *MockUserRepository) { /* Not called */ },
			mockUserFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, assert.AnError).Once()
			},
			expectedStatus:     http.StatusInternalServerError,
			expectedBodyRegexp: `{"error":"failed to check email existence"}`,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks for subtest
			mockUserRepo := new(MockUserRepository)
			mockHasher := new(MockPasswordHasher)

			// Create a NEW AuthHandler INSIDE t.Run using subtest mocks
			authHandler := handlers.NewAuthHandler(mockUserRepo, mockHasher, testJwtSecret, time.Hour*1)

			// Setup mocks for the specific test case by passing the subtest mocks
			tc.mockUserFindByEmail(mockUserRepo)
			tc.mockHashPassword(mockHasher)
			tc.mockUserCreate(mockUserRepo)

			// Create router and register handler INSIDE t.Run for isolation
			router := mux.NewRouter()
			router.HandleFunc("/register", authHandler.Register).Methods("POST")

			req, _ := http.NewRequest("POST", "/register", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "") // Check status code first
			// Use Regexp for body check because of dynamic ID/timestamps
			require.Regexp(t, tc.expectedBodyRegexp, rr.Body.String(), "handler returned unexpected body content")

			mockUserRepo.AssertExpectations(t)
			mockHasher.AssertExpectations(t)
		})
	}
}

func TestAuthHandler_Login(t *testing.T) {
	// User details
	userEmail := "test@example.com"
	userPassword := "password123"
	storedHash := "correct_hashed_password"
	fakeUserID := uuid.New()
	foundUser := &models.User{ID: fakeUserID, Email: userEmail, PasswordHash: storedHash}

	tests := []struct {
		name string
		body string
		// Pass mocks created inside t.Run
		mockFindByEmail   func(*MockUserRepository)
		mockCheckPassword func(*MockPasswordHasher)
		expectedStatus    int
		expectedBodyJSON  map[string]interface{} // Check specific fields
		expectedBodyError string                 // For error cases
	}{
		{
			name: "Success",
			body: fmt.Sprintf(`{"email":"%s", "password":"%s"}`, userEmail, userPassword),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(foundUser, nil).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) {
				hasher.On("CheckPassword", storedHash, userPassword).Return(nil).Once()
			},
			expectedStatus:    http.StatusOK,
			expectedBodyJSON:  map[string]interface{}{"token": mock.AnythingOfType("string")},
			expectedBodyError: "",
		},
		{
			name: "User Not Found",
			body: fmt.Sprintf(`{"email":"%s", "password":"%s"}`, userEmail, userPassword),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, users.ErrUserNotFound).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) { /* Not called */ },
			expectedStatus:    http.StatusUnauthorized,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"invalid email or password"}`,
		},
		{
			name: "Incorrect Password",
			body: fmt.Sprintf(`{"email":"%s", "password":"wrongpassword"}`, userEmail),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(foundUser, nil).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) {
				hasher.On("CheckPassword", storedHash, "wrongpassword").Return(bcrypt.ErrMismatchedHashAndPassword).Once()
			},
			expectedStatus:    http.StatusUnauthorized,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"invalid email or password"}`,
		},
		{
			name: "Find User DB Error",
			body: fmt.Sprintf(`{"email":"%s", "password":"%s"}`, userEmail, userPassword),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(nil, assert.AnError).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) { /* Not called */ },
			expectedStatus:    http.StatusInternalServerError,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"login failed"}`, // Updated error message
		},
		{
			name: "Check Password Error",
			body: fmt.Sprintf(`{"email":"%s", "password":"%s"}`, userEmail, userPassword),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(foundUser, nil).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) {
				// Simulate an error during the check (could be bcrypt error or other)
				hasher.On("CheckPassword", storedHash, userPassword).Return(assert.AnError).Once()
			},
			expectedStatus:    http.StatusUnauthorized,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"invalid email or password"}`,
		},
		{
			name: "Token Generation Error",
			body: fmt.Sprintf(`{"email":"%s", "password":"%s"}`, userEmail, userPassword),
			mockFindByEmail: func(repo *MockUserRepository) {
				repo.On("FindByEmail", mock.Anything, userEmail).Return(foundUser, nil).Once()
			},
			mockCheckPassword: func(hasher *MockPasswordHasher) {
				hasher.On("CheckPassword", storedHash, userPassword).Return(nil).Once()
				// Assume GenerateToken succeeds if CheckPassword is nil.
			},
			expectedStatus:    http.StatusOK,
			expectedBodyJSON:  map[string]interface{}{"token": mock.AnythingOfType("string")},
			expectedBodyError: "",
		},
		{
			name:              "Invalid JSON",
			body:              `{"email":"bad}`, // Malformed
			mockFindByEmail:   func(repo *MockUserRepository) { /* Not called */ },
			mockCheckPassword: func(hasher *MockPasswordHasher) { /* Not called */ },
			expectedStatus:    http.StatusBadRequest,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"invalid request body"}`,
		},
		{
			name:              "Missing Field",
			body:              fmt.Sprintf(`{"email":"%s"}`, userEmail), // Missing password
			mockFindByEmail:   func(repo *MockUserRepository) { /* Not called */ },
			mockCheckPassword: func(hasher *MockPasswordHasher) { /* Not called */ },
			expectedStatus:    http.StatusBadRequest,
			expectedBodyJSON:  nil,
			expectedBodyError: `{"error":"email and password are required"}`, // Updated error message
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks for subtest
			mockUserRepo := new(MockUserRepository)
			mockHasher := new(MockPasswordHasher)

			// Create a NEW AuthHandler INSIDE t.Run using subtest mocks
			authHandler := handlers.NewAuthHandler(mockUserRepo, mockHasher, testJwtSecret, time.Hour*1)

			// Setup mock expectations on the subtest mocks
			tc.mockFindByEmail(mockUserRepo)
			tc.mockCheckPassword(mockHasher)

			// Create router and register handler INSIDE t.Run for isolation
			router := mux.NewRouter()
			router.HandleFunc("/login", authHandler.Login).Methods("POST")

			req, _ := http.NewRequest("POST", "/login", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")

			rr := executeRequestAndAssert(t, router, req, tc.expectedStatus, "") // Check status only first

			if tc.expectedBodyError != "" {
				require.JSONEq(t, tc.expectedBodyError, rr.Body.String(), "Handler returned wrong error body")
			} else {
				// Check for token presence and type
				var respBody map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &respBody)
				require.NoError(t, err, "Failed to unmarshal response body")
				require.Contains(t, respBody, "token", "Response body should contain token")
				require.IsType(t, "string", respBody["token"], "Token should be a string")
				// Optionally, validate the token structure/payload here if needed
			}

			mockUserRepo.AssertExpectations(t)
			mockHasher.AssertExpectations(t)
		})
	}
}
