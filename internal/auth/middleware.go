package auth

import (
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"bullet-commerce/internal/webutils"
	"context"
	"errors"
	"net/http"
	"strings"
)

type ContextKey string

const UserIDContextKey ContextKey = "userID"

type Middleware struct {
	jwtSecret string
	userRepo  users.UserRepository
}

func NewMiddleware(jwtSecret string, userRepo users.UserRepository) *Middleware {
	return &Middleware{jwtSecret: jwtSecret, userRepo: userRepo}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			webutils.ErrorJSON(w, errors.New("authorization header required"), http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			webutils.ErrorJSON(w, errors.New("invalid authorization header format"), http.StatusUnauthorized)
			return
		}

		claims, err := ValidateToken(parts[1], m.jwtSecret)
		if err != nil {
			webutils.ErrorJSON(w, ErrInvalidToken, http.StatusUnauthorized)
			return
		}

		user, err := m.userRepo.FindByID(r.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, users.ErrUserNotFound) {
				webutils.ErrorJSON(w, errors.New("user associated with token not found"), http.StatusUnauthorized)
			} else {
				webutils.ErrorJSON(w, errors.New("error verifying user"), http.StatusInternalServerError)
			}
			return
		}

		ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
		// Store the user role so RequireAdmin doesn't need another DB lookup.
		ctx = context.WithValue(ctx, userRoleKey, user.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type roleContextKey string

const userRoleKey roleContextKey = "userRole"

func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(userRoleKey).(models.UserRole)
		if role != models.RoleAdmin {
			webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
