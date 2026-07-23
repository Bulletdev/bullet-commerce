package charges

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

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresChargeRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresChargeRepository{db: db}
}

func chargeCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "order_id", "type", "method", "amount_cents", "reference", "status", "created_at",
	})
}

func TestNewPostgresChargeRepository(t *testing.T) {
	assert.NotNil(t, NewPostgresChargeRepository(nil))
}

func TestCreate(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()
	newID := uuid.New()
	now := time.Now()

	charge := &models.Charge{
		OrderID:     orderID,
		Type:        models.ChargeMain,
		Method:      "credit_card",
		AmountCents: 9990,
		Reference:   "pi_123",
		Status:      "pending",
	}

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO payment_charges")).
		WithArgs(orderID, models.ChargeMain, "credit_card", int64(9990), "pi_123", "pending").
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(newID, now))

	err := repo.Create(context.Background(), charge)
	require.NoError(t, err)
	assert.Equal(t, newID, charge.ID)
	assert.Equal(t, now, charge.CreatedAt)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByOrderID(t *testing.T) {
	db, repo := newMock(t)
	orderID := uuid.New()

	rows := chargeCols().
		AddRow(uuid.New(), orderID, models.ChargeMain, "credit_card", int64(7000), "pi_1", "pending", time.Now()).
		AddRow(uuid.New(), orderID, models.ChargeGiftCard, "giftcard", int64(2990), "gc_1", "pending", time.Now())

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, order_id, type")).
		WithArgs(orderID).
		WillReturnRows(rows)

	charges, err := repo.FindByOrderID(context.Background(), orderID)
	require.NoError(t, err)
	require.Len(t, charges, 2)
	assert.Equal(t, orderID, charges[0].OrderID)
	assert.Equal(t, models.ChargeMain, charges[0].Type)
	assert.Equal(t, models.ChargeGiftCard, charges[1].Type)
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestPaymentSelectionTotalCents pins the accept criterion: main + giftcard sum exactly.
func TestPaymentSelectionTotalCents(t *testing.T) {
	sel := models.PaymentSelection{
		Charges: []models.Charge{
			{Type: models.ChargeMain, AmountCents: 7000},
			{Type: models.ChargeGiftCard, AmountCents: 2990},
		},
	}
	assert.Equal(t, int64(9990), sel.TotalCents())
}

func TestPaymentSelectionTotalCents_Empty(t *testing.T) {
	assert.Equal(t, int64(0), models.PaymentSelection{}.TotalCents())
}
