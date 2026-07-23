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
	"log/slog"

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
	// Refund sentinels — handlers map these to 4xx/5xx.
	ErrOrderNotRefundable      = errors.New("order cannot be refunded in its current payment status")
	ErrRefundNotSupported      = errors.New("payment provider does not support refunds")
	ErrRefundAmountInvalid     = errors.New("refund amount exceeds the refundable balance")
	ErrRefundItemNotFound      = errors.New("refund item variant is not part of the order")
	ErrMissingPaymentReference = errors.New("order has no payment reference to refund against")
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
	// flips the main charge to paid — all atomically. Called by the payment webhook.
	ConfirmOrderPayment(ctx context.Context, orderID uuid.UUID) error
	// MarkPendingPayment records that a payment flow was started: it moves the order to
	// pending_payment and stores the PSP reference so a later webhook can be reconciled.
	MarkPendingPayment(ctx context.Context, orderID uuid.UUID, reference string) error
	ExpireOrphanedOrders(ctx context.Context) (int64, error)
	ExpireUnpaidOrders(ctx context.Context) (int64, error)
	// RefundOrder refunds a Paid order via the financial Refunder and, for each item flagged
	// restock, returns physical stock at the source it was claimed from — all in one tx.
	// amountCents == 0 means a full refund of the remaining refundable balance.
	RefundOrder(ctx context.Context, refunder payment.Refunder, orderID uuid.UUID, items []RefundItem, amountCents int64) error
	// Idempotency store for POST /api/orders (opt-in via the Idempotency-Key header).
	LookupIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*IdempotencyRecord, error)
	ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, requestHash string) (bool, error)
	SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, status int, body []byte, orderID uuid.UUID) error
	ReleaseIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) error
}

// RefundItem targets one order line for refund. Restock is opt-in per line: only flagged
// lines return physical stock (inverse of the Claim done at payment confirmation).
type RefundItem struct {
	VariantID uuid.UUID
	Qty       int
	Restock   bool
}

// variantStockRepo is the slice of the variant repository the order aggregate drives for its
// stock invariants. Declared locally (rather than depending on the whole
// variants.VariantRepository) so the order layer states exactly the operations it needs.
// Restock is the inverse of Claim — it adds physical stock back at a (variant, source) — and
// is implemented by the variants repository (catalog agent).
type variantStockRepo interface {
	Reserve(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Release(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Claim(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Restock(ctx context.Context, exec variants.DBExecutor, variantID, sourceID uuid.UUID, qty int) error
}

// OrderRefundedEvent is emitted once a refund is durably committed. Full reports whether the
// order's whole balance was refunded (vs a partial refund).
type OrderRefundedEvent struct {
	OrderID     uuid.UUID
	AmountCents int64
	Full        bool
}

func (OrderRefundedEvent) Name() string { return "order.refunded" }

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

func (r *postgresOrderRepository) CreateOrderFromCart(ctx context.Context, userID, cartID, shippingAddressID uuid.UUID, cartItems []models.CartItem, shippingCostCents int64, shippingMethod *string) (*models.Order, error) {
	if len(cartItems) == 0 {
		return nil, errors.New("cannot create order from empty cart")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Subtotal = sum of line items. Shipping is priced independently of items.
	var subtotalCents int64
	for _, item := range cartItems {
		subtotalCents += item.PriceCents * int64(item.Quantity)
	}

	// Freeze the cart's coupon discount at checkout: read the codes off the cart, let the
	// voucher port price them against the subtotal, and carry the reductions through so the
	// order total and the applied_discounts rows both reflect the frozen amount.
	var discounts []models.AppliedDiscount
	if r.voucher != nil {
		var couponCodes []string
		if err := tx.QueryRow(ctx, `SELECT applied_coupon_codes FROM carts WHERE id = $1`, cartID).Scan(&couponCodes); err != nil {
			return nil, err
		}
		discounts, err = r.voucher.Apply(ctx, subtotalCents, couponCodes)
		if err != nil {
			return nil, err
		}
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

	order := &models.Order{
		UserID:            userID,
		ShippingAddressID: shippingAddressID,
		TotalCents:        totalCents,
		ShippingCostCents: shippingCostCents,
		ShippingMethod:    shippingMethod,
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO orders (user_id, shipping_address_id, status, payment_status, total_cents, shipping_cost_cents, shipping_method)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, status, payment_status, currency, created_at, updated_at
	`, userID, shippingAddressID, models.StatusPending, models.PaymentUnpaid, totalCents, shippingCostCents, shippingMethod,
	).Scan(&order.ID, &order.Status, &order.PaymentStatus, &order.Currency, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Mirror the cart's default delivery onto the order (V1: the single default shipment).
	// Freight moves onto the delivery, but the order total above is unchanged — the order
	// still owns shipping_cost_cents, the delivery just records which shipment incurred it.
	var deliveryID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO deliveries (order_id, code, location_type, shipping_cost_cents, method)
		VALUES ($1, $2, 'address', $3, $4)
		RETURNING id
	`, order.ID, models.DefaultDeliveryCode, shippingCostCents, shippingMethod).Scan(&deliveryID); err != nil {
		return nil, err
	}

	// Ask the allocator which source each line ships from before reserving. V1 returns the
	// default source for every line, so this collapses to the pre-sourcing behavior; a future
	// multi-source allocator changes only the chosen source, not the reserve/insert path.
	sourceByVariant, err := r.allocateSources(ctx, cartItems)
	if err != nil {
		return nil, err
	}

	for _, item := range cartItems {
		sourceID := sourceByVariant[item.VariantID]
		// Reserve holds the stock for this order at the allocated source without decrementing
		// physical stock; the atomic guard inside Reserve makes concurrent checkouts safe. On
		// shortage we abort the whole order (tx rollback) so nothing is half-reserved.
		if err := r.variantRepo.Reserve(ctx, tx, item.VariantID, sourceID, item.Quantity); err != nil {
			return nil, err
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
			order.ID, item.ProductID, item.VariantID, item.Quantity, item.PriceCents, deliveryID, sourceID,
		); err != nil {
			return nil, err
		}
	}

	// Freeze each computed reduction onto the order in the SAME tx as the order/lines, so
	// the historical discount can never diverge from the order total.
	for _, d := range discounts {
		if _, err := tx.Exec(ctx,
			`INSERT INTO applied_discounts (order_id, level, type, applied_cents, coupon_code)
			 VALUES ($1, $2, $3, $4, $5)`,
			order.ID, d.Level, d.Type, d.AppliedCents, d.CouponCode,
		); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cartID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Redeem the coupons post-commit (non-fatal, like the main charge): a redemption-count
	// bump must not undo a placed order, so a failure here is logged, not returned. Each
	// distinct coupon is counted once per order.
	if r.couponRepo != nil {
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

	// Post-commit side effects. WHY here (not in the tx): ChargeRepository.Create runs on
	// the pool (no tx handle) and the event bus contract is "publish only durable facts",
	// so both must run AFTER the order is committed. A charge-insert failure must not undo
	// a placed order, so it is logged, not returned.
	if r.chargeRepo != nil {
		charge := &models.Charge{
			OrderID:     order.ID,
			Type:        models.ChargeMain,
			AmountCents: totalCents,
			Status:      "pending",
			Method:      "",
		}
		if err := r.chargeRepo.Create(ctx, charge); err != nil {
			slog.Error("failed to create main charge for order", "order_id", order.ID, "error", err)
		}
	}
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
		// Not found, soft-deleted, or already paid/failed — nothing to claim.
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
	// best-effort create that failed post-commit) is not fatal — chargeRef stays empty.
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
// it, so (a) concurrent cleanup goroutines/instances never contend on the same order —
// each skips rows another already locked — and (b) LIMIT is honored (a LIMIT + SKIP LOCKED
// directly inside an UPDATE can lock more rows than LIMIT during planning). SKIP LOCKED
// also means a row a concurrent ConfirmOrderPayment is mid-confirming is skipped, not
// clobbered. interval is a package constant, never user input — safe to interpolate.
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

type orderItemStock struct {
	variantID uuid.UUID
	sourceID  uuid.UUID
	quantity  int
}

// loadOrderItemStock reads (variant_id, source_id, quantity) for every line of an order — the
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

// RefundOrder refunds a Paid order. In ONE transaction it (1) calls the financial Refunder
// on the PSP, (2) flips payment_status to refunded/partially_refunded and stamps
// refunded_at + refund_amount_cents, (3) marks the main charge refunded, and (4) for each
// item flagged restock returns physical stock at the (variant, source) it was Claimed from —
// the inverse of ConfirmOrderPayment's Claim. WHY one tx: money-state and stock-state must
// never diverge, exactly as ConfirmOrderPayment couples the claim and the charge flip.
//
// Guard: only a Paid order can be refunded (a single refund op per order; a partial refund
// leaves the order 'partially_refunded' and is not refundable again by this method). The
// row is locked FOR UPDATE so a concurrent refund/confirm cannot race the guard.
func (r *postgresOrderRepository) RefundOrder(ctx context.Context, refunder payment.Refunder, orderID uuid.UUID, items []RefundItem, amountCents int64) error {
	if refunder == nil {
		return ErrRefundNotSupported
	}
	if amountCents < 0 {
		return ErrRefundAmountInvalid
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var paymentStatus models.PaymentStatus
	var paymentRef *string
	var totalCents, alreadyRefunded int64
	err = tx.QueryRow(ctx, `
		SELECT payment_status, payment_reference, total_cents, refund_amount_cents
		FROM orders
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, orderID).Scan(&paymentStatus, &paymentRef, &totalCents, &alreadyRefunded)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}

	// Only a settled (Paid) order can be refunded.
	if paymentStatus != models.PaymentPaid {
		return ErrOrderNotRefundable
	}
	if paymentRef == nil || *paymentRef == "" {
		return ErrMissingPaymentReference
	}

	// Resolve the amount: 0 means a full refund of the remaining balance.
	refundable := totalCents - alreadyRefunded
	if amountCents == 0 {
		amountCents = refundable
	}
	if amountCents <= 0 || amountCents > refundable {
		return ErrRefundAmountInvalid
	}
	full := amountCents >= refundable

	// Call the PSP refund first: if the money movement fails, the tx rolls back and neither
	// the status flip nor the restock is applied.
	if _, err := refunder.Refund(ctx, *paymentRef, payment.Money(amountCents)); err != nil {
		return err
	}

	newStatus := models.PaymentPartiallyRefunded
	if full {
		newStatus = models.PaymentRefunded
	}
	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET payment_status = $1, refunded_at = NOW(), refund_amount_cents = refund_amount_cents + $2, updated_at = NOW()
		WHERE id = $3
	`, newStatus, amountCents, orderID); err != nil {
		return err
	}

	// Restock each flagged line at the exact source it was claimed from. order_items.source_id
	// (added in 000020) records that pair, so the return targets the same (variant, source).
	for _, it := range items {
		if !it.Restock {
			continue
		}
		var sourceID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT source_id FROM order_items WHERE order_id = $1 AND variant_id = $2
		`, orderID, it.VariantID).Scan(&sourceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrRefundItemNotFound
			}
			return err
		}
		if err := r.variantRepo.Restock(ctx, tx, it.VariantID, sourceID, it.Qty); err != nil {
			return err
		}
	}

	// Flip the main charge to refunded in the same tx (mirrors ConfirmOrderPayment). A missing
	// main charge is not fatal — older orders may have none.
	if _, err := tx.Exec(ctx, `
		UPDATE payment_charges SET status = 'refunded'
		WHERE order_id = $1 AND type = 'main'
	`, orderID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if r.bus != nil {
		r.bus.Publish(ctx, OrderRefundedEvent{OrderID: orderID, AmountCents: amountCents, Full: full})
	}
	return nil
}
