package orders

import (
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/sourcing"
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// sumLineItems totals the cart's line items. Shipping is priced independently of items.
func sumLineItems(cartItems []models.CartItem) int64 {
	var subtotalCents int64
	for _, item := range cartItems {
		subtotalCents += item.PriceCents * int64(item.Quantity)
	}
	return subtotalCents
}

// freezeCartDiscounts reads the cart's coupon codes and prices them against the subtotal via
// the voucher port, returning the reductions to carry through so the order total and the
// applied_discounts rows both reflect the frozen amount. A nil voucher means no promotion is
// frozen (the pre-coupon behavior).
func (r *postgresOrderRepository) freezeCartDiscounts(ctx context.Context, tx pgx.Tx, cartID uuid.UUID, subtotalCents int64) ([]models.AppliedDiscount, error) {
	if r.voucher == nil {
		return nil, nil
	}
	var couponCodes []string
	if err := tx.QueryRow(ctx, `SELECT applied_coupon_codes FROM carts WHERE id = $1`, cartID).Scan(&couponCodes); err != nil {
		return nil, err
	}
	return r.voucher.Apply(ctx, subtotalCents, couponCodes)
}

// insertOrder inserts the order header and returns it hydrated with the DB-assigned id and
// the server-defaulted status/currency/timestamps.
func (r *postgresOrderRepository) insertOrder(ctx context.Context, tx pgx.Tx, userID, shippingAddressID uuid.UUID, totalCents, shippingCostCents int64, shippingMethod *string) (*models.Order, error) {
	order := &models.Order{
		UserID:            userID,
		ShippingAddressID: shippingAddressID,
		TotalCents:        totalCents,
		ShippingCostCents: shippingCostCents,
		ShippingMethod:    shippingMethod,
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO orders (user_id, shipping_address_id, status, payment_status, total_cents, shipping_cost_cents, shipping_method)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, status, payment_status, currency, created_at, updated_at
	`, userID, shippingAddressID, models.StatusPending, models.PaymentUnpaid, totalCents, shippingCostCents, shippingMethod,
	).Scan(&order.ID, &order.Status, &order.PaymentStatus, &order.Currency, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return order, nil
}

// insertDefaultDelivery mirrors the cart's default delivery onto the order (V1: the single
// default shipment). Freight moves onto the delivery, but the order total is unchanged - the
// order still owns shipping_cost_cents, the delivery just records which shipment incurred it.
func (r *postgresOrderRepository) insertDefaultDelivery(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, shippingCostCents int64, shippingMethod *string) (uuid.UUID, error) {
	var deliveryID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO deliveries (order_id, code, location_type, shipping_cost_cents, method)
		VALUES ($1, $2, 'address', $3, $4)
		RETURNING id
	`, orderID, models.DefaultDeliveryCode, shippingCostCents, shippingMethod).Scan(&deliveryID); err != nil {
		return uuid.Nil, err
	}
	return deliveryID, nil
}

// reserveCartLines allocates a source per line, reserves stock there, and freezes each line
// onto the order in the SAME tx. On shortage it returns the error so the caller rolls the
// whole order back (nothing half-reserved).
func (r *postgresOrderRepository) reserveCartLines(ctx context.Context, tx pgx.Tx, orderID, deliveryID uuid.UUID, cartItems []models.CartItem) error {
	// Ask the allocator which source each line ships from before reserving. V1 returns the
	// default source for every line, so this collapses to the pre-sourcing behavior; a future
	// multi-source allocator changes only the chosen source, not the reserve/insert path.
	sourceByVariant, err := r.allocateSources(ctx, cartItems)
	if err != nil {
		return err
	}

	for _, item := range cartItems {
		sourceID := sourceByVariant[item.VariantID]
		// Reserve holds the stock for this order at the allocated source without decrementing
		// physical stock; the atomic guard inside Reserve makes concurrent checkouts safe. On
		// shortage we abort the whole order (tx rollback) so nothing is half-reserved.
		if err := r.variantRepo.Reserve(ctx, tx, item.VariantID, sourceID, item.Quantity); err != nil {
			return err
		}
		// INSERT ... SELECT freezes the product name and variant SKU from the catalog at
		// purchase time in a single atomic statement (no extra round trip), so a later
		// rename/soft-delete never rewrites this line. $3 (variant id) doubles as the join key.
		// source_id ($7) records where it was reserved so Release/Claim free the same pair.
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, variant_id, delivery_id, source_id, quantity, price_cents, product_name, variant_sku)
			 SELECT $1, $2, $3, $6, $7, $4, $5, p.name, v.sku
			 FROM product_variants v JOIN products p ON p.id = v.product_id
			 WHERE v.id = $3`,
			orderID, item.ProductID, item.VariantID, item.Quantity, item.PriceCents, deliveryID, sourceID,
		); err != nil {
			return err
		}
	}
	return nil
}

// freezeAppliedDiscounts writes each computed reduction onto the order in the SAME tx as the
// order/lines, so the historical discount can never diverge from the order total.
func (r *postgresOrderRepository) freezeAppliedDiscounts(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, discounts []models.AppliedDiscount) error {
	for _, d := range discounts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO applied_discounts (order_id, level, type, applied_cents, coupon_code)
			 VALUES ($1, $2, $3, $4, $5)`,
			orderID, d.Level, d.Type, d.AppliedCents, d.CouponCode,
		); err != nil {
			return err
		}
	}
	return nil
}

// allocateSources runs the allocator over the cart's lines and returns the chosen source per
// variant. WHY keyed by variant: cart_items are unique per variant within a cart, so one source
// per variant is a faithful mapping for V1's single-source allocator.
func (r *postgresOrderRepository) allocateSources(ctx context.Context, cartItems []models.CartItem) (map[uuid.UUID]uuid.UUID, error) {
	allocItems := make([]sourcing.AllocItem, len(cartItems))
	for i, item := range cartItems {
		allocItems[i] = sourcing.AllocItem{VariantID: item.VariantID, Qty: item.Quantity}
	}
	allocations, err := r.allocator.Allocate(ctx, allocItems)
	if err != nil {
		return nil, err
	}
	sourceByVariant := make(map[uuid.UUID]uuid.UUID, len(allocations))
	for _, a := range allocations {
		sourceByVariant[a.VariantID] = a.SourceID
	}
	return sourceByVariant, nil
}

// redeemCoupons bumps each distinct coupon's redemption count post-commit (non-fatal, like
// the main charge): a redemption-count bump must not undo a placed order, so a failure here
// is logged, not returned. Each distinct coupon is counted once per order.
func (r *postgresOrderRepository) redeemCoupons(ctx context.Context, discounts []models.AppliedDiscount) {
	if r.couponRepo == nil {
		return
	}
	seen := make(map[string]bool, len(discounts))
	for _, d := range discounts {
		if d.CouponCode == "" || seen[d.CouponCode] {
			continue
		}
		seen[d.CouponCode] = true
		coupon, err := r.couponRepo.FindByCode(ctx, d.CouponCode)
		if err != nil {
			slog.Error("failed to load coupon for redemption", "code", d.CouponCode, "error", err)
			continue
		}
		if err := r.couponRepo.IncrementUse(ctx, coupon.ID); err != nil {
			slog.Error("failed to increment coupon usage", "code", d.CouponCode, "error", err)
		}
	}
}

// createMainCharge records the order's "main" charge post-commit. WHY here (not in the tx):
// ChargeRepository.Create runs on the pool (no tx handle) and the event bus contract is
// "publish only durable facts", so both must run AFTER the order is committed. A charge-insert
// failure must not undo a placed order, so it is logged, not returned.
func (r *postgresOrderRepository) createMainCharge(ctx context.Context, orderID uuid.UUID, totalCents int64) {
	if r.chargeRepo == nil {
		return
	}
	charge := &models.Charge{
		OrderID:     orderID,
		Type:        models.ChargeMain,
		AmountCents: totalCents,
		Status:      "pending",
		Method:      "",
	}
	if err := r.chargeRepo.Create(ctx, charge); err != nil {
		slog.Error("failed to create main charge for order", "order_id", orderID, "error", err)
	}
}
