package orders

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFindUserOrders_DBError(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE user_id = $1")).
		WithArgs(userID, 20, 0).
		WillReturnError(errors.New("db error"))

	_, err := repo.FindUserOrders(context.Background(), userID, 20, 0)
	assert.Error(t, err)
}

func TestFindOrderByID_DBError(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, shipping_address_id")).
		WithArgs(id).
		WillReturnError(errors.New("db error"))

	_, _, err := repo.FindOrderByID(context.Background(), id)
	assert.Error(t, err)
}

func TestUpdateOrderStatus_DBError(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status")).
		WithArgs(models.StatusProcessing, id, models.StatusPending).
		WillReturnError(errors.New("db timeout"))

	err := repo.UpdateOrderStatus(context.Background(), id, models.StatusPending, models.StatusProcessing)
	assert.Error(t, err)
}

func TestUpdateOrderTracking_DBError(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE orders SET tracking_number")).
		WithArgs("BR123", id).
		WillReturnError(errors.New("db error"))

	assert.Error(t, repo.UpdateOrderTracking(context.Background(), id, "BR123"))
}

func TestExpireOrphanedOrders_DBError(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectBegin()
	db.ExpectQuery(regexp.QuoteMeta("FOR UPDATE SKIP LOCKED")).
		WithArgs(models.PaymentPending, models.StatusCancelled, models.PaymentFailed).
		WillReturnError(errors.New("db error"))
	db.ExpectRollback()

	_, err := repo.ExpireOrphanedOrders(context.Background())
	assert.Error(t, err)
}
