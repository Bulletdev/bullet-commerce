package coupons

import (
	"bullet-commerce/internal/models"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresCouponRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresCouponRepository{db: db}
}

func couponCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "code", "discount_type", "value", "min_cart_cents",
		"max_uses", "used_count", "expires_at", "active", "created_at",
	})
}

func TestNewPostgresCouponRepository(t *testing.T) {
	assert.NotNil(t, NewPostgresCouponRepository(nil))
}

func TestFindByCode_Found(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, code, discount_type")).
		WithArgs("SAVE10").
		WillReturnRows(couponCols().AddRow(
			id, "SAVE10", "percent", int64(10), int64(0),
			nil, 0, nil, true, time.Now(),
		))

	c, err := repo.FindByCode(context.Background(), "SAVE10")
	require.NoError(t, err)
	assert.Equal(t, id, c.ID)
	assert.Equal(t, "percent", c.DiscountType)
	assert.Equal(t, int64(10), c.Value)
	assert.True(t, c.Active)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByCode_NotFound(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, code, discount_type")).
		WithArgs("NOPE").
		WillReturnRows(couponCols())

	_, err := repo.FindByCode(context.Background(), "NOPE")
	assert.ErrorIs(t, err, ErrCouponNotFound)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestIncrementUse(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE coupons SET used_count = used_count + 1 WHERE id = $1")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.IncrementUse(context.Background(), id))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestMockCouponRepository(t *testing.T) {
	m := &MockCouponRepository{}
	id := uuid.New()
	coupon := &models.Coupon{ID: id, Code: "X", DiscountType: models.CouponFixed, Value: 500}

	m.On("FindByCode", context.Background(), "X").Return(coupon, nil)
	m.On("FindByCode", context.Background(), "MISSING").Return(nil, ErrCouponNotFound)
	m.On("IncrementUse", context.Background(), id).Return(nil)

	got, err := m.FindByCode(context.Background(), "X")
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)

	_, err = m.FindByCode(context.Background(), "MISSING")
	assert.ErrorIs(t, err, ErrCouponNotFound)

	require.NoError(t, m.IncrementUse(context.Background(), id))
	m.AssertExpectations(t)
}
