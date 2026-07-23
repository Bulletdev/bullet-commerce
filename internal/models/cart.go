package models

import (
	"time"

	"github.com/google/uuid"
)

type Cart struct {
	ID     uuid.UUID `json:"id" db:"id"`
	UserID uuid.UUID `json:"user_id" db:"user_id"`
	// AppliedCouponCodes are the promo codes the shopper attached to this cart. They are
	// re-validated and re-priced by the promotions port on every read, so the cart stores
	// only the codes, never the computed discount.
	AppliedCouponCodes []string  `json:"applied_coupon_codes" db:"applied_coupon_codes"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}
