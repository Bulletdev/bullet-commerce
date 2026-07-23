package promotions

import (
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/models"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

func newCoupon(code, typ string, value int64) *models.Coupon {
	return &models.Coupon{ID: uuid.New(), Code: code, DiscountType: typ, Value: value, Active: true}
}

func TestCouponHandler_Percent(t *testing.T) {
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "SAVE10").Return(newCoupon("SAVE10", models.CouponPercent, 10), nil)

	h := NewCouponHandler(repo)
	ds, err := h.Apply(context.Background(), 10000, []string{"SAVE10"})
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, int64(-1000), ds[0].AppliedCents) // 10% of 10000
	assert.Equal(t, models.DiscountLevelCart, ds[0].Level)
	assert.Equal(t, "SAVE10", ds[0].CouponCode)
}

func TestCouponHandler_Fixed(t *testing.T) {
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "MINUS5").Return(newCoupon("MINUS5", models.CouponFixed, 500), nil)

	h := NewCouponHandler(repo)
	ds, err := h.Apply(context.Background(), 10000, []string{"MINUS5"})
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, int64(-500), ds[0].AppliedCents)
}

func TestCouponHandler_FixedClampedToSubtotal(t *testing.T) {
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "BIG").Return(newCoupon("BIG", models.CouponFixed, 99999), nil)

	h := NewCouponHandler(repo)
	ds, err := h.Apply(context.Background(), 3000, []string{"BIG"})
	require.NoError(t, err)
	require.Len(t, ds, 1)
	// A fixed coupon never discounts more than the cart is worth.
	assert.Equal(t, int64(-3000), ds[0].AppliedCents)
}

func TestCouponHandler_NoCodes(t *testing.T) {
	h := NewCouponHandler(&coupons.MockCouponRepository{})
	ds, err := h.Apply(context.Background(), 10000, nil)
	require.NoError(t, err)
	assert.Empty(t, ds)
}

func TestCouponHandler_NotFound(t *testing.T) {
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "NOPE").Return(nil, coupons.ErrCouponNotFound)

	h := NewCouponHandler(repo)
	_, err := h.Apply(context.Background(), 10000, []string{"NOPE"})
	require.Error(t, err)
}

func TestCouponHandler_Inactive(t *testing.T) {
	c := newCoupon("OFF", models.CouponPercent, 10)
	c.Active = false
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "OFF").Return(c, nil)

	h := NewCouponHandler(repo)
	_, err := h.Apply(context.Background(), 10000, []string{"OFF"})
	require.Error(t, err)
}

func TestCouponHandler_Expired(t *testing.T) {
	c := newCoupon("OLD", models.CouponPercent, 10)
	past := time.Now().Add(-time.Hour)
	c.ExpiresAt = &past
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "OLD").Return(c, nil)

	h := NewCouponHandler(repo)
	_, err := h.Apply(context.Background(), 10000, []string{"OLD"})
	require.Error(t, err)
}

func TestCouponHandler_BelowMinCart(t *testing.T) {
	c := newCoupon("MIN", models.CouponFixed, 500)
	c.MinCartCents = 20000
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "MIN").Return(c, nil)

	h := NewCouponHandler(repo)
	_, err := h.Apply(context.Background(), 10000, []string{"MIN"})
	require.Error(t, err)
}

func TestCouponHandler_MaxUsesReached(t *testing.T) {
	c := newCoupon("CAP", models.CouponFixed, 500)
	c.MaxUses = intPtr(5)
	c.UsedCount = 5
	repo := &coupons.MockCouponRepository{}
	repo.On("FindByCode", context.Background(), "CAP").Return(c, nil)

	h := NewCouponHandler(repo)
	_, err := h.Apply(context.Background(), 10000, []string{"CAP"})
	require.Error(t, err)
}

// The no-op default must keep returning nothing so a build without a real promotions
// provider still prices carts correctly.
func TestNoopVoucherHandler(t *testing.T) {
	ds, err := NoopVoucherHandler{}.Apply(context.Background(), 10000, []string{"ANY"})
	require.NoError(t, err)
	assert.Nil(t, ds)
}
