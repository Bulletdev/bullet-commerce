package orders

import (
	"bullet-commerce/internal/events"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/sourcing"
	"bullet-commerce/internal/variants"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDefaultSource is the source the SingleSourceAllocator routes every line to in these
// tests, so Reserve/Claim/Release expectations can assert the (variant, source) pair.
var testDefaultSource = uuid.New()

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresOrderRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	// A real (pool-less) variant repo issues its stock SQL against the passed executor
	// (the tx), so pgxmock intercepts Reserve/Claim/Release exactly like any other query.
	// The fake charge repo and bus record the post-commit side effects (main charge +
	// order.placed / payment.confirmed) without a real DB or dispatcher. The single-source
	// allocator routes every line to testDefaultSource.
	return db, &postgresOrderRepository{
		db:          db,
		variantRepo: variants.NewPostgresVariantRepository(nil),
		chargeRepo:  &fakeChargeRepo{},
		bus:         &fakeBus{},
		allocator:   sourcing.NewSingleSourceAllocator(testDefaultSource),
	}
}

func orderCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "user_id", "shipping_address_id", "status", "payment_status",
		"payment_method", "payment_reference", "total_cents", "shipping_cost_cents", "shipping_method", "currency", "tracking_number", "created_at", "updated_at",
	})
}

func addOrderRow(rows *pgxmock.Rows, id, userID uuid.UUID) *pgxmock.Rows {
	return rows.AddRow(
		id, userID, uuid.New(), "pending", "unpaid",
		nil, nil, int64(9990), int64(0), nil, "BRL", nil, time.Now(), time.Now(),
	)
}

func TestFindOrderByID_Found(t *testing.T) {
	db, repo := newMock(t)
	orderID, userID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, shipping_address_id")).
		WithArgs(orderID).
		WillReturnRows(addOrderRow(orderCols(), orderID, userID))

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, order_id, product_id")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "order_id", "product_id", "variant_id", "delivery_id", "quantity", "price_cents", "created_at", "updated_at"}))

	order, items, err := repo.FindOrderByID(context.Background(), orderID)
	require.NoError(t, err)
	assert.Equal(t, orderID, order.ID)
	assert.Empty(t, items)
	assert.NoError(t, db.ExpectationsWereMet())
}

// Given an order whose two items ship on two DISTINCT deliveries, When it is read back,
// Then each order_item carries its own delivery_id - the schema/read path supports N
// deliveries per order, not just the transparent default.
func TestFindOrderByID_MultiDeliveryItems(t *testing.T) {
	db, repo := newMock(t)
	orderID, userID := uuid.New(), uuid.New()
	deliveryA, deliveryB := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, shipping_address_id")).
		WithArgs(orderID).
		WillReturnRows(addOrderRow(orderCols(), orderID, userID))

	cols := []string{"id", "order_id", "product_id", "variant_id", "delivery_id", "quantity", "price_cents", "product_name", "variant_sku", "created_at", "updated_at"}
	db.ExpectQuery(regexp.QuoteMeta("SELECT id, order_id, product_id")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow(uuid.New(), orderID, uuid.New(), uuid.New(), deliveryA, 1, int64(1000), "A", "sku-a", time.Now(), time.Now()).
			AddRow(uuid.New(), orderID, uuid.New(), uuid.New(), deliveryB, 2, int64(2000), "B", "sku-b", time.Now(), time.Now()))

	_, items, err := repo.FindOrderByID(context.Background(), orderID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, deliveryA, items[0].DeliveryID)
	assert.Equal(t, deliveryB, items[1].DeliveryID)
	assert.NotEqual(t, items[0].DeliveryID, items[1].DeliveryID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindOrderByID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, shipping_address_id")).
		WithArgs(orderID).
		WillReturnRows(orderCols())

	_, _, err := repo.FindOrderByID(context.Background(), orderID)
	assert.ErrorIs(t, err, ErrOrderNotFound)
}

func TestFindUserOrders(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()
	orderID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE user_id = $1")).
		WithArgs(userID, 20, 0).
		WillReturnRows(addOrderRow(orderCols(), orderID, userID))

	list, err := repo.FindUserOrders(context.Background(), userID, 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, orderID, list[0].ID)
}

func TestUpdateOrderStatus_ValidTransition(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status")).
		WithArgs(models.StatusProcessing, orderID, models.StatusPending).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := repo.UpdateOrderStatus(context.Background(), orderID, models.StatusPending, models.StatusProcessing)
	assert.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestUpdateOrderStatus_InvalidTransition(t *testing.T) {
	_, repo := newMock(t)

	// pending → shipped is forbidden by the state machine - no DB call should be made
	err := repo.UpdateOrderStatus(context.Background(), uuid.New(), models.StatusPending, models.StatusShipped)
	assert.ErrorIs(t, err, ErrInvalidStatusTransition)
}

func TestUpdateOrderStatus_NotFound(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status")).
		WithArgs(models.StatusProcessing, orderID, models.StatusPending).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := repo.UpdateOrderStatus(context.Background(), orderID, models.StatusPending, models.StatusProcessing)
	assert.ErrorIs(t, err, ErrOrderNotFound)
}

func TestUpdateOrderTracking(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET tracking_number")).
		WithArgs("BR123456789", orderID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	assert.NoError(t, repo.UpdateOrderTracking(context.Background(), orderID, "BR123456789"))
}

func TestUpdateOrderTracking_NotFound(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET tracking_number")).
		WithArgs("BR123", orderID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	assert.ErrorIs(t, repo.UpdateOrderTracking(context.Background(), orderID, "BR123"), ErrOrderNotFound)
}

func TestExpireOrphanedOrders(t *testing.T) {
	db, repo := newMock(t)
	orderID, variantID := uuid.New(), uuid.New()

	// One tx: the CTE (FOR UPDATE SKIP LOCKED LIMIT) claims and cancels the batch,
	// RETURNING the cancelled ids; then each order's reservation is released in the same tx.
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).
		WithArgs(models.PaymentPending, models.StatusCancelled, models.PaymentFailed).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(orderID))
	db.ExpectQuery(regexp.QuoteMeta("SELECT variant_id, source_id, quantity FROM order_items")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"variant_id", "source_id", "quantity"}).AddRow(variantID, testDefaultSource, 2))
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(2, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	n, err := repo.ExpireOrphanedOrders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.NoError(t, db.ExpectationsWereMet())
}

// Given a product with default variant stock=5, When an order of qty=3 is created, Then
// the variant is reserved and the order_item records the variant_id.
func TestCreateOrderFromCart_ReservesStock(t *testing.T) {
	db, repo := newMock(t)
	userID, cartID, addrID := uuid.New(), uuid.New(), uuid.New()
	productID, variantID, orderID := uuid.New(), uuid.New(), uuid.New()
	deliveryID := uuid.New()
	items := []models.CartItem{{ProductID: productID, VariantID: variantID, Quantity: 3, PriceCents: 1000}}

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO orders")).
		WithArgs(userID, addrID, models.StatusPending, models.PaymentUnpaid, int64(3500), int64(500), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "status", "payment_status", "currency", "created_at", "updated_at"}).
			AddRow(orderID, models.StatusPending, models.PaymentUnpaid, "BRL", time.Now(), time.Now()))
	// The order's default delivery is created mirroring the cart's, carrying the freight.
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO deliveries")).
		WithArgs(orderID, "default", int64(500), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(deliveryID))
	// Reserve issues the atomic UPDATE ... WHERE available >= qty on the (variant, source) row.
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO order_items")).
		WithArgs(orderID, productID, variantID, 3, int64(1000), deliveryID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items")).
		WithArgs(cartID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	db.ExpectCommit()

	order, err := repo.CreateOrderFromCart(context.Background(), userID, cartID, addrID, items, 500, nil)
	require.NoError(t, err)
	assert.Equal(t, orderID, order.ID)
	assert.Equal(t, int64(3500), order.TotalCents) // 3*1000 items + 500 shipping
	assert.NoError(t, db.ExpectationsWereMet())

	// Exactly one "main" charge for the full order total is created post-commit.
	cr := repo.chargeRepo.(*fakeChargeRepo)
	require.Len(t, cr.created, 1)
	assert.Equal(t, models.ChargeMain, cr.created[0].Type)
	assert.Equal(t, int64(3500), cr.created[0].AmountCents)
	assert.Equal(t, "pending", cr.created[0].Status)
	assert.Equal(t, orderID, cr.created[0].OrderID)

	// order.placed is published post-commit.
	bus := repo.bus.(*fakeBus)
	require.Len(t, bus.published, 1)
	evt, ok := bus.published[0].(events.OrderPlacedEvent)
	require.True(t, ok)
	assert.Equal(t, orderID, evt.OrderID)
}

// Given a cart carrying a 10%-off coupon on a 10000-cent subtotal, When the order is
// created, Then the order total is frozen at 9000, the reduction is written to
// applied_discounts, and the coupon's redemption is counted.
func TestCreateOrderFromCart_FreezesDiscount(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)

	couponID := uuid.New()
	couponRepo := &fakeCouponRepo{byCode: map[string]*models.Coupon{
		"SAVE10": {ID: couponID, Code: "SAVE10", DiscountType: models.CouponPercent, Value: 10, Active: true},
	}}
	repo := &postgresOrderRepository{
		db:          db,
		variantRepo: variants.NewPostgresVariantRepository(nil),
		chargeRepo:  &fakeChargeRepo{},
		bus:         &fakeBus{},
		voucher: fakeVoucher{discounts: []models.AppliedDiscount{
			{Level: models.DiscountLevelCart, Type: models.CouponPercent, AppliedCents: -1000, CouponCode: "SAVE10"},
		}},
		couponRepo: couponRepo,
		allocator:  sourcing.NewSingleSourceAllocator(testDefaultSource),
	}

	userID, cartID, addrID := uuid.New(), uuid.New(), uuid.New()
	productID, variantID, orderID := uuid.New(), uuid.New(), uuid.New()
	deliveryID := uuid.New()
	items := []models.CartItem{{ProductID: productID, VariantID: variantID, Quantity: 1, PriceCents: 10000}}

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT applied_coupon_codes FROM carts")).
		WithArgs(cartID).
		WillReturnRows(pgxmock.NewRows([]string{"applied_coupon_codes"}).AddRow([]string{"SAVE10"}))
	// Total is frozen at 9000 = 10000 subtotal − 1000 discount + 0 shipping.
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO orders")).
		WithArgs(userID, addrID, models.StatusPending, models.PaymentUnpaid, int64(9000), int64(0), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "status", "payment_status", "currency", "created_at", "updated_at"}).
			AddRow(orderID, models.StatusPending, models.PaymentUnpaid, "BRL", time.Now(), time.Now()))
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO deliveries")).
		WithArgs(orderID, "default", int64(0), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(deliveryID))
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(1, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO order_items")).
		WithArgs(orderID, productID, variantID, 1, int64(10000), deliveryID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO applied_discounts")).
		WithArgs(orderID, models.DiscountLevelCart, models.CouponPercent, int64(-1000), "SAVE10").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items")).
		WithArgs(cartID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	db.ExpectCommit()

	order, err := repo.CreateOrderFromCart(context.Background(), userID, cartID, addrID, items, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), order.TotalCents)
	assert.NoError(t, db.ExpectationsWereMet())

	// The coupon was redeemed exactly once, and the main charge is the discounted total.
	require.Len(t, couponRepo.incremented, 1)
	assert.Equal(t, couponID, couponRepo.incremented[0])
	cr := repo.chargeRepo.(*fakeChargeRepo)
	require.Len(t, cr.created, 1)
	assert.Equal(t, int64(9000), cr.created[0].AmountCents)
}

// Given available=2, When an order of qty=3 is created, Then Reserve fails with
// ErrInsufficientStock and the whole order is rolled back (nothing persisted).
func TestCreateOrderFromCart_InsufficientStock(t *testing.T) {
	db, repo := newMock(t)
	userID, cartID, addrID := uuid.New(), uuid.New(), uuid.New()
	productID, variantID, orderID := uuid.New(), uuid.New(), uuid.New()
	items := []models.CartItem{{ProductID: productID, VariantID: variantID, Quantity: 3, PriceCents: 1000}}

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO orders")).
		WithArgs(userID, addrID, models.StatusPending, models.PaymentUnpaid, int64(3000), int64(0), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "status", "payment_status", "currency", "created_at", "updated_at"}).
			AddRow(orderID, models.StatusPending, models.PaymentUnpaid, "BRL", time.Now(), time.Now()))
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO deliveries")).
		WithArgs(orderID, "default", int64(0), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	// Zero rows affected => insufficient stock at the allocated source.
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()

	_, err := repo.CreateOrderFromCart(context.Background(), userID, cartID, addrID, items, 0, nil)
	assert.ErrorIs(t, err, ErrInsufficientStock)
	assert.NoError(t, db.ExpectationsWereMet())
}

// Given a not-yet-paid order with a reservation, When it is cancelled, Then the
// reservation is released (physical stock untouched).
func TestCancelOrder_ReleasesReservation(t *testing.T) {
	db, repo := newMock(t)
	orderID, variantID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT status, payment_status FROM orders")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"status", "payment_status"}).AddRow(models.StatusPending, models.PaymentUnpaid))
	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status")).
		WithArgs(models.StatusCancelled, orderID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(regexp.QuoteMeta("SELECT variant_id, source_id, quantity FROM order_items")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"variant_id", "source_id", "quantity"}).AddRow(variantID, testDefaultSource, 2))
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(2, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	require.NoError(t, repo.CancelOrder(context.Background(), orderID))
	assert.NoError(t, db.ExpectationsWereMet())
}

// A paid order that is cancelled must NOT release (stock was already claimed).
func TestCancelOrder_PaidDoesNotRelease(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("SELECT status, payment_status FROM orders")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"status", "payment_status"}).AddRow(models.StatusProcessing, models.PaymentPaid))
	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status")).
		WithArgs(models.StatusCancelled, orderID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	require.NoError(t, repo.CancelOrder(context.Background(), orderID))
	assert.NoError(t, db.ExpectationsWereMet())
}

// Given a pending_payment reserved order, When ConfirmOrderPayment runs, Then it becomes
// paid+processing and each variant is claimed (stock and reserved drop).
func TestConfirmOrderPayment_Claims(t *testing.T) {
	db, repo := newMock(t)
	orderID, variantID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET payment_status")).
		WithArgs(models.PaymentPaid, models.StatusProcessing, orderID, models.PaymentUnpaid, models.PaymentPending).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(regexp.QuoteMeta("SELECT variant_id, source_id, quantity FROM order_items")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"variant_id", "source_id", "quantity"}).AddRow(variantID, testDefaultSource, 3))
	// Claim decrements physical stock and the reservation together on the (variant, source) row.
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, variantID, testDefaultSource).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// The main charge is flipped to paid in the same tx, returning its reference.
	db.ExpectQuery(regexp.QuoteMeta("UPDATE payment_charges SET status = 'paid'")).
		WithArgs(orderID).
		WillReturnRows(pgxmock.NewRows([]string{"reference"}).AddRow("propay-tx-1"))
	db.ExpectCommit()

	require.NoError(t, repo.ConfirmOrderPayment(context.Background(), orderID))
	assert.NoError(t, db.ExpectationsWereMet())

	// payment.confirmed is published post-commit, carrying the charge reference.
	bus := repo.bus.(*fakeBus)
	require.Len(t, bus.published, 1)
	evt, ok := bus.published[0].(events.PaymentConfirmedEvent)
	require.True(t, ok)
	assert.Equal(t, orderID, evt.OrderID)
	assert.Equal(t, "propay-tx-1", evt.ChargeRef)
}

func TestConfirmOrderPayment_AlreadyPaid(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET payment_status")).
		WithArgs(models.PaymentPaid, models.StatusProcessing, orderID, models.PaymentUnpaid, models.PaymentPending).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()

	assert.ErrorIs(t, repo.ConfirmOrderPayment(context.Background(), orderID), ErrOrderNotFound)
}

func TestExpireUnpaidOrders(t *testing.T) {
	db, repo := newMock(t)

	// No eligible orders - the CTE batch returns empty, so no release runs and the tx
	// commits with a zero count.
	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).
		WithArgs(models.PaymentUnpaid, models.StatusCancelled, models.PaymentFailed).
		WillReturnRows(pgxmock.NewRows([]string{"id"}))
	db.ExpectCommit()

	n, err := repo.ExpireUnpaidOrders(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, db.ExpectationsWereMet())
}
