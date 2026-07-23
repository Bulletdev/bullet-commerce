package cart

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresCartRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresCartRepository{db: db}
}

func cartCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "applied_coupon_codes", "created_at", "updated_at"})
}

func itemCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "cart_id", "product_id", "variant_id", "delivery_id", "quantity", "price_cents", "created_at", "updated_at"})
}

func TestGetOrCreateCartByUserID_Upsert(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()
	cartID := uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO carts")).
		WithArgs(userID).
		WillReturnRows(cartCols().AddRow(cartID, userID, []string{}, now, now))
	// The default delivery is ensured transparently right after the cart upsert.
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO deliveries")).
		WithArgs(cartID, "default").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	cart, err := repo.GetOrCreateCartByUserID(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, cartID, cart.ID)
	assert.Empty(t, cart.AppliedCouponCodes)
	assert.NoError(t, db.ExpectationsWereMet())
}

// A new cart automatically gains its transparent default delivery: ensureDefaultDelivery
// is idempotent (ON CONFLICT DO NOTHING), so calling it again is a no-op.
func TestEnsureDefaultDelivery_Idempotent(t *testing.T) {
	db, repo := newMock(t)
	cartID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("INSERT INTO deliveries")).
		WithArgs(cartID, "default").
		WillReturnResult(pgxmock.NewResult("INSERT", 0)) // conflict -> no-op

	assert.NoError(t, repo.ensureDefaultDelivery(context.Background(), cartID))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestAddCouponCode(t *testing.T) {
	db, repo := newMock(t)
	cartID, userID := uuid.New(), uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("SET applied_coupon_codes = CASE")).
		WithArgs(cartID, "SAVE10").
		WillReturnRows(cartCols().AddRow(cartID, userID, []string{"SAVE10"}, now, now))

	cart, err := repo.AddCouponCode(context.Background(), cartID, "SAVE10")
	require.NoError(t, err)
	assert.Equal(t, []string{"SAVE10"}, cart.AppliedCouponCodes)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestRemoveCouponCode(t *testing.T) {
	db, repo := newMock(t)
	cartID, userID := uuid.New(), uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("array_remove(applied_coupon_codes, $2)")).
		WithArgs(cartID, "SAVE10").
		WillReturnRows(cartCols().AddRow(cartID, userID, []string{}, now, now))

	cart, err := repo.RemoveCouponCode(context.Background(), cartID, "SAVE10")
	require.NoError(t, err)
	assert.Empty(t, cart.AppliedCouponCodes)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestGetCartItems_Empty(t *testing.T) {
	db, repo := newMock(t)
	cartID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, cart_id")).
		WithArgs(cartID).
		WillReturnRows(itemCols())

	items, err := repo.GetCartItems(context.Background(), cartID)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestGetCartItems_WithItems(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID, variantID := uuid.New(), uuid.New(), uuid.New()
	itemID, deliveryID := uuid.New(), uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, cart_id")).
		WithArgs(cartID).
		WillReturnRows(itemCols().AddRow(itemID, cartID, prodID, variantID, deliveryID, 2, int64(2990), now, now))

	items, err := repo.GetCartItems(context.Background(), cartID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].Quantity)
	assert.Equal(t, deliveryID, items[0].DeliveryID)
}

func TestAddItem_Upsert(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID, variantID := uuid.New(), uuid.New(), uuid.New()
	itemID, deliveryID := uuid.New(), uuid.New()
	now := time.Now()

	// AddItem resolves the cart's default delivery via subquery, so the returned line carries
	// delivery_id transparently while the call args stay (cart, product, variant, qty, price).
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO cart_items")).
		WithArgs(cartID, prodID, variantID, 3, int64(1500)).
		WillReturnRows(itemCols().AddRow(itemID, cartID, prodID, variantID, deliveryID, 3, int64(1500), now, now))

	item, err := repo.AddItem(context.Background(), cartID, prodID, variantID, 3, int64(1500))
	require.NoError(t, err)
	assert.Equal(t, 3, item.Quantity)
	assert.Equal(t, deliveryID, item.DeliveryID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestRemoveItem_Success(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items")).
		WithArgs(cartID, prodID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	assert.NoError(t, repo.RemoveItem(context.Background(), cartID, prodID))
}

func TestRemoveItem_NotInCart(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items")).
		WithArgs(cartID, prodID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	assert.ErrorIs(t, repo.RemoveItem(context.Background(), cartID, prodID), ErrProductNotInCart)
}

func TestClearCart(t *testing.T) {
	db, repo := newMock(t)
	cartID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items WHERE cart_id")).
		WithArgs(cartID).
		WillReturnResult(pgxmock.NewResult("DELETE", 3))

	assert.NoError(t, repo.ClearCart(context.Background(), cartID))
}

func TestUpdateItemQuantity_Success(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID, variantID := uuid.New(), uuid.New(), uuid.New()
	itemID, deliveryID := uuid.New(), uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE cart_items")).
		WithArgs(5, cartID, variantID).
		WillReturnRows(itemCols().AddRow(itemID, cartID, prodID, variantID, deliveryID, 5, int64(1000), now, now))

	item, err := repo.UpdateItemQuantity(context.Background(), cartID, variantID, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, item.Quantity)
}

func TestUpdateItemQuantity_NotFound(t *testing.T) {
	db, repo := newMock(t)
	cartID, variantID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE cart_items")).
		WithArgs(5, cartID, variantID).
		WillReturnRows(itemCols())

	_, err := repo.UpdateItemQuantity(context.Background(), cartID, variantID, 5)
	assert.ErrorIs(t, err, ErrProductNotInCart)
}
