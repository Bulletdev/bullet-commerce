package variants

import (
	"bullet-commerce/internal/models"
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresVariantRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresVariantRepository{db: db}
}

// variantCols mirrors variantSelectColumns (the read projection), ending with the derived
// stock_available column FindByID/FindByProductID compute from variant_stock.
func variantCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "product_id", "sku", "attributes", "price_cents", "price_inherited", "currency",
		"weight_grams", "length_mm", "width_mm", "height_mm", "barcode", "active", "position",
		"compare_at_price_cents", "stock_policy", "stock", "stock_reserved", "deleted_at",
		"created_at", "updated_at", "stock_available",
	})
}

func addVariantRow(rows *pgxmock.Rows, id, productID uuid.UUID, sku string, stock, reserved int) *pgxmock.Rows {
	// stock_available mirrors the real per-source sum (stock - reserved) so tests exercise the
	// derived field the same way the DB would populate it.
	return rows.AddRow(id, productID, sku, json.RawMessage(`{}`), int64(1990), true, "BRL",
		nil, nil, nil, nil, nil, true, 0, nil, "deny",
		stock, reserved, nil, time.Now(), time.Now(), stock-reserved)
}

func TestCreate_ReturnsGenerated(t *testing.T) {
	db, repo := newMock(t)
	id, productID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO product_variants")).
		WithArgs(productID, "sku-1", json.RawMessage(`{}`), int64(1990), true, "BRL", 10, 0,
			(*int)(nil), (*int)(nil), (*int)(nil), (*int)(nil), (*string)(nil), (*int64)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "active", "position", "stock_policy", "deleted_at", "created_at", "updated_at",
		}).AddRow(id, true, 0, "deny", nil, time.Now(), time.Now()))

	v, err := repo.Create(context.Background(), &models.ProductVariant{
		ProductID:      productID,
		SKU:            "sku-1",
		Attributes:     json.RawMessage(`{}`),
		PriceCents:     1990,
		PriceInherited: true,
		Currency:       "BRL",
		Stock:          10,
	})
	require.NoError(t, err)
	assert.Equal(t, id, v.ID)
	assert.True(t, v.Active)
	assert.Equal(t, "deny", v.StockPolicy)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByID_Found(t *testing.T) {
	db, repo := newMock(t)
	id, productID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("id, product_id, sku")).
		WithArgs(id).
		WillReturnRows(addVariantRow(variantCols(), id, productID, "sku-1", 10, 0))

	v, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "sku-1", v.SKU)
	// Available is the derived stock_available column (Σ across sources), not a stored value.
	assert.Equal(t, 10, v.Available)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("id, product_id, sku")).
		WithArgs(id).
		WillReturnRows(variantCols())

	_, err := repo.FindByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrVariantNotFound)
}

// FindByProductID filters deleted_at IS NULL in SQL; the query text carries the
// guard, and only non-deleted rows are ever returned by the DB.
func TestFindByProductID_ExcludesDeleted(t *testing.T) {
	db, repo := newMock(t)
	productID := uuid.New()
	id1, id2 := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE product_id = $1 AND deleted_at IS NULL")).
		WithArgs(productID).
		WillReturnRows(addVariantRow(
			addVariantRow(variantCols(), id1, productID, "sku-a", 5, 0),
			id2, productID, "sku-b", 3, 1))

	list, err := repo.FindByProductID(context.Background(), productID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, v := range list {
		assert.Nil(t, v.DeletedAt)
	}
	assert.NoError(t, db.ExpectationsWereMet())
}

// AC: stock=10, reserved=0, Reserve(3, source) -> RowsAffected=1 (reserved becomes 3),
// scoped to the (variant, source) row of variant_stock.
func TestReserve_Succeeds(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := repo.Reserve(context.Background(), db, id, sourceID, 3)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

// AC: available=2, Reserve(3, source) -> ErrInsufficientStock, nothing changes (0 rows).
func TestReserve_InsufficientStock(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := repo.Reserve(context.Background(), db, id, sourceID, 3)
	assert.ErrorIs(t, err, ErrInsufficientStock)
	assert.NoError(t, db.ExpectationsWereMet())
}

// AC: reserved=3, Release(3, source) -> reserved=0 (the source's row is updated).
func TestRelease_Succeeds(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("SET stock_reserved = GREATEST(stock_reserved - $1, 0)")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := repo.Release(context.Background(), db, id, sourceID, 3)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestRelease_VariantNotFound(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := repo.Release(context.Background(), db, id, sourceID, 3)
	assert.ErrorIs(t, err, ErrVariantNotFound)
	assert.NoError(t, db.ExpectationsWereMet())
}

// AC: stock=10, reserved=3, Claim(3, source) -> stock=7, reserved=0 (the source's row).
func TestClaim_Succeeds(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("SET stock = stock - $1")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err := repo.Claim(context.Background(), db, id, sourceID, 3)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestClaim_Conflict(t *testing.T) {
	db, repo := newMock(t)
	id, sourceID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(3, id, sourceID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := repo.Claim(context.Background(), db, id, sourceID, 3)
	assert.ErrorIs(t, err, ErrStockClaimConflict)
	assert.NoError(t, db.ExpectationsWereMet())
}

// AC: available = sum across sources. A variant stocked at two sources (7 available + 3
// available) reports 10 available for display.
func TestAvailableForVariant_SumsAcrossSources(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(stock - stock_reserved), 0) FROM variant_stock")).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(10))

	available, err := repo.AvailableForVariant(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, 10, available)
	assert.NoError(t, db.ExpectationsWereMet())
}

// Reserve targets exactly ONE (variant, source) row: reserving from source B leaves source A
// untouched. The WHERE carries both keys, so the same variant can carry distinct stock per
// source and each reservation is scoped to its own source.
func TestReserve_IsScopedToSource(t *testing.T) {
	db, repo := newMock(t)
	id, sourceA, sourceB := uuid.New(), uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(2, id, sourceA).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectExec(regexp.QuoteMeta("UPDATE variant_stock")).
		WithArgs(4, id, sourceB).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.Reserve(context.Background(), db, id, sourceA, 2))
	require.NoError(t, repo.Reserve(context.Background(), db, id, sourceB, 4))
	assert.NoError(t, db.ExpectationsWereMet())
}
