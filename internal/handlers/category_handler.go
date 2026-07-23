package handlers

import (
	"bullet-commerce/internal/categories" // Category Repository
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/webutils" // JSON Helpers
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// CategoryHandler handles category-related requests.
type CategoryHandler struct {
	CategoryRepo categories.CategoryRepository
}

// NewCategoryHandler creates a new CategoryHandler.
func NewCategoryHandler(categoryRepo categories.CategoryRepository) *CategoryHandler {
	return &CategoryHandler{CategoryRepo: categoryRepo}
}

// --- Request Structs ---

type CreateCategoryRequest struct {
	Name string `json:"name"`
}

type UpdateCategoryRequest struct {
	Name string `json:"name"`
}

// --- Handlers ---

// CreateCategory handles POST requests to create a new category.
// Typically requires admin authentication.
func (h *CategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		webutils.ErrorJSON(w, errors.New("category name is required"), http.StatusBadRequest)
		return
	}

	newCategory := &models.Category{Name: req.Name}

	createdCategory, err := h.CategoryRepo.Create(r.Context(), newCategory)
	if err != nil {
		if errors.Is(err, categories.ErrCategoryNameExists) {
			webutils.ErrorJSON(w, err, http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to create category"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, createdCategory)
}

// GetAllCategories handles GET requests to list all categories.
// Often a public endpoint.
func (h *CategoryHandler) GetAllCategories(w http.ResponseWriter, r *http.Request) {
	categoryList, err := h.CategoryRepo.FindAll(r.Context())
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve categories"), http.StatusInternalServerError)
		return
	}
	webutils.WriteJSON(w, http.StatusOK, categoryList)
}

// GetCategory handles GET requests for a specific category by ID.
// Often a public endpoint.
func (h *CategoryHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		webutils.ErrorJSON(w, errors.New("category ID is required"), http.StatusBadRequest)
		return
	}

	categoryID, err := uuid.Parse(idStr)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid category ID format"), http.StatusBadRequest)
		return
	}

	category, err := h.CategoryRepo.FindByID(r.Context(), categoryID)
	if err != nil {
		if errors.Is(err, categories.ErrCategoryNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve category"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, category)
}

// UpdateCategory handles PUT requests to update an existing category.
// Typically requires admin authentication.
func (h *CategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		webutils.ErrorJSON(w, errors.New("category ID is required"), http.StatusBadRequest)
		return
	}

	categoryID, err := uuid.Parse(idStr)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid category ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateCategoryRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		webutils.ErrorJSON(w, errors.New("category name is required"), http.StatusBadRequest)
		return
	}

	categoryToUpdate := &models.Category{Name: req.Name}

	updatedCategory, err := h.CategoryRepo.Update(r.Context(), categoryID, categoryToUpdate)
	if err != nil {
		if errors.Is(err, categories.ErrCategoryNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else if errors.Is(err, categories.ErrCategoryNameExists) {
			webutils.ErrorJSON(w, err, http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update category"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, updatedCategory)
}

// DeleteCategory handles DELETE requests for a specific category.
// Typically requires admin authentication.
func (h *CategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		webutils.ErrorJSON(w, errors.New("category ID is required"), http.StatusBadRequest)
		return
	}

	categoryID, err := uuid.Parse(idStr)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid category ID format"), http.StatusBadRequest)
		return
	}

	err = h.CategoryRepo.Delete(r.Context(), categoryID)
	if err != nil {
		if errors.Is(err, categories.ErrCategoryNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			// Consider potential FK constraint issues if ON DELETE was different
			webutils.ErrorJSON(w, errors.New("failed to delete category"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204 No Content
}
