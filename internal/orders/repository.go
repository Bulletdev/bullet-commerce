package orders

import (
	"bullet-commerce/internal/charges"
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/events"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/promotions"
	"bullet-commerce/internal/sourcing"
	"bullet-commerce/internal/variants"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

var (
	ErrOrderNotFound           = errors.New("order not found")
	ErrOrderCannotBeCancelled  = errors.New("order cannot be cancelled in its current status")
	ErrInvalidStatusTransition = errors.New("invalid order status transition")
	// ErrInsufficientStock is returned when a variant cannot satisfy the requested
	// quantity at order creation. It wraps variants.ErrInsufficientStock so handlers can
	// match either sentinel.
	ErrInsufficientStock = variants.ErrInsufficientStock
)

// Expiry windows for the two cleanup passes. WHY two windows: an order that reached
// pending_payment (a charge was created) gets a longer grace period than one still
// unpaid (checkout abandoned), but both must release their reservation.
const (
	orphanedPaymentInterval = "INTERVAL '30 minutes'"
	unpaidInterval          = "INTERVAL '15 minutes'"
)

type OrderRepository interface {
	CreateOrderFromCart(ctx context.Context, userID, cartID, shippingAddressID uuid.UUID, cartItems []models.CartItem, shippingCostCents int64, shippingMethod *string) (*models.Order, error)
	FindUserOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Order, error)
	FindOrderByID(ctx context.Context, orderID uuid.UUID) (*models.Order, []models.OrderItem, error)
	UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, currentStatus, nextStatus models.OrderStatus) error
	UpdateOrderTracking(ctx context.Context, orderID uuid.UUID, trackingNumber string) error
	// CancelOrder transitions an order to cancelled and, for not-yet-paid orders,
	// releases each item's reservation in the SAME transaction.
	CancelOrder(ctx context.Context, orderID uuid.UUID) error
	// ConfirmOrderPayment marks an order paid+processing, claims each item's stock, and
	// flips the main charge to paid - all atomically. Called by the payment webhook.
	ConfirmOrderPayment(ctx context.Context, orderID uuid.UUID) error
	// MarkPendingPayment records that a payment flow was started: it moves the order to
	// pending_payment and stores the PSP reference so a later webhook can be reconciled.
	MarkPendingPayment(ctx context.Context, orderID uuid.UUID, reference string) error
	ExpireOrphanedOrders(ctx context.Context) (int64, error)
	ExpireUnpaidOrders(ctx context.Context) (int64, error)
	// RefundOrder refunds a Paid order via the financial Refunder and, for each item flagged
	// restock, returns physical stock at the source it was claimed from - all in one tx.
	// amountCents == 0 means a full refund of the remaining refundable balance.
	RefundOrder(ctx context.Context, refunder payment.Refunder, orderID uuid.UUID, items []RefundItem, amountCents int64) error
	// Idempotency store for POST /api/orders (opt-in via the Idempotency-Key header).
	LookupIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*IdempotencyRecord, error)
	ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, requestHash string) (bool, error)
	SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, status int, body []byte, orderID uuid.UUID) error
	ReleaseIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) error
}

// variantStockRepo is the slice of the variant repository the order aggregate drives for its
// stock invariants. Declared locally (rather than depending on the whole
// variants.VariantRepository) so the order layer states exactly the operations it needs.
// Restock is the inverse of Claim - it adds physical stock back at a (variant, source) - and
// is implemented by the variants repository (catalog agent).
type variantStockRepo interface {
	Reserve(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Release(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Claim(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Restock(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
}

type postgresOrderRepository struct {
	db          DBPool
	variantRepo variantStockRepo
	chargeRepo  charges.ChargeRepository
	bus         events.Bus
	// voucher prices the cart's coupon codes at checkout and couponRepo redeems them.
	// Both are optional: a nil voucher means no promotion is frozen (the pre-coupon behavior).
	voucher    promotions.VoucherHandler
	couponRepo coupons.CouponRepository
	// allocator decides which stock source each line is reserved from. V1 is a
	// SingleSourceAllocator (everything from the default source), so sourcing is transparent.
	allocator sourcing.Allocator
}

// NewPostgresOrderRepository wires the order aggregate to the variant repository so
// stock invariants (Reserve/Claim/Release) run inside the order's transaction, plus the
// charge repository (the order's "main" charge) and the event bus (order.placed /
// payment.confirmed are published AFTER the owning transaction commits). The voucher
// handler + coupon repository freeze the cart's coupon discount onto the order and redeem
// the coupons (used_count) at creation time. The allocator chooses each line's stock source
// (V1: the default source), so Reserve/Claim/Release run against the right (variant, source).
func NewPostgresOrderRepository(db *pgxpool.Pool, variantRepo variants.VariantRepository, chargeRepo charges.ChargeRepository, bus events.Bus, voucher promotions.VoucherHandler, couponRepo coupons.CouponRepository, allocator sourcing.Allocator) OrderRepository {
	return &postgresOrderRepository{db: db, variantRepo: variantRepo, chargeRepo: chargeRepo, bus: bus, voucher: voucher, couponRepo: couponRepo, allocator: allocator}
}

// CreateOrderFromCart is a short transaction orchestrator: it freezes the pricing (subtotal,
// coupon discount, total), inserts the order + default delivery, reserves and freezes each
// line, clears the cart, and commits. The coupon redemption, the main charge, and the
// order.placed event are best-effort side effects that run only AFTER the commit.
func (r *postgresOrderRepository) CreateOrderFromCart(ctx context.Context, userID, cartID, shippingAddressID uuid.UUID, cartItems []models.CartItem, shippingCostCents int64, shippingMethod *string) (*models.Order, error) {
	if len(cartItems) == 0 {
		return nil, errors.New("cannot create order from empty cart")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	subtotalCents := sumLineItems(cartItems)

	discounts, err := r.freezeCartDiscounts(ctx, tx, cartID, subtotalCents)
	if err != nil {
		return nil, err
	}
	var discountCents int64
	for _, d := range discounts {
		discountCents += d.AppliedCents // negative by contract
	}

	// Total = subtotal + shipping − discount, never below zero.
	totalCents := subtotalCents + shippingCostCents + discountCents
	if totalCents < 0 {
		totalCents = 0
	}

	order, err := r.insertOrder(ctx, tx, userID, shippingAddressID, totalCents, shippingCostCents, shippingMethod)
	if err != nil {
		return nil, err
	}

	deliveryID, err := r.insertDefaultDelivery(ctx, tx, order.ID, shippingCostCents, shippingMethod)
	if err != nil {
		return nil, err
	}

	if err := r.reserveCartLines(ctx, tx, order.ID, deliveryID, cartItems); err != nil {
		return nil, err
	}

	if err := r.freezeAppliedDiscounts(ctx, tx, order.ID, discounts); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cartID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	r.redeemCoupons(ctx, discounts)
	r.createMainCharge(ctx, order.ID, totalCents)
	if r.bus != nil {
		r.bus.Publish(ctx, events.OrderPlacedEvent{OrderID: order.ID})
	}

	return order, nil
}

func (r *postgresOrderRepository) FindUserOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Order, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, shipping_address_id, status, payment_status,
		       payment_method, payment_reference, total_cents, shipping_cost_cents, shipping_method, currency, tracking_number, created_at, updated_at
		FROM orders
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orderList []models.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orderList = append(orderList, *o)
	}
	return orderList, rows.Err()
}

func (r *postgresOrderRepository) FindOrderByID(ctx context.Context, orderID uuid.UUID) (*models.Order, []models.OrderItem, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, shipping_address_id, status, payment_status,
		       payment_method, payment_reference, total_cents, shipping_cost_cents, shipping_method, currency, tracking_number, created_at, updated_at
		FROM orders
		WHERE id = $1 AND deleted_at IS NULL
	`, orderID)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, order_id, product_id, variant_id, delivery_id, quantity, price_cents,
		       COALESCE(product_name, ''), COALESCE(variant_sku, ''), created_at, updated_at
		FROM order_items
		WHERE order_id = $1
		ORDER BY created_at ASC
	`, orderID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var items []models.OrderItem
	for rows.Next() {
		var item models.OrderItem
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.ProductID, &item.VariantID, &item.DeliveryID,
			&item.Quantity, &item.PriceCents, &item.ProductName, &item.VariantSKU,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}

	return order, items, rows.Err()
}

// scanRow is satisfied by both pgx.Row and pgx.Rows, so scanOrder serves single-row and
// multi-row reads with one column list.
type scanRow interface {
	Scan(dest ...any) error
}

func scanOrder(row scanRow) (*models.Order, error) {
	o := &models.Order{}
	err := row.Scan(
		&o.ID, &o.UserID, &o.ShippingAddressID, &o.Status, &o.PaymentStatus,
		&o.PaymentMethod, &o.PaymentReference, &o.TotalCents, &o.ShippingCostCents, &o.ShippingMethod,
		&o.Currency, &o.TrackingNumber, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (r *postgresOrderRepository) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, currentStatus, nextStatus models.OrderStatus) error {
	if !currentStatus.CanTransitionTo(nextStatus) {
		return ErrInvalidStatusTransition
	}

	result, err := r.db.Exec(ctx, `
		UPDATE orders SET status = $1, updated_at = NOW()
		WHERE id = $2 AND status = $3 AND deleted_at IS NULL
	`, nextStatus, orderID, currentStatus)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		// Either not found or currentStatus no longer matches (concurrent update).
		return ErrOrderNotFound
	}
	return nil
}

func (r *postgresOrderRepository) UpdateOrderTracking(ctx context.Context, orderID uuid.UUID, trackingNumber string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE orders SET tracking_number = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, trackingNumber, orderID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (r *postgresOrderRepository) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status models.OrderStatus
	var paymentStatus models.PaymentStatus
	err = tx.QueryRow(ctx, `
		SELECT status, payment_status FROM orders
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, orderID).Scan(&status, &paymentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}

	if !status.CanTransitionTo(models.StatusCancelled) {
		return ErrInvalidStatusTransition
	}

	if _, err := tx.Exec(ctx, `
		UPDATE orders SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, models.StatusCancelled, orderID); err != nil {
		return err
	}

	// Only release for NOT-YET-PAID orders: a paid order already consumed its
	// reservation via Claim, so releasing would double-count stock.
	if paymentStatus == models.PaymentUnpaid || paymentStatus == models.PaymentPending {
		if err := r.releaseOrderItems(ctx, tx, orderID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *postgresOrderRepository) ConfirmOrderPayment(ctx context.Context, orderID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Guarded transition: only unpaid/pending_payment orders can be confirmed, so a
	// duplicate webhook cannot claim stock twice.
	result, err := tx.Exec(ctx, `
		UPDATE orders SET payment_status = $1, status = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		  AND payment_status IN ($4, $5)
	`, models.PaymentPaid, models.StatusProcessing, orderID, models.PaymentUnpaid, models.PaymentPending)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		// Not found, soft-deleted, or already paid/failed - nothing to claim.
		return ErrOrderNotFound
	}

	items, err := r.loadOrderItemStock(ctx, tx, orderID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := r.variantRepo.Claim(ctx, tx, it.variantID, it.sourceID, it.quantity); err != nil {
			return err
		}
	}

	// Flip the main charge to paid in the SAME tx as the claim so money-state and
	// stock-state can never diverge. ChargeRepository exposes no tx-aware status update,
	// so this is an inline UPDATE on its table. A missing main charge (older order, or a
	// best-effort create that failed post-commit) is not fatal - chargeRef stays empty.
	var chargeRef string
	err = tx.QueryRow(ctx, `
		UPDATE payment_charges SET status = 'paid'
		WHERE order_id = $1 AND type = 'main'
		RETURNING COALESCE(reference, '')
	`, orderID).Scan(&chargeRef)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if r.bus != nil {
		r.bus.Publish(ctx, events.PaymentConfirmedEvent{OrderID: orderID, ChargeRef: chargeRef})
	}
	return nil
}

// MarkPendingPayment moves an order into pending_payment and records the PSP reference.
// The guard (only unpaid/pending_payment) makes it safe to call again on retry and keeps
// a paid/cancelled order from being dragged back into an open payment flow.
func (r *postgresOrderRepository) MarkPendingPayment(ctx context.Context, orderID uuid.UUID, reference string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE orders SET payment_status = $1, payment_reference = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL
		  AND payment_status IN ($4, $5)
	`, models.PaymentPending, reference, orderID, models.PaymentUnpaid, models.PaymentPending)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// ExpireOrphanedOrders cancels pending_payment orders older than 30 minutes with no
// payment reference, releasing each item's reservation in the same transaction.
func (r *postgresOrderRepository) ExpireOrphanedOrders(ctx context.Context) (int64, error) {
	return r.expireOrders(ctx, models.PaymentPending, orphanedPaymentInterval)
}

// ExpireUnpaidOrders cancels unpaid orders older than 15 minutes with no payment
// reference (abandoned checkout), releasing each item's reservation.
func (r *postgresOrderRepository) ExpireUnpaidOrders(ctx context.Context) (int64, error) {
	return r.expireOrders(ctx, models.PaymentUnpaid, unpaidInterval)
}

// expireOrders claims a bounded batch of eligible orders and cancels them, releasing each
// item's reservation in the SAME transaction so the status flip and the stock release
// commit atomically. WHY the CTE + FOR UPDATE SKIP LOCKED + LIMIT shape (PRD §6 #9): the
// SELECT ... FOR UPDATE SKIP LOCKED LIMIT lives inside the CTE and the UPDATE references
// it, so (a) concurrent cleanup goroutines/instances never contend on the same order -
// each skips rows another already locked - and (b) LIMIT is honored (a LIMIT + SKIP LOCKED
// directly inside an UPDATE can lock more rows than LIMIT during planning). SKIP LOCKED
// also means a row a concurrent ConfirmOrderPayment is mid-confirming is skipped, not
// clobbered. interval is a package constant, never user input - safe to interpolate.
func (r *postgresOrderRepository) expireOrders(ctx context.Context, paymentStatus models.PaymentStatus, interval string) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH batch AS (
			SELECT id FROM orders
			WHERE payment_status = $1
			  AND payment_reference IS NULL
			  AND created_at < NOW() - `+interval+`
			  AND deleted_at IS NULL
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 100
		)
		UPDATE orders o
		SET status = $2, payment_status = $3, updated_at = NOW()
		FROM batch WHERE o.id = batch.id
		RETURNING o.id
	`, paymentStatus, models.StatusCancelled, models.PaymentFailed)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	// Fully drain the RETURNING rows before issuing further queries on the same tx.
	rows.Close()

	for _, id := range ids {
		if err := r.releaseOrderItems(ctx, tx, id); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

type orderItemStock struct {
	variantID uuid.UUID
	sourceID  uuid.UUID
	quantity  int
}

// loadOrderItemStock reads (variant_id, source_id, quantity) for every line of an order - the
// source_id is what lets Release/Claim free the exact (variant, source) that was reserved. Rows
// are fully drained before the caller issues further queries on the same tx.
func (r *postgresOrderRepository) loadOrderItemStock(ctx context.Context, exec variants.DBExecutor, orderID uuid.UUID) ([]orderItemStock, error) {
	rows, err := exec.Query(ctx, `SELECT variant_id, source_id, quantity FROM order_items WHERE order_id = $1`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []orderItemStock
	for rows.Next() {
		var it orderItemStock
		if err := rows.Scan(&it.variantID, &it.sourceID, &it.quantity); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *postgresOrderRepository) releaseOrderItems(ctx context.Context, exec variants.DBExecutor, orderID uuid.UUID) error {
	items, err := r.loadOrderItemStock(ctx, exec, orderID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := r.variantRepo.Release(ctx, exec, it.variantID, it.sourceID, it.quantity); err != nil {
			return err
		}
	}
	return nil
}
