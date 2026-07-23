package cart

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindCartItem_Found(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID := uuid.New(), uuid.New()
	itemID := uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, cart_id, product_id, variant_id, delivery_id, quantity, price")).
		WithArgs(cartID, prodID).
		WillReturnRows(itemCols().AddRow(itemID, cartID, prodID, uuid.New(), uuid.New(), 1, int64(1000), now, now))

	item, err := repo.FindCartItem(context.Background(), cartID, prodID)
	require.NoError(t, err)
	assert.Equal(t, itemID, item.ID)
}

func TestFindCartItem_NotFound(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, cart_id, product_id, variant_id, delivery_id, quantity, price")).
		WithArgs(cartID, prodID).
		WillReturnRows(itemCols())

	_, err := repo.FindCartItem(context.Background(), cartID, prodID)
	assert.ErrorIs(t, err, ErrProductNotInCart)
}

func TestGetOrCreateCartByUserID_DBError(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO carts")).
		WithArgs(userID).
		WillReturnError(errors.New("db error"))

	_, err := repo.GetOrCreateCartByUserID(context.Background(), userID)
	assert.Error(t, err)
}

func TestGetCartItems_DBError(t *testing.T) {
	db, repo := newMock(t)
	cartID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, cart_id")).
		WithArgs(cartID).
		WillReturnError(errors.New("db error"))

	_, err := repo.GetCartItems(context.Background(), cartID)
	assert.Error(t, err)
}

func TestRemoveItem_DBError(t *testing.T) {
	db, repo := newMock(t)
	cartID, prodID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items")).
		WithArgs(cartID, prodID).
		WillReturnError(errors.New("db error"))

	assert.Error(t, repo.RemoveItem(context.Background(), cartID, prodID))
}

func TestClearCart_DBError(t *testing.T) {
	db, repo := newMock(t)
	cartID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM cart_items WHERE cart_id")).
		WithArgs(cartID).
		WillReturnError(errors.New("db error"))

	assert.Error(t, repo.ClearCart(context.Background(), cartID))
}
