package models

import (
	"time"

	"github.com/google/uuid"
)

// Coupon discount kinds. WHY string constants (not an enum type): the value mirrors the
// DB CHECK ('percent','fixed') and flows straight into AppliedDiscount.Type, which the
// core keeps as a plain string so it never has to know the full promotion catalogue.
const (
	CouponPercent = "percent"
	CouponFixed   = "fixed"
)

// Coupon is the RULE behind a cart-level promotion. WHY the pointers on MaxUses/ExpiresAt:
// both are optional constraints — a nil MaxUses means unlimited redemptions and a nil
// ExpiresAt means the coupon never expires, which a zero value could not distinguish.
type Coupon struct {
	ID   uuid.UUID `json:"id" db:"id"`
	Code string    `json:"code" db:"code"`
	// DiscountType is CouponPercent or CouponFixed and dictates how Value is read.
	DiscountType string `json:"discount_type" db:"discount_type"`
	// Value is a percentage (0..100) when DiscountType is percent, or an absolute amount
	// in minor units (cents) when fixed.
	Value int64 `json:"value" db:"value"`
	// MinCartCents is the smallest subtotal the coupon applies to (0 = no minimum).
	MinCartCents int64      `json:"min_cart_cents" db:"min_cart_cents"`
	MaxUses      *int       `json:"max_uses" db:"max_uses"`
	UsedCount    int        `json:"used_count" db:"used_count"`
	ExpiresAt    *time.Time `json:"expires_at" db:"expires_at"`
	Active       bool       `json:"active" db:"active"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}
