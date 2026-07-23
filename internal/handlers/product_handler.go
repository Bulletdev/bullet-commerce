package handlers

import (
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/products"
	"bullet-commerce/internal/sourcing"
	"bullet-commerce/internal/variants"
	"bullet-commerce/internal/webutils"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type ProductHandler struct {
	ProductRepo products.ProductRepository
	VariantRepo variants.VariantRepository
	MediaRepo   media.MediaRepository
	SourceRepo  sourcing.SourceRepository
}

func NewProductHandler(productRepo products.ProductRepository, variantRepo variants.VariantRepository, mediaRepo media.MediaRepository, sourceRepo sourcing.SourceRepository) *ProductHandler {
	return &ProductHandler{ProductRepo: productRepo, VariantRepo: variantRepo, MediaRepo: mediaRepo, SourceRepo: sourceRepo}
}

// productCatalogFields groups the v2 enrichment fields (dims / fiscal / merchandising)
// shared by the create and update requests so the two stay in lockstep.
type productCatalogFields struct {
	WeightGrams         *int       `json:"weight_grams,omitempty"`
	LengthMM            *int       `json:"length_mm,omitempty"`
	WidthMM             *int       `json:"width_mm,omitempty"`
	HeightMM            *int       `json:"height_mm,omitempty"`
	NCM                 *string    `json:"ncm,omitempty"`
	CEST                *string    `json:"cest,omitempty"`
	Origem              int        `json:"origem,omitempty"`
	Unit                string     `json:"unit,omitempty"`
	Status              string     `json:"status,omitempty"`
	Slug                *string    `json:"slug,omitempty"`
	MetaTitle           *string    `json:"meta_title,omitempty"`
	MetaDescription     *string    `json:"meta_description,omitempty"`
	BrandID             *uuid.UUID `json:"brand_id,omitempty"`
	CompareAtPriceCents *int64     `json:"compare_at_price_cents,omitempty"`
}

type CreateProductRequest struct {
	Name                       string          `json:"name"`
	Description                string          `json:"description"`
	PriceCents                 int64           `json:"price_cents"`
	Currency                   string          `json:"currency"`
	CategoryID                 *uuid.UUID      `json:"category_id"`
	Stock                      int             `json:"stock"`
	Featured                   bool            `json:"featured"`
	Type                       string          `json:"type,omitempty"`
	Attributes                 json.RawMessage `json:"attributes,omitempty"`
	VariantVariationAttributes []string        `json:"variant_variation_attributes,omitempty"`
	productCatalogFields
}

type UpdateProductRequest struct {
	Name                       string          `json:"name"`
	Description                string          `json:"description"`
	PriceCents                 int64           `json:"price_cents"`
	Currency                   string          `json:"currency"`
	CategoryID                 *uuid.UUID      `json:"category_id"`
	Featured                   bool            `json:"featured"`
	Type                       string          `json:"type,omitempty"`
	Attributes                 json.RawMessage `json:"attributes,omitempty"`
	VariantVariationAttributes []string        `json:"variant_variation_attributes,omitempty"`
	// Version is the client's expected product version for optimistic concurrency; it must be the
	// value last read (>= 1). A stale value collides with a concurrent edit and returns 409.
	Version int `json:"version"`
	productCatalogFields
}

// validProductStatus gates the status field when a client supplies one; an empty status is
// allowed and normalized to "active" by the repository.
func validProductStatus(s string) bool {
	switch s {
	case "", models.ProductStatusDraft, models.ProductStatusActive, models.ProductStatusArchived:
		return true
	default:
		return false
	}
}

// validProductTypes gates the type field when a client supplies one; an empty type
// is allowed and normalized to "simple" by the repository.
func validProductType(t string) bool {
	switch t {
	case "", models.ProductTypeSimple, models.ProductTypeConfigurable, models.ProductTypeBundle:
		return true
	default:
		return false
	}
}

// validateCatalogFields checks the v2 enrichment fields the DB also constrains, so a bad
// request fails with 400 rather than a 500 from a CHECK violation deeper in the repo.
func validateCatalogFields(f productCatalogFields) error {
	if !validProductStatus(f.Status) {
		return errors.New("invalid product status")
	}
	if f.Origem < 0 || f.Origem > 8 {
		return errors.New("origem must be between 0 and 8")
	}
	if f.CompareAtPriceCents != nil && *f.CompareAtPriceCents < 0 {
		return errors.New("compare_at_price_cents cannot be negative")
	}
	return nil
}

type UpdateStockRequest struct {
	Stock int `json:"stock"`
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if err := validateCreateProductRequest(req); err != nil {
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
		return
	}

	product, err := h.ProductRepo.Create(r.Context(), buildCreateProductModel(req))
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create product"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, product)
}

// validateCreateProductRequest gates the create body (required name, positive price, non-negative
// stock, known type) plus the shared catalog-field constraints. The order matches the original
// per-field checks so the same first failing field is reported.
func validateCreateProductRequest(req CreateProductRequest) error {
	if req.Name == "" {
		return errors.New("product name is required")
	}
	if req.PriceCents <= 0 {
		return errors.New("product price_cents must be positive")
	}
	if req.Stock < 0 {
		return errors.New("stock cannot be negative")
	}
	if !validProductType(req.Type) {
		return errors.New("invalid product type")
	}
	return validateCatalogFields(req.productCatalogFields)
}

// buildCreateProductModel assembles the product to persist, defaulting an omitted currency.
func buildCreateProductModel(req CreateProductRequest) *models.Product {
	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	return &models.Product{
		Name:                       req.Name,
		Description:                req.Description,
		PriceCents:                 req.PriceCents,
		Currency:                   currency,
		CategoryID:                 req.CategoryID,
		Stock:                      req.Stock,
		Featured:                   req.Featured,
		Type:                       req.Type,
		Attributes:                 req.Attributes,
		VariantVariationAttributes: req.VariantVariationAttributes,
		WeightGrams:                req.WeightGrams,
		LengthMM:                   req.LengthMM,
		WidthMM:                    req.WidthMM,
		HeightMM:                   req.HeightMM,
		NCM:                        req.NCM,
		CEST:                       req.CEST,
		Origem:                     req.Origem,
		Unit:                       req.Unit,
		Status:                     req.Status,
		Slug:                       req.Slug,
		MetaTitle:                  req.MetaTitle,
		MetaDescription:            req.MetaDescription,
		BrandID:                    req.BrandID,
		CompareAtPriceCents:        req.CompareAtPriceCents,
	}
}

func (h *ProductHandler) GetAllProducts(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	list, err := h.ProductRepo.FindAll(r.Context(), limit, offset)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve products"), http.StatusInternalServerError)
		return
	}
	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *ProductHandler) GetFeaturedProducts(w http.ResponseWriter, r *http.Request) {
	list, err := h.ProductRepo.FindFeatured(r.Context())
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve featured products"), http.StatusInternalServerError)
		return
	}
	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *ProductHandler) GetProductsByCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	categoryID, err := uuid.Parse(vars["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid category ID format"), http.StatusBadRequest)
		return
	}

	limit, offset := parsePagination(r)
	list, err := h.ProductRepo.FindByCategoryID(r.Context(), categoryID, limit, offset)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve products"), http.StatusInternalServerError)
		return
	}
	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *ProductHandler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		webutils.ErrorJSON(w, errors.New("search query parameter 'q' is required"), http.StatusBadRequest)
		return
	}

	limit, offset := parsePagination(r)
	list, err := h.ProductRepo.Search(r.Context(), query, limit, offset)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to search products"), http.StatusInternalServerError)
		return
	}
	webutils.WriteJSON(w, http.StatusOK, list)
}

func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := uuid.Parse(vars["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	product, err := h.ProductRepo.FindByID(r.Context(), productID)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to retrieve product"), http.StatusInternalServerError)
		}
		return
	}

	// The sellable units live on the variants, so a product detail must expose them.
	vs, err := h.VariantRepo.FindByProductID(r.Context(), productID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve product variants"), http.StatusInternalServerError)
		return
	}
	if vs == nil {
		vs = []models.ProductVariant{}
	}

	// The gallery (product- and variant-scoped) and the N:N secondary category memberships
	// round out the product read model; both are separate stores from products, so fetch them
	// alongside. The primary category stays on the embedded product's category_id.
	mediaList, err := h.MediaRepo.ListByProduct(r.Context(), productID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve product media"), http.StatusInternalServerError)
		return
	}
	if mediaList == nil {
		mediaList = []models.ProductMedia{}
	}

	categoryIDs, err := h.ProductRepo.FindCategoryIDs(r.Context(), productID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve product categories"), http.StatusInternalServerError)
		return
	}
	if categoryIDs == nil {
		categoryIDs = []uuid.UUID{}
	}

	webutils.WriteJSON(w, http.StatusOK, ProductWithVariants{
		Product:     product,
		Variants:    vs,
		Media:       mediaList,
		CategoryIDs: categoryIDs,
	})
}

// ProductWithVariants embeds the product so its fields stay top-level (unchanged shape)
// while adding the variant list, media gallery and secondary category ids.
type ProductWithVariants struct {
	*models.Product
	Variants    []models.ProductVariant `json:"variants"`
	Media       []models.ProductMedia   `json:"media"`
	CategoryIDs []uuid.UUID             `json:"category_ids"`
}

func (h *ProductHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := uuid.Parse(vars["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateProductRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if err := validateUpdateProductRequest(req); err != nil {
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
		return
	}

	updated, err := h.ProductRepo.Update(r.Context(), productID, buildUpdateProductModel(req))
	if err != nil {
		writeProductUpdateError(w, err)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, updated)
}

// validateUpdateProductRequest gates the update body. The check order matches the original so the
// same first failing rule is reported: name/price, then type, then catalog fields, then version.
func validateUpdateProductRequest(req UpdateProductRequest) error {
	if req.Name == "" || req.PriceCents < 0 {
		return errors.New("product name is required and price_cents must be non-negative")
	}
	if !validProductType(req.Type) {
		return errors.New("invalid product type")
	}
	if err := validateCatalogFields(req.productCatalogFields); err != nil {
		return err
	}
	// Optimistic concurrency: the caller must echo the version it last read so a stale edit is
	// rejected (409) instead of silently clobbering a concurrent update.
	if req.Version <= 0 {
		return errors.New("version is required and must be a positive integer")
	}
	return nil
}

// buildUpdateProductModel assembles the product update payload, defaulting an omitted currency.
func buildUpdateProductModel(req UpdateProductRequest) *models.Product {
	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	return &models.Product{
		Name:                       req.Name,
		Description:                req.Description,
		PriceCents:                 req.PriceCents,
		Currency:                   currency,
		CategoryID:                 req.CategoryID,
		Featured:                   req.Featured,
		Type:                       req.Type,
		Attributes:                 req.Attributes,
		VariantVariationAttributes: req.VariantVariationAttributes,
		Version:                    req.Version,
		WeightGrams:                req.WeightGrams,
		LengthMM:                   req.LengthMM,
		WidthMM:                    req.WidthMM,
		HeightMM:                   req.HeightMM,
		NCM:                        req.NCM,
		CEST:                       req.CEST,
		Origem:                     req.Origem,
		Unit:                       req.Unit,
		Status:                     req.Status,
		Slug:                       req.Slug,
		MetaTitle:                  req.MetaTitle,
		MetaDescription:            req.MetaDescription,
		BrandID:                    req.BrandID,
		CompareAtPriceCents:        req.CompareAtPriceCents,
	}
}

// writeProductUpdateError maps a repository update failure to its response (404 for a missing
// product, 409 for a stale version, 500 otherwise).
func writeProductUpdateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, products.ErrProductNotFound):
		webutils.ErrorJSON(w, err, http.StatusNotFound)
	case errors.Is(err, products.ErrProductVersionConflict):
		webutils.ErrorJSON(w, err, http.StatusConflict)
	default:
		webutils.ErrorJSON(w, errors.New("failed to update product"), http.StatusInternalServerError)
	}
}

func (h *ProductHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := uuid.Parse(vars["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	if err := h.ProductRepo.Delete(r.Context(), productID); err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to delete product"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parsePagination reads limit/offset from query params with safe defaults.
func parsePagination(r *http.Request) (limit, offset int) {
	limit = 20
	offset = 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}
