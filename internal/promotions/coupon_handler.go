package promotions

import (
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/money"
	"context"
	"errors"
	"fmt"
	"time"
)

// CouponHandler is the REAL VoucherHandler adapter: it resolves each coupon code against
// the coupons repository, validates it against the current subtotal, and returns the
// computed cart-level reductions. WHY it lives behind the port: the core (cart/order)
// depends only on VoucherHandler, so swapping the no-op for this real handler is pure
// wiring in main - no core code changes.
type CouponHandler struct {
	repo coupons.CouponRepository
}

func NewCouponHandler(repo coupons.CouponRepository) *CouponHandler {
	return &CouponHandler{repo: repo}
}

// Apply validates every code and returns one cart-level AppliedDiscount per valid coupon.
// WHY fail the whole call on the first invalid code (rather than skip it): the shopper
// explicitly attached that code, so silently dropping it would misprice the cart without
// telling them; the caller surfaces the error so the code can be rejected/removed.
func (h *CouponHandler) Apply(ctx context.Context, subtotalCents int64, couponCodes []string) ([]models.AppliedDiscount, error) {
	if len(couponCodes) == 0 {
		return nil, nil
	}

	out := make([]models.AppliedDiscount, 0, len(couponCodes))
	for _, code := range couponCodes {
		c, err := h.resolveValidCoupon(ctx, subtotalCents, code)
		if err != nil {
			return nil, err
		}

		discount := computeDiscountCents(subtotalCents, c)
		out = append(out, models.AppliedDiscount{
			Level: models.DiscountLevelCart,
			Type:  c.DiscountType,
			// AppliedCents is negative by contract: a discount subtracts from the total.
			AppliedCents: -discount,
			CouponCode:   code,
		})
	}
	return out, nil
}

// resolveValidCoupon looks up a single code and runs every eligibility check against
// the current subtotal. It returns the coupon only when it is valid to apply; any
// failure yields the shopper-facing error Apply propagates verbatim (same first-invalid
// -> fail-the-call contract, unchanged messages and ordering).
func (h *CouponHandler) resolveValidCoupon(ctx context.Context, subtotalCents int64, code string) (*models.Coupon, error) {
	c, err := h.repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, coupons.ErrCouponNotFound) {
			return nil, fmt.Errorf("coupon %q is not valid", code)
		}
		return nil, err
	}
	if !c.Active {
		return nil, fmt.Errorf("coupon %q is not active", code)
	}
	if c.ExpiresAt != nil && time.Now().After(*c.ExpiresAt) {
		return nil, fmt.Errorf("coupon %q has expired", code)
	}
	if subtotalCents < c.MinCartCents {
		return nil, fmt.Errorf("coupon %q requires a minimum cart of %d cents", code, c.MinCartCents)
	}
	if c.MaxUses != nil && c.UsedCount >= *c.MaxUses {
		return nil, fmt.Errorf("coupon %q has reached its usage limit", code)
	}
	return c, nil
}

// computeDiscountCents returns the (positive) reduction a coupon yields on a subtotal.
func computeDiscountCents(subtotalCents int64, c *models.Coupon) int64 {
	switch c.DiscountType {
	case models.CouponPercent:
		// money.Allocate splits the subtotal by weights [value, 100-value], so the first
		// share is the coupon's cut with largest-remainder rounding - exact cents, no float.
		parts := money.New(subtotalCents, "").Allocate([]int64{c.Value, 100 - c.Value})
		if len(parts) == 0 {
			return 0
		}
		return parts[0].Cents
	case models.CouponFixed:
		// A fixed coupon never pays out more than the cart is worth.
		if c.Value > subtotalCents {
			return subtotalCents
		}
		return c.Value
	}
	return 0
}
