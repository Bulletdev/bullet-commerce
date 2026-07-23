package handlers

import (
	// For UserIDContextKey
	"bullet-commerce/internal/cart" // Cart Repository
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/products"   // Product Repository
	"bullet-commerce/internal/promotions" // VoucherHandler port (coupon pricing)
	"bullet-commerce/internal/variants"   // Variant Repository (stock owner)
	"bullet-commerce/internal/webutils"   // JSON Helpers
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// CartHandler handles cart-related requests.
type CartHandler struct {
	CartRepo    cart.CartRepository
	ProductRepo products.ProductRepository // Needed to get current price on add
	VariantRepo variants.VariantRepository // Resolves the sellable variant + its price
	// Voucher prices the coupon codes on the cart. The handler depends on the port, not a
	// concrete promotions provider, so the no-op or the real coupon handler wire in identically.
	Voucher promotions.VoucherHandler
}

// NewCartHandler creates a new CartHandler.
func NewCartHandler(cartRepo cart.CartRepository, productRepo products.ProductRepository, variantRepo variants.VariantRepository, voucher promotions.VoucherHandler) *CartHandler {
	return &CartHandler{
		CartRepo:    cartRepo,
		ProductRepo: productRepo,
		VariantRepo: variantRepo,
		Voucher:     voucher,
	}
}

// --- Request/Response Structs ---

type AddCartItemRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	// VariantID is optional: when omitted the product's default variant is used.
	VariantID *uuid.UUID `json:"variant_id,omitempty"`
	Quantity  int        `json:"quantity"`
}

type UpdateCartItemRequest struct {
	Quantity int `json:"quantity"`
}

type CartCouponRequest struct {
	Code string `json:"code"`
}

// CartResponse includes the cart and its items.
type CartResponse struct {
	Cart  models.Cart       `json:"cart"`
	Items []models.CartItem `json:"items"`
	// DiscountCents is the total reduction from applied coupons (>= 0), and TotalCents is
	// subtotal + freight − discount, floored at 0. Freight is 0 at the cart stage (quoted
	// later at checkout), so TotalCents = subtotal − discount here.
	DiscountCents int64 `json:"discount_cents"`
	TotalCents    int64 `json:"total_cents"` // Calculated total, minor units
}

// --- Handlers ---

// getOrCreateUserCart is a helper to get the cart for the authenticated user.
func (h *CartHandler) getOrCreateUserCart(w http.ResponseWriter, r *http.Request) (*models.Cart, bool) {
	authUserID, err := getAuthenticatedUserID(r) // Use helper from user_handler
	if err != nil {
		webutils.ErrorJSON(w, err, http.StatusInternalServerError)
		return nil, false
	}

	userCart, err := h.CartRepo.GetOrCreateCartByUserID(r.Context(), authUserID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to get or create cart"), http.StatusInternalServerError)
		return nil, false
	}
	return userCart, true
}

// GetCart handles GET /api/cart
func (h *CartHandler) GetCart(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	items, err := h.CartRepo.GetCartItems(r.Context(), userCart.ID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart items"), http.StatusInternalServerError)
		return
	}

	// Subtotal from line items, then let the voucher port price the attached coupons.
	var subtotalCents int64
	for _, item := range items {
		subtotalCents += item.PriceCents * int64(item.Quantity)
	}

	discountCents := h.cartDiscount(r, userCart, subtotalCents)
	totalCents := subtotalCents - discountCents
	if totalCents < 0 {
		totalCents = 0
	}

	resp := CartResponse{
		Cart:          *userCart,
		Items:         items,
		DiscountCents: discountCents,
		TotalCents:    totalCents,
	}

	webutils.WriteJSON(w, http.StatusOK, resp)
}

// cartDiscount re-prices the cart's coupon codes against the current subtotal and returns
// the positive discount magnitude. WHY errors are swallowed here (0 discount): GetCart is
// a read; a stored coupon that no longer validates (expired, min no longer met) must not
// break cart display - it simply stops discounting until removed or re-added.
func (h *CartHandler) cartDiscount(r *http.Request, userCart *models.Cart, subtotalCents int64) int64 {
	if h.Voucher == nil || len(userCart.AppliedCouponCodes) == 0 {
		return 0
	}
	discounts, err := h.Voucher.Apply(r.Context(), subtotalCents, userCart.AppliedCouponCodes)
	if err != nil {
		return 0
	}
	var discountCents int64
	for _, d := range discounts {
		// AppliedCents is negative by contract; accumulate its magnitude.
		discountCents -= d.AppliedCents
	}
	return discountCents
}

// resolveVariant picks the variant to buy. WHY here: the cart line identity is the
// variant, so a request without variant_id must resolve the product's default variant
// (sku "default-<product_id>", or the sole variant if there is exactly one).
func (h *CartHandler) resolveVariant(r *http.Request, product *models.Product, requested *uuid.UUID) (*models.ProductVariant, error) {
	if requested != nil {
		v, err := h.VariantRepo.FindByID(r.Context(), *requested)
		if err != nil {
			return nil, err
		}
		if v.ProductID != product.ID {
			return nil, errors.New("variant does not belong to product")
		}
		return v, nil
	}

	vs, err := h.VariantRepo.FindByProductID(r.Context(), product.ID)
	if err != nil {
		return nil, err
	}
	if len(vs) == 0 {
		return nil, variants.ErrVariantNotFound
	}
	if len(vs) == 1 {
		return &vs[0], nil
	}
	defaultSKU := fmt.Sprintf("default-%s", product.ID)
	for i := range vs {
		if vs[i].SKU == defaultSKU {
			return &vs[i], nil
		}
	}
	return nil, errors.New("no default variant; variant_id is required")
}

// AddItem handles POST /api/cart/items
func (h *CartHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	var req AddCartItemRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Quantity <= 0 {
		webutils.ErrorJSON(w, errors.New("quantity must be positive"), http.StatusBadRequest)
		return
	}

	// Validate product exists and get its current price
	product, err := h.ProductRepo.FindByID(r.Context(), req.ProductID)
	if err != nil {
		if errors.Is(err, products.ErrProductNotFound) {
			webutils.ErrorJSON(w, errors.New("product not found"), http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to validate product"), http.StatusInternalServerError)
		}
		return
	}

	variant, err := h.resolveVariant(r, product, req.VariantID)
	if err != nil {
		if errors.Is(err, variants.ErrVariantNotFound) {
			webutils.ErrorJSON(w, errors.New("variant not found"), http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, err, http.StatusBadRequest)
		}
		return
	}

	// Variant price is materialized (NOT NULL): it already carries the inherited or admin-set
	// price, so no product fallback is needed.
	priceCents := variant.PriceCents

	cartItem, err := h.CartRepo.AddItem(r.Context(), userCart.ID, req.ProductID, variant.ID, req.Quantity, priceCents)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to add item to cart"), http.StatusInternalServerError)
		return
	}

	// Return 201 Created on success
	webutils.WriteJSON(w, http.StatusCreated, cartItem)
}

// UpdateItem handles PUT /api/cart/items/{variantId}
func (h *CartHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	vars := mux.Vars(r)
	variantID, err := uuid.Parse(vars["variantId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid variant ID format"), http.StatusBadRequest)
		return
	}

	var req UpdateCartItemRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}

	if req.Quantity <= 0 {
		// Treat quantity 0 or less as a removal request
		h.DeleteItem(w, r) // Reuse DeleteItem handler logic
		return
	}

	_, err = h.CartRepo.UpdateItemQuantity(r.Context(), userCart.ID, variantID, req.Quantity)
	if err != nil {
		if errors.Is(err, cart.ErrProductNotInCart) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to update cart item"), http.StatusInternalServerError)
		}
		return
	}

	// Return the updated cart content
	h.GetCart(w, r)
}

// DeleteItem handles DELETE /api/cart/items/{variantId}
func (h *CartHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	vars := mux.Vars(r)
	variantID, err := uuid.Parse(vars["variantId"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid variant ID format"), http.StatusBadRequest)
		return
	}

	err = h.CartRepo.RemoveItem(r.Context(), userCart.ID, variantID)
	if err != nil {
		if errors.Is(err, cart.ErrProductNotInCart) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to remove cart item"), http.StatusInternalServerError)
		}
		return
	}

	// Return the updated cart content
	h.GetCart(w, r)
}

// AddCoupon handles POST /api/cart/coupon - validates the code against the current
// subtotal via the voucher port before persisting it, so an invalid code is rejected
// (400) and never attached.
func (h *CartHandler) AddCoupon(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	var req CartCouponRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		webutils.ErrorJSON(w, errors.New("coupon code is required"), http.StatusBadRequest)
		return
	}

	items, err := h.CartRepo.GetCartItems(r.Context(), userCart.ID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to retrieve cart items"), http.StatusInternalServerError)
		return
	}
	var subtotalCents int64
	for _, item := range items {
		subtotalCents += item.PriceCents * int64(item.Quantity)
	}

	// Validate before persisting: reject the code if the voucher port refuses it.
	if h.Voucher != nil {
		if _, err := h.Voucher.Apply(r.Context(), subtotalCents, []string{req.Code}); err != nil {
			webutils.ErrorJSON(w, err, http.StatusBadRequest)
			return
		}
	}

	if _, err := h.CartRepo.AddCouponCode(r.Context(), userCart.ID, req.Code); err != nil {
		webutils.ErrorJSON(w, errors.New("failed to apply coupon"), http.StatusInternalServerError)
		return
	}

	// Return the re-priced cart.
	h.GetCart(w, r)
}

// RemoveCoupon handles DELETE /api/cart/coupon/{code}
func (h *CartHandler) RemoveCoupon(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	code := mux.Vars(r)["code"]
	if code == "" {
		webutils.ErrorJSON(w, errors.New("coupon code is required"), http.StatusBadRequest)
		return
	}

	if _, err := h.CartRepo.RemoveCouponCode(r.Context(), userCart.ID, code); err != nil {
		webutils.ErrorJSON(w, errors.New("failed to remove coupon"), http.StatusInternalServerError)
		return
	}

	h.GetCart(w, r)
}

// ClearCart handles DELETE /api/cart
func (h *CartHandler) ClearCart(w http.ResponseWriter, r *http.Request) {
	userCart, ok := h.getOrCreateUserCart(w, r)
	if !ok {
		return
	}

	err := h.CartRepo.ClearCart(r.Context(), userCart.ID)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to clear cart"), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204 No Content
}
