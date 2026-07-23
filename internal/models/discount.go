package models

import (
	"time"

	"github.com/google/uuid"
)

// DiscountLevel names where in the pricing breakdown a discount lands. WHY an
// explicit level: the same money reduction is presented and refunded differently
// depending on whether it hit a line item, the delivery fee, the whole cart, or
// shipping — the level is what lets downstream code attribute the reduction.
type DiscountLevel string

const (
	DiscountLevelItem     DiscountLevel = "item"
	DiscountLevelDelivery DiscountLevel = "delivery"
	DiscountLevelCart     DiscountLevel = "cart"
	DiscountLevelShipping DiscountLevel = "shipping"
)

// AppliedDiscount is the RESULT of a promotion decision, never the rule that
// produced it. WHY the core only carries the result: the rule engine lives behind
// a plugable port (see internal/promotions), so the core stays free of campaign
// logic and simply loads/persists what a handler already computed.
type AppliedDiscount struct {
	ID    uuid.UUID     `json:"id" db:"id"`
	Level DiscountLevel `json:"level" db:"level"`
	// Type is the reduction shape: "percent" or "fixed". Kept as a string so the
	// core does not need to know the full catalogue of promotion kinds.
	Type string `json:"type" db:"type"`
	// AppliedCents is ALWAYS NEGATIVE: a discount subtracts from the total, so
	// summing it into a subtotal reduces it without any special-casing.
	AppliedCents int64 `json:"applied_cents" db:"applied_cents"`
	// IsItemRelated marks reductions tied to line items (vs. cart/shipping level),
	// so refunds and tax reasoning can split item value from freight value.
	IsItemRelated bool `json:"is_item_related" db:"is_item_related"`
	// SortOrder fixes presentation and application order when several discounts stack.
	SortOrder    int       `json:"sort_order" db:"sort_order"`
	CampaignCode string    `json:"campaign_code" db:"campaign_code"`
	CouponCode   string    `json:"coupon_code" db:"coupon_code"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// MergeDiscounts aggregates applied discounts by CampaignCode, returning the total
// cents reduced per campaign. WHY aggregate by campaign: several lines can be hit
// by one campaign, and reporting/analytics reason per campaign, not per line.
// Values stay negative because AppliedCents is negative.
func MergeDiscounts(ds []AppliedDiscount) map[string]int64 {
	totals := make(map[string]int64)
	for _, d := range ds {
		totals[d.CampaignCode] += d.AppliedCents
	}
	return totals
}
