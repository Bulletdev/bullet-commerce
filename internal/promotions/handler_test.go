package promotions

import (
	"context"
	"testing"

	"bullet-commerce/internal/models"
)

// NoopVoucherHandler must never apply a promotion: nil discounts, nil error.
func TestNoopVoucherHandler_Apply_Empty(t *testing.T) {
	var h VoucherHandler = NoopVoucherHandler{}

	got, err := h.Apply(context.Background(), 10_000, []string{"WELCOME10", "FRETEGRATIS"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no discounts, got %d", len(got))
	}
}

// MergeDiscounts aggregates the (negative) cents per CampaignCode.
func TestMergeDiscounts_AggregatesByCampaign(t *testing.T) {
	ds := []models.AppliedDiscount{
		{CampaignCode: "BLACKFRIDAY", AppliedCents: -500, IsItemRelated: true},
		{CampaignCode: "BLACKFRIDAY", AppliedCents: -300, IsItemRelated: true},
		{CampaignCode: "FRETEGRATIS", AppliedCents: -1200, Level: models.DiscountLevelShipping},
	}

	got := models.MergeDiscounts(ds)

	if len(got) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(got))
	}
	if got["BLACKFRIDAY"] != -800 {
		t.Errorf("BLACKFRIDAY: expected -800, got %d", got["BLACKFRIDAY"])
	}
	if got["FRETEGRATIS"] != -1200 {
		t.Errorf("FRETEGRATIS: expected -1200, got %d", got["FRETEGRATIS"])
	}
}

// MergeDiscounts on an empty slice yields an empty (non-nil) map.
func TestMergeDiscounts_Empty(t *testing.T) {
	got := models.MergeDiscounts(nil)
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

// A discount reduces a total: AppliedCents is negative, so folding it into a
// subtotal must shrink it.
func TestAppliedDiscount_NegativeReducesSubtotal(t *testing.T) {
	subtotal := int64(10_000)
	d := models.AppliedDiscount{
		Level:        models.DiscountLevelCart,
		Type:         "fixed",
		AppliedCents: -2_500,
		CampaignCode: "PROMO",
	}

	if d.AppliedCents >= 0 {
		t.Fatalf("AppliedCents must be negative, got %d", d.AppliedCents)
	}
	if subtotal+d.AppliedCents != 7_500 {
		t.Errorf("expected 7500 after discount, got %d", subtotal+d.AppliedCents)
	}
}
