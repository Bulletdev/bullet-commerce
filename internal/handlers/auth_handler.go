package handlers

import (
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"bullet-commerce/internal/webutils"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// AuthHandler handles authentication requests.
type AuthHandler struct {
	UserRepo            users.UserRepository
	Hasher              auth.PasswordHasher
	JwtSecret           string
	TokenExpiryDuration time.Duration
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userRepo users.UserRepository, hasher auth.PasswordHasher, jwtSecret string, tokenExpiry time.Duration) *AuthHandler {
	return &AuthHandler{
		UserRepo:            userRepo,
		Hasher:              hasher,
		JwtSecret:           jwtSecret,
		TokenExpiryDuration: tokenExpiry,
	}
}

// --- Request/Response Structs ---

type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

// --- Handlers ---

// Register handles new user registration.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Name == "" || req.Email == "" || req.Password == "" {
		webutils.ErrorJSON(w, errors.New("name, email, and password are required"), http.StatusBadRequest)
		return
	}

	// Check if email already exists
	_, err := h.UserRepo.FindByEmail(context.Background(), req.Email)
	if err == nil {
		webutils.ErrorJSON(w, errors.New("email already registered"), http.StatusConflict)
		return
	} else if !errors.Is(err, users.ErrUserNotFound) {
		webutils.ErrorJSON(w, errors.New("failed to check email existence"), http.StatusInternalServerError)
		return
	}

	hashedPassword, err := h.Hasher.HashPassword(req.Password)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to register user"), http.StatusInternalServerError)
		return
	}

	user := &models.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: hashedPassword,
	}

	createdUser, err := h.UserRepo.Create(context.Background(), user.Name, user.Email, user.PasswordHash)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to register user"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, createdUser)
}

// Login handles user login and JWT generation.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		webutils.ErrorJSON(w, errors.New("email and password are required"), http.StatusBadRequest)
		return
	}

	user, err := h.UserRepo.FindByEmail(context.Background(), req.Email)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			webutils.ErrorJSON(w, errors.New("invalid email or password"), http.StatusUnauthorized)
		} else {
			webutils.ErrorJSON(w, errors.New("login failed"), http.StatusInternalServerError)
		}
		return
	}

	err = h.Hasher.CheckPassword(user.PasswordHash, req.Password)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid email or password"), http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user.ID, h.JwtSecret, h.TokenExpiryDuration)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("login failed"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
}
