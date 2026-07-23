package handlers

import (
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
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

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

// stockEndpointSunset is the retirement date advertised for the deprecated product-level stock
// endpoint (RFC 8594 Sunset header, HTTP-date). The per-variant endpoint is the successor.
const stockEndpointSunset = "Sat, 23 Jan 2027 00:00:00 GMT"

// stockWriteResult is the response of both stock-write paths. It names exactly which
// (variant, source) was touched - an admin thinking in product/variant terms needs to see where
// the write actually landed - plus the variant's aggregate availability across all sources.
type stockWriteResult struct {
	VariantID        uuid.UUID `json:"variant_id"`
	SourceID         uuid.UUID `json:"source_id"`
	StockAtSource    int       `json:"stock_at_source"`
	ReservedAtSource int       `json:"reserved_at_source"`
	StockAvailable   int       `json:"stock_available"`
}

// errNoVariants / ambiguousVariantError explain why the product-level stock endpoint cannot pick
// a target variant: a product with zero variants has nothing to stock, and one with several real
// variants has no unambiguous default - both steer the admin to the per-variant endpoint.
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
// and target source (an explicit source must exist - 404, never created implicitly; otherwise the
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

	source, ok := h.resolveStockSource(w, r, srcOverride)
	if !ok {
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

// resolveStockSource picks the (variant, source) write target: an explicit override must already
// exist (404, never created implicitly), otherwise the transparent default source. Returns ok=false
// after already having written the error response.
func (h *ProductHandler) resolveStockSource(w http.ResponseWriter, r *http.Request, srcOverride *uuid.UUID) (*models.Source, bool) {
	if srcOverride != nil {
		source, err := h.SourceRepo.GetByID(r.Context(), *srcOverride)
		if err != nil {
			if errors.Is(err, sourcing.ErrSourceNotFound) {
				webutils.ErrorJSON(w, err, http.StatusNotFound)
			} else {
				webutils.ErrorJSON(w, errors.New("failed to resolve source"), http.StatusInternalServerError)
			}
			return nil, false
		}
		return source, true
	}

	source, err := h.SourceRepo.GetDefault(r.Context())
	if err != nil {
		webutils.ErrorJSON(w, errors.New("no default source configured"), http.StatusInternalServerError)
		return nil, false
	}
	return source, true
}

// CreateVariant handles POST /api/products/{id}/variants (admin).
func (h *ProductHandler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	product, ok := h.loadProductForAdmin(w, r, productID)
	if !ok {
		return
	}

	var req CreateVariantRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if err := validateCreateVariantRequest(req); err != nil {
		webutils.ErrorJSON(w, err, http.StatusBadRequest)
		return
	}

	variant, err := h.VariantRepo.Create(r.Context(), buildVariantModel(productID, product, req))
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create variant"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, variant)
}

// loadProductForAdmin uses the status-agnostic read so variants can be added to draft/archived
// products (the public FindByID would 404 them). Returns ok=false after having written the response.
func (h *ProductHandler) loadProductForAdmin(w http.ResponseWriter, r *http.Request, productID uuid.UUID) (*models.Product, bool) {
	product, err := h.ProductRepo.FindByIDAdmin(r.Context(), productID)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate product"), http.StatusInternalServerError)
		}
		return nil, false
	}
	return product, true
}

// validateCreateVariantRequest gates the variant creation body (required SKU, non-negative stock
// and optional non-negative price).
func validateCreateVariantRequest(req CreateVariantRequest) error {
	if req.SKU == "" {
		return errors.New("sku is required")
	}
	if req.Stock < 0 {
		return errors.New("stock cannot be negative")
	}
	if req.PriceCents != nil && *req.PriceCents < 0 {
		return errors.New("price_cents cannot be negative")
	}
	return nil
}

// buildVariantModel assembles the variant to persist, applying the attribute/currency defaults and
// the materialized-price rule (PRD §4): an omitted price inherits the product's and is flagged so a
// later product-price change can fan out to it; an explicit price is admin-set (not inherited).
func buildVariantModel(productID uuid.UUID, product *models.Product, req CreateVariantRequest) *models.ProductVariant {
	attributes := req.Attributes
	if len(attributes) == 0 {
		attributes = json.RawMessage(`{}`)
	}
	currency := req.Currency
	if currency == "" {
		currency = models.DefaultCurrency
	}

	priceCents := product.PriceCents
	priceInherited := true
	if req.PriceCents != nil {
		priceCents = *req.PriceCents
		priceInherited = false
	}

	return &models.ProductVariant{
		ProductID:      productID,
		SKU:            req.SKU,
		Attributes:     attributes,
		PriceCents:     priceCents,
		PriceInherited: priceInherited,
		Currency:       currency,
		Stock:          req.Stock,
	}
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
