package handlers

import (
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/products"
	"bullet-commerce/internal/sourcing"
	"bullet-commerce/internal/variants"
	"bullet-commerce/internal/webutils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

// stockEndpointSunset is the retirement date advertised for the deprecated product-level stock
// endpoint (RFC 8594 Sunset header, HTTP-date). The per-variant endpoint is the successor.
const stockEndpointSunset = "Sat, 23 Jan 2027 00:00:00 GMT"

// stockWriteResult is the response of both stock-write paths. It names exactly which
// (variant, source) was touched — an admin thinking in product/variant terms needs to see where
// the write actually landed — plus the variant's aggregate availability across all sources.
type stockWriteResult struct {
	VariantID        uuid.UUID `json:"variant_id"`
	SourceID         uuid.UUID `json:"source_id"`
	StockAtSource    int       `json:"stock_at_source"`
	ReservedAtSource int       `json:"reserved_at_source"`
	StockAvailable   int       `json:"stock_available"`
}

// errNoVariants / ambiguousVariantError explain why the product-level stock endpoint cannot pick
// a target variant: a product with zero variants has nothing to stock, and one with several real
// variants has no unambiguous default — both steer the admin to the per-variant endpoint.
var errNoVariants = errors.New("product has no variants; create one via POST /products/{id}/variants before setting stock")

type ambiguousVariantError struct{ VariantIDs []string }

func (e *ambiguousVariantError) Error() string {
	return "product has multiple variants; set stock per variant via PATCH /products/{id}/variants/{variantId}/stock (candidates: " + strings.Join(e.VariantIDs, ", ") + ")"
}

// resolveDefaultVariant finds the unambiguous default variant for a product, mirroring the
// POST /cart/items resolution: the sole variant when there is exactly one, else the
// "default-<product_id>" SKU. Otherwise it returns ambiguousVariantError with the candidate ids.
func (h *ProductHandler) resolveDefaultVariant(ctx context.Context, productID uuid.UUID) (uuid.UUID, error) {
	vs, err := h.VariantRepo.FindByProductID(ctx, productID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(vs) == 0 {
		return uuid.Nil, errNoVariants
	}
	if len(vs) == 1 {
		return vs[0].ID, nil
	}
	defaultSKU := fmt.Sprintf("default-%s", productID)
	for _, v := range vs {
		if v.SKU == defaultSKU {
			return v.ID, nil
		}
	}
	ids := make([]string, len(vs))
	for i, v := range vs {
		ids[i] = v.ID.String()
	}
	return uuid.Nil, &ambiguousVariantError{VariantIDs: ids}
}

// writeVariantStock is the single place the stock invariant is applied. It validates the variant
// and target source (an explicit source must exist — 404, never created implicitly; otherwise the
// transparent default), performs the absolute SetStock UPSERT, and assembles the response with the
// variant's aggregate availability. Returns ok=false after already having written the error
// response. Both the per-variant and the deprecated product-level endpoints funnel through here.
func (h *ProductHandler) writeVariantStock(w http.ResponseWriter, r *http.Request, variantID uuid.UUID, srcOverride *uuid.UUID, stock int) (*stockWriteResult, bool) {
	if _, err := h.VariantRepo.FindByID(r.Context(), variantID); err != nil {
		if errors.Is(err, variants.ErrVariantNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate variant"), http.StatusInternalServerError)
		}
		return nil, false
	}

	var source *models.Source
	var err error
	if srcOverride != nil {
		if source, err = h.SourceRepo.GetByID(r.Context(), *srcOverride); err != nil {
			if errors.Is(err, sourcing.ErrSourceNotFound) {
				webutils.ErrorJSON(w, err, http.StatusNotFound)
			} else {
				webutils.ErrorJSON(w, errors.New("failed to resolve source"), http.StatusInternalServerError)
			}
			return nil, false
		}
	} else if source, err = h.SourceRepo.GetDefault(r.Context()); err != nil {
		webutils.ErrorJSON(w, errors.New("no default source configured"), http.StatusInternalServerError)
		return nil, false
	}

	atSource, reservedAtSource, err := h.VariantRepo.SetStock(r.Context(), variantID, source.ID, stock)
	if err != nil {
		var below *variants.StockBelowReservedError
		if errors.As(err, &below) {
			webutils.ErrorJSON(w, err, http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update stock"), http.StatusInternalServerError)
		}
		return nil, false
	}

	available, err := h.VariantRepo.AvailableForVariant(r.Context(), variantID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to read availability"), http.StatusInternalServerError)
		return nil, false
	}

	return &stockWriteResult{
		VariantID:        variantID,
		SourceID:         source.ID,
		StockAtSource:    atSource,
		ReservedAtSource: reservedAtSource,
		StockAvailable:   available,
	}, true
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		webutils.ErrorJSON(w, errors.New("product name is required"), http.StatusBadRequest)
		return
	}
	if req.PriceCents <= 0 {
		webutils.ErrorJSON(w, errors.New("product price_cents must be positive"), http.StatusBadRequest)
		return
	}
	if req.Stock < 0 {
		webutils.ErrorJSON(w, errors.New("stock cannot be negative"), http.StatusBadRequest)
		return
	}
	if !validProductType(req.Type) {
		webutils.ErrorJSON(w, errors.New("invalid product type"), http.StatusBadRequest)
		return
	}
	if err := validateCatalogFields(req.productCatalogFields); err != nil {
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
		return
	}

	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	product, err := h.ProductRepo.Create(r.Context(), &models.Product{
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
	})
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create product"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, product)
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

type CreateVariantRequest struct {
	SKU        string          `json:"sku"`
	Attributes json.RawMessage `json:"attributes,omitempty"`
	PriceCents *int64          `json:"price_cents,omitempty"`
	Currency   string          `json:"currency,omitempty"`
	Stock      int             `json:"stock"`
}

type UpdateVariantStockRequest struct {
	Stock int `json:"stock"`
	// SourceID is optional: omitted writes to the transparent default source (single-store),
	// matching POST /cart/items' optional variant_id. An explicit id must exist (404 otherwise).
	SourceID *uuid.UUID `json:"source_id,omitempty"`
}

// CreateVariant handles POST /api/products/{id}/variants (admin).
func (h *ProductHandler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	// Admin path: use the status-agnostic read so variants can be added to draft/archived
	// products (the public FindByID would 404 them).
	product, err := h.ProductRepo.FindByIDAdmin(r.Context(), productID)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate product"), http.StatusInternalServerError)
		}
		return
	}

	var req CreateVariantRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.SKU == "" {
		webutils.ErrorJSON(w, errors.New("sku is required"), http.StatusBadRequest)
		return
	}
	if req.Stock < 0 {
		webutils.ErrorJSON(w, errors.New("stock cannot be negative"), http.StatusBadRequest)
		return
	}
	if req.PriceCents != nil && *req.PriceCents < 0 {
		webutils.ErrorJSON(w, errors.New("price_cents cannot be negative"), http.StatusBadRequest)
		return
	}

	attributes := req.Attributes
	if len(attributes) == 0 {
		attributes = json.RawMessage(`{}`)
	}
	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	// Materialized price (PRD §4): an omitted price inherits the product's and is flagged so a
	// later product-price change can fan out to it; an explicit price is admin-set (not inherited).
	priceCents := product.PriceCents
	priceInherited := true
	if req.PriceCents != nil {
		priceCents = *req.PriceCents
		priceInherited = false
	}

	variant, err := h.VariantRepo.Create(r.Context(), &models.ProductVariant{
		ProductID:      productID,
		SKU:            req.SKU,
		Attributes:     attributes,
		PriceCents:     priceCents,
		PriceInherited: priceInherited,
		Currency:       currency,
		Stock:          req.Stock,
	})
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create variant"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, variant)
}

// UpdateVariantStock handles PATCH /api/products/{id}/variants/{variantId}/stock (admin).
func (h *ProductHandler) UpdateVariantStock(w http.ResponseWriter, r *http.Request) {
	variantID, err := uuid.Parse(mux.Vars(r)["variantId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid variant ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateVariantStockRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.Stock < 0 {
		webutils.ErrorJSON(w, errors.New("stock cannot be negative"), http.StatusBadRequest)
		return
	}

	result, ok := h.writeVariantStock(w, r, variantID, req.SourceID, req.Stock)
	if !ok {
		return
	}
	webutils.WriteJSON(w, http.StatusOK, result)
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

	if req.Name == "" || req.PriceCents < 0 {
		webutils.ErrorJSON(w, errors.New("product name is required and price_cents must be non-negative"), http.StatusBadRequest)
		return
	}
	if !validProductType(req.Type) {
		webutils.ErrorJSON(w, errors.New("invalid product type"), http.StatusBadRequest)
		return
	}
	if err := validateCatalogFields(req.productCatalogFields); err != nil {
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
		return
	}
	// Optimistic concurrency: the caller must echo the version it last read so a stale edit is
	// rejected (409) instead of silently clobbering a concurrent update.
	if req.Version <= 0 {
		webutils.ErrorJSON(w, errors.New("version is required and must be a positive integer"), http.StatusBadRequest)
		return
	}

	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	updated, err := h.ProductRepo.Update(r.Context(), productID, &models.Product{
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
	})
	if err != nil {
		switch {
		case errors.Is(err, products.ErrProductNotFound):
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		case errors.Is(err, products.ErrProductVersionConflict):
			webutils.ErrorJSON(w, err, http.StatusConflict)
		default:
			webutils.ErrorJSON(w, errors.New("failed to update product"), http.StatusInternalServerError)
		}
		return
	}

	webutils.WriteJSON(w, http.StatusOK, updated)
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

func (h *ProductHandler) UpdateStock(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := uuid.Parse(vars["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateStockRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.Stock < 0 {
		webutils.ErrorJSON(w, errors.New("stock cannot be negative"), http.StatusBadRequest)
		return
	}

	// DEPRECATED: product-level stock is ambiguous now that stock is held per (variant, source).
	// The endpoint is kept working by resolving the product's default variant and delegating to
	// the same per-source write as PATCH /products/{id}/variants/{variantId}/stock. Signal the
	// deprecation on every response (even errors) and log each call so retirement is observable.
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", stockEndpointSunset)
	w.Header().Set("Link", `</api/products/{id}/variants/{variantId}/stock>; rel="successor-version"`)
	adminID, _ := getAuthenticatedUserID(r)
	slog.Warn("deprecated product-level stock write", "product_id", productID, "user_id", adminID)

	variantID, err := h.resolveDefaultVariant(r.Context(), productID)
	if err != nil {
		var amb *ambiguousVariantError
		if errors.As(err, &amb) || errors.Is(err, errNoVariants) {
			webutils.ErrorJSON(w, err, http.StatusConflict)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to resolve default variant"), http.StatusInternalServerError)
		}
		return
	}

	// Product-level always targets the default source (the ambiguity it inherits is the variant,
	// resolved above; the source stays transparent, matching pre-sourcing behavior).
	result, ok := h.writeVariantStock(w, r, variantID, nil, req.Stock)
	if !ok {
		return
	}
	webutils.WriteJSON(w, http.StatusOK, result)
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
