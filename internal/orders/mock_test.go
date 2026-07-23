package orders

import (
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/events"
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewPostgresOrderRepository(t *testing.T) {
	repo := NewPostgresOrderRepository(nil, nil, nil, nil, nil, nil, nil)
	assert.NotNil(t, repo)
}

// fakeChargeRepo records the charges created so tests can assert the order's "main"
// charge without a real database.
type fakeChargeRepo struct {
	created []models.Charge
}

func (f *fakeChargeRepo) Create(ctx context.Context, charge *models.Charge) error {
	charge.ID = uuid.New()
	f.created = append(f.created, *charge)
	return nil
}

func (f *fakeChargeRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.Charge, error) {
	return nil, nil
}

// fakeBus records published events so tests can assert order.placed / payment.confirmed.
type fakeBus struct {
	published []events.Event
}

func (f *fakeBus) Subscribe(name string, h events.Handler) {}

func (f *fakeBus) Publish(ctx context.Context, e events.Event) {
	f.published = append(f.published, e)
}

// fakeVoucher is a canned VoucherHandler: it returns a fixed set of discounts so the
// freeze path can be tested without the real coupons table.
type fakeVoucher struct {
	discounts []models.AppliedDiscount
	err       error
}

func (f fakeVoucher) Apply(ctx context.Context, subtotalCents int64, codes []string) ([]models.AppliedDiscount, error) {
	return f.discounts, f.err
}

// fakeCouponRepo records IncrementUse calls so the redemption assertion can check the
// coupon was counted, and resolves codes to ids via byCode.
type fakeCouponRepo struct {
	byCode      map[string]*models.Coupon
	incremented []uuid.UUID
}

func (f *fakeCouponRepo) FindByCode(ctx context.Context, code string) (*models.Coupon, error) {
	if c, ok := f.byCode[code]; ok {
		return c, nil
	}
	return nil, coupons.ErrCouponNotFound
}

func (f *fakeCouponRepo) IncrementUse(ctx context.Context, id uuid.UUID) error {
	f.incremented = append(f.incremented, id)
	return nil
}
