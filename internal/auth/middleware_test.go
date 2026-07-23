package auth

import (
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testSecret = "test-secret-32-chars-minimum-len"

type mockUserRepo struct{ mock.Mock }

func (m *mockUserRepo) Create(_ context.Context, name, email, hash string) (*models.User, error) {
	args := m.Called(name, email, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) FindByEmail(_ context.Context, email string) (*models.User, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) FindByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *mockUserRepo) Update(_ context.Context, id uuid.UUID, name, email string, cpf *string) (*models.User, error) {
	args := m.Called(id, name, email, cpf)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func buildMiddleware(repo *mockUserRepo) *Middleware {
	return NewMiddleware(testSecret, repo)
}

func validToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := GenerateToken(userID, testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAuthenticate_Valid(t *testing.T) {
	userID := uuid.New()
	repo := &mockUserRepo{}
	repo.On("FindByID", userID).Return(&models.User{ID: userID, Role: models.RoleUser}, nil)

	mw := buildMiddleware(repo)
	called := false

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotID := r.Context().Value(UserIDContextKey)
		assert.Equal(t, userID, gotID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, userID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
	repo.AssertExpectations(t)
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	mw := buildMiddleware(&mockUserRepo{})

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	mw := buildMiddleware(&mockUserRepo{})

	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer notavalidtoken")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthenticate_UserNotFound(t *testing.T) {
	userID := uuid.New()
	repo := &mockUserRepo{}
	repo.On("FindByID", userID).Return(nil, users.ErrUserNotFound)

	mw := buildMiddleware(repo)
	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, userID))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthenticate_DBError(t *testing.T) {
	userID := uuid.New()
	repo := &mockUserRepo{}
	repo.On("FindByID", userID).Return(nil, errors.New("db timeout"))

	mw := buildMiddleware(repo)
	handler := mw.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, userID))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestRequireAdmin_Allow(t *testing.T) {
	userID := uuid.New()
	repo := &mockUserRepo{}
	repo.On("FindByID", userID).Return(&models.User{ID: userID, Role: models.RoleAdmin}, nil)

	mw := buildMiddleware(repo)
	called := false

	chain := mw.Authenticate(mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/products", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, userID))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireAdmin_Deny(t *testing.T) {
	userID := uuid.New()
	repo := &mockUserRepo{}
	repo.On("FindByID", userID).Return(&models.User{ID: userID, Role: models.RoleUser}, nil)

	mw := buildMiddleware(repo)

	chain := mw.Authenticate(mw.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach admin handler")
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/products", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, userID))
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
