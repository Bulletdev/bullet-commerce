package handlers

import (
	"bullet-commerce/internal/addresses"
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/users"
	"bullet-commerce/internal/webutils"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type UserHandler struct {
	UserRepo    users.UserRepository
	AddressRepo addresses.AddressRepository
}

func NewUserHandler(userRepo users.UserRepository, addressRepo addresses.AddressRepository) *UserHandler {
	return &UserHandler{UserRepo: userRepo, AddressRepo: addressRepo}
}

func getAuthenticatedUserID(r *http.Request) (uuid.UUID, error) {
	userIDValue := r.Context().Value(auth.UserIDContextKey)
	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		slog.Error("user ID not found in context", "path", r.URL.Path)
		return uuid.Nil, errors.New("authentication context error")
	}
	return userID, nil
}

func checkUserAuthorization(w http.ResponseWriter, r *http.Request, targetUserIDStr string) (uuid.UUID, bool) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return uuid.Nil, false
	}

	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid user ID in URL"), http.StatusBadRequest)
		return uuid.Nil, false
	}

	if authUserID != targetUserID {
		webutils.ErrorJSON(w, errors.New("forbidden"), http.StatusForbidden)
		return uuid.Nil, false
	}

	return targetUserID, true
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	user, err := h.UserRepo.FindByID(r.Context(), authUserID)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			webutils.ErrorJSON(w, errors.New("user not found"), http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve user data"), http.StatusInternalServerError)
		}
		return
	}

	user.PasswordHash = ""
	webutils.WriteJSON(w, http.StatusOK, user)
}

type UpdateUserRequest struct {
	Name  string  `json:"name"`
	Email string  `json:"email"`
	CPF   *string `json:"cpf"`
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	authUserID, err := getAuthenticatedUserID(r)
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return
	}

	var req UpdateUserRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Email == "" {
		webutils.ErrorJSON(w, errors.New("name and email are required"), http.StatusBadRequest)
		return
	}

	updated, err := h.UserRepo.Update(r.Context(), authUserID, req.Name, req.Email, req.CPF)
	if err != nil {
		if errors.Is(err, users.ErrUserNotFound) {
			webutils.ErrorJSON(w, errors.New("user not found"), http.StatusNotFound)
		} else if errors.Is(err, users.ErrEmailAlreadyExists) {
			webutils.ErrorJSON(w, errors.New("email already in use"), http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update user"), http.StatusInternalServerError)
		}
		return
	}

	updated.PasswordHash = ""
	webutils.WriteJSON(w, http.StatusOK, updated)
}

type AddressRequest struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	IsDefault  bool   `json:"is_default"`
}

func (h *UserHandler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	targetUserID, authorized := checkUserAuthorization(w, r, mux.Vars(r)["userId"])
	if !authorized {
		return
	}

	list, err := h.AddressRepo.FindByUserID(r.Context(), targetUserID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve addresses"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *UserHandler) AddAddress(w http.ResponseWriter, r *http.Request) {
	targetUserID, authorized := checkUserAuthorization(w, r, mux.Vars(r)["userId"])
	if !authorized {
		return
	}

	var req AddressRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Street == "" || req.City == "" || req.State == "" || req.PostalCode == "" || req.Country == "" {
		webutils.ErrorJSON(w, errors.New("all address fields are required"), http.StatusBadRequest)
		return
	}

	created, err := h.AddressRepo.Create(r.Context(), &models.Address{
		UserID:     targetUserID,
		Street:     req.Street,
		City:       req.City,
		State:      req.State,
		PostalCode: req.PostalCode,
		Country:    req.Country,
		IsDefault:  req.IsDefault,
	})
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create address"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, created)
}

func (h *UserHandler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUserID, authorized := checkUserAuthorization(w, r, vars["userId"])
	if !authorized {
		return
	}

	addressID, err := uuid.Parse(vars["addressId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid address ID format"), http.StatusBadRequest)
		return
	}

	var req AddressRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Street == "" || req.City == "" || req.State == "" || req.PostalCode == "" || req.Country == "" {
		webutils.ErrorJSON(w, errors.New("all address fields are required"), http.StatusBadRequest)
		return
	}

	updated, err := h.AddressRepo.Update(r.Context(), targetUserID, addressID, &models.Address{
		Street:     req.Street,
		City:       req.City,
		State:      req.State,
		PostalCode: req.PostalCode,
		Country:    req.Country,
		IsDefault:  req.IsDefault,
	})
	if err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update address"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, updated)
}

func (h *UserHandler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUserID, authorized := checkUserAuthorization(w, r, vars["userId"])
	if !authorized {
		return
	}

	addressID, err := uuid.Parse(vars["addressId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid address ID format"), http.StatusBadRequest)
		return
	}

	if err := h.AddressRepo.Delete(r.Context(), targetUserID, addressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to delete address"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) SetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUserID, authorized := checkUserAuthorization(w, r, vars["userId"])
	if !authorized {
		return
	}

	addressID, err := uuid.Parse(vars["addressId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid address ID format"), http.StatusBadRequest)
		return
	}

	if err := h.AddressRepo.SetDefault(r.Context(), targetUserID, addressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to set default address"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *UserHandler) SetDefaultBillingAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUserID, authorized := checkUserAuthorization(w, r, vars["userId"])
	if !authorized {
		return
	}

	addressID, err := uuid.Parse(vars["addressId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid address ID format"), http.StatusBadRequest)
		return
	}

	if err := h.AddressRepo.SetDefaultBilling(r.Context(), targetUserID, addressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to set default billing address"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *UserHandler) SetDefaultShippingAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	targetUserID, authorized := checkUserAuthorization(w, r, vars["userId"])
	if !authorized {
		return
	}

	addressID, err := uuid.Parse(vars["addressId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid address ID format"), http.StatusBadRequest)
		return
	}

	if err := h.AddressRepo.SetDefaultShipping(r.Context(), targetUserID, addressID); err != nil {
		if errors.Is(err, addresses.ErrAddressNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to set default shipping address"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}
