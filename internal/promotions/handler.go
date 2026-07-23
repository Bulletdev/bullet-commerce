package promotions

import (
	"context"

	"bullet-commerce/internal/models"
)

// VoucherHandler is the plugable PORT for promotion pricing. WHY a port instead of
// a rule engine in the core: campaign/coupon logic is a product concern that varies
// per deployment, so the core depends only on this interface and any implementation
// (an external promotions service, a rules engine, a stub) can be wired in later.
type VoucherHandler interface {
	// Apply resolves the discounts for a given subtotal and set of coupon codes.
	// Returning ([]models.AppliedDiscount, error) keeps the result-only contract:
	// the handler decides the reductions, the core just receives them.
	Apply(ctx context.Context, subtotalCents int64, couponCodes []string) ([]models.AppliedDiscount, error)
}

// NoopVoucherHandler is the default adapter: it applies no promotion at all. WHY a
// no-op default: the core can depend on VoucherHandler unconditionally, and a build
// without a real promotions provider still behaves correctly (nothing is discounted).
type NoopVoucherHandler struct{}

// Apply returns no discounts and no error.
func (NoopVoucherHandler) Apply(_ context.Context, _ int64, _ []string) ([]models.AppliedDiscount, error) {
	return nil, nil
}
