package addresses

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresAddressRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresAddressRepository{db: db}
}

func addrCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "street", "city", "state", "postal_code", "country", "is_default", "is_default_billing", "is_default_shipping", "created_at", "updated_at"})
}

func addAddrRow(rows *pgxmock.Rows, id, userID uuid.UUID) *pgxmock.Rows {
	return rows.AddRow(id, userID, "Rua A", "SP", "SP", "01310100", "BR", false, false, false, time.Now(), time.Now())
}

func TestCreate_NotDefault(t *testing.T) {
	db, repo := newMock(t)
	addr := &models.Address{UserID: uuid.New(), Street: "Rua A", City: "SP", State: "SP", PostalCode: "01310100", Country: "BR", IsDefault: false}
	id := uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO addresses")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(id, now, now))

	result, err := repo.Create(context.Background(), addr)
	require.NoError(t, err)
	assert.Equal(t, id, result.ID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreate_IsDefault_UnsetsThenInserts(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()
	addr := &models.Address{UserID: userID, Street: "Rua B", City: "RJ", State: "RJ", PostalCode: "20040020", Country: "BR", IsDefault: true}
	id := uuid.New()
	now := time.Now()

	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO addresses")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(id, now, now))

	result, err := repo.Create(context.Background(), addr)
	require.NoError(t, err)
	assert.Equal(t, id, result.ID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestCreate_UnsetError(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()
	addr := &models.Address{UserID: userID, IsDefault: true}

	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = false WHERE user_id")).
		WithArgs(userID).
		WillReturnError(errors.New("db error"))

	_, err := repo.Create(context.Background(), addr)
	assert.Error(t, err)
}

func TestFindByUserID_Found(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, street")).
		WithArgs(userID).
		WillReturnRows(addAddrRow(addrCols(), id, userID))

	list, err := repo.FindByUserID(context.Background(), userID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestFindByUserID_Empty(t *testing.T) {
	db, repo := newMock(t)
	userID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, street")).
		WithArgs(userID).
		WillReturnRows(addrCols())

	list, err := repo.FindByUserID(context.Background(), userID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestFindByUserAndID_Found(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE id = $1 AND user_id = $2")).
		WithArgs(addrID, userID).
		WillReturnRows(addAddrRow(addrCols(), addrID, userID))

	addr, err := repo.FindByUserAndID(context.Background(), userID, addrID)
	require.NoError(t, err)
	assert.Equal(t, addrID, addr.ID)
}

func TestFindByUserAndID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE id = $1 AND user_id = $2")).
		WithArgs(addrID, userID).
		WillReturnRows(addrCols())

	_, err := repo.FindByUserAndID(context.Background(), userID, addrID)
	assert.ErrorIs(t, err, ErrAddressNotFound)
}

func TestUpdate_Success(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()
	addr := &models.Address{Street: "New St", City: "MG", State: "MG", PostalCode: "30112000", Country: "BR", IsDefault: false}
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE addresses")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"updated_at"}).AddRow(now))

	result, err := repo.Update(context.Background(), userID, addrID, addr)
	require.NoError(t, err)
	assert.Equal(t, addrID, result.ID)
}

func TestUpdate_IsDefault(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()
	addr := &models.Address{IsDefault: true}
	now := time.Now()

	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectQuery(regexp.QuoteMeta("UPDATE addresses")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"updated_at"}).AddRow(now))

	_, err := repo.Update(context.Background(), userID, addrID, addr)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestUpdate_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()
	addr := &models.Address{}

	db.ExpectQuery(regexp.QuoteMeta("UPDATE addresses")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"updated_at"}))

	_, err := repo.Update(context.Background(), userID, addrID, addr)
	assert.ErrorIs(t, err, ErrAddressNotFound)
}

func TestDelete_Success(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM addresses WHERE id")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	assert.NoError(t, repo.Delete(context.Background(), userID, addrID))
}

func TestDelete_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM addresses WHERE id")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	assert.ErrorIs(t, repo.Delete(context.Background(), userID, addrID), ErrAddressNotFound)
}

func TestSetDefault_Success(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	assert.NoError(t, repo.SetDefault(context.Background(), userID, addrID))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestSetDefault_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()

	assert.ErrorIs(t, repo.SetDefault(context.Background(), userID, addrID), ErrAddressNotFound)
}

// TestSetDefaultBilling_OnlyTouchesBilling asserts that promoting a billing default
// unsets other billing defaults and sets the target, without touching shipping columns.
func TestSetDefaultBilling_OnlyTouchesBilling(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_billing = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_billing = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	assert.NoError(t, repo.SetDefaultBilling(context.Background(), userID, addrID))
	// ExpectationsWereMet proves no shipping-column UPDATE was issued: billing is independent.
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestSetDefaultBilling_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_billing = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_billing = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()

	assert.ErrorIs(t, repo.SetDefaultBilling(context.Background(), userID, addrID), ErrAddressNotFound)
}

// TestSetDefaultShipping_OnlyTouchesShipping is the shipping mirror of the billing test,
// asserting the shipping default is independent of billing.
func TestSetDefaultShipping_OnlyTouchesShipping(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_shipping = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_shipping = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()

	assert.NoError(t, repo.SetDefaultShipping(context.Background(), userID, addrID))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestSetDefaultShipping_NotFound(t *testing.T) {
	db, repo := newMock(t)
	userID, addrID := uuid.New(), uuid.New()

	db.ExpectBegin()
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_shipping = false WHERE user_id")).
		WithArgs(userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectExec(regexp.QuoteMeta("UPDATE addresses SET is_default_shipping = true")).
		WithArgs(addrID, userID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	db.ExpectRollback()

	assert.ErrorIs(t, repo.SetDefaultShipping(context.Background(), userID, addrID), ErrAddressNotFound)
}
