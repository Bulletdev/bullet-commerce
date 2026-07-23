package products

import (
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

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresProductRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresProductRepository{db: db}
}

func productCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "name", "description", "price_cents", "currency", "category_id",
		"stock", "featured", "type", "attributes", "variant_variation_attributes",
		"weight_grams", "length_mm", "width_mm", "height_mm",
		"ncm", "cest", "origem", "unit",
		"status", "slug", "meta_title", "meta_description", "brand_id", "compare_at_price_cents",
		"version", "rating_avg", "rating_count",
		"deleted_at", "created_at", "updated_at",
	})
}

func addProductRow(rows *pgxmock.Rows, id uuid.UUID, name string, priceCents int64) *pgxmock.Rows {
	return rows.AddRow(id, name, "desc", priceCents, "BRL", nil, 10, false,
		"simple", json.RawMessage(`{}`), []string{},
		nil, nil, nil, nil,
		nil, nil, 0, "UN",
		"active", nil, nil, nil, nil, nil,
		1, nil, 0,
		nil, time.Now(), time.Now())
}

func TestFindByID_Found(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(id).
		WillReturnRows(addProductRow(productCols(), id, "Widget", 2990))

	p, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Widget", p.Name)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(id).
		WillReturnRows(productCols())

	_, err := repo.FindByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrProductNotFound)
}

func TestFindAll_ReturnsList(t *testing.T) {
	db, repo := newMock(t)
	id1, id2 := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(20, 0).
		WillReturnRows(
			addProductRow(addProductRow(productCols(), id1, "A", 1000), id2, "B", 2000))

	list, err := repo.FindAll(context.Background(), 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindAll_Empty(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(20, 0).
		WillReturnRows(productCols())

	list, err := repo.FindAll(context.Background(), 20, 0)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestFindFeatured(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE featured = TRUE")).
		WillReturnRows(addProductRow(productCols(), id, "Star", 9900))

	list, err := repo.FindFeatured(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestDelete_Success(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE products SET deleted_at")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	assert.NoError(t, repo.Delete(context.Background(), id))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestDelete_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE products SET deleted_at")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	assert.ErrorIs(t, repo.Delete(context.Background(), id), ErrProductNotFound)
}

func TestUpdateStock_Success(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE products SET stock")).
		WithArgs(50, id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	assert.NoError(t, repo.UpdateStock(context.Background(), id, 50))
}

func TestUpdateStock_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("UPDATE products SET stock")).
		WithArgs(50, id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	assert.ErrorIs(t, repo.UpdateStock(context.Background(), id, 50), ErrProductNotFound)
}

func TestSearch(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE (name ILIKE")).
		WithArgs("%go%", 20, 0).
		WillReturnRows(addProductRow(productCols(), id, "Go Shirt", 4900))

	list, err := repo.Search(context.Background(), "go", 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
