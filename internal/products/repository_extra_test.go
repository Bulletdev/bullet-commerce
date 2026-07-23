package products

import (
	"bullet-commerce/internal/models"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_Success(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO products")).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "attributes", "variant_variation_attributes", "stock", "featured", "version", "deleted_at", "created_at", "updated_at"}).
			AddRow(id, "simple", json.RawMessage(`{}`), []string{}, 0, false, 1, nil, time.Now(), time.Now()))

	p, err := repo.Create(context.Background(), &models.Product{Name: "Widget", PriceCents: 990})
	require.NoError(t, err)
	assert.Equal(t, id, p.ID)
	// Catalog defaults are backfilled by the repository even when the caller omits them.
	assert.Equal(t, models.ProductTypeSimple, p.Type)
}

// TestCreate_CatalogFields verifies the new catalog fields (type, attributes,
// variant_variation_attributes) round-trip through Create.
func TestCreate_CatalogFields(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO products")).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			"configurable", json.RawMessage(`{"brand":"acme"}`), []string{"tamanho", "cor"},
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "type", "attributes", "variant_variation_attributes", "stock", "featured", "version", "deleted_at", "created_at", "updated_at"}).
			AddRow(id, "configurable", json.RawMessage(`{"brand":"acme"}`), []string{"tamanho", "cor"}, 0, false, 1, nil, time.Now(), time.Now()))

	p, err := repo.Create(context.Background(), &models.Product{
		Name:                       "Shirt",
		PriceCents:                 4900,
		Type:                       models.ProductTypeConfigurable,
		Attributes:                 json.RawMessage(`{"brand":"acme"}`),
		VariantVariationAttributes: []string{"tamanho", "cor"},
	})
	require.NoError(t, err)
	assert.Equal(t, models.ProductTypeConfigurable, p.Type)
	assert.Equal(t, []string{"tamanho", "cor"}, p.VariantVariationAttributes)
	assert.JSONEq(t, `{"brand":"acme"}`, string(p.Attributes))
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestFindByID_CatalogFields verifies the catalog fields are read back on a lookup.
func TestFindByID_CatalogFields(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	rows := productCols().AddRow(id, "Shirt", "desc", int64(4900), "BRL", nil, 10, false,
		"configurable", json.RawMessage(`{"brand":"acme"}`), []string{"tamanho", "cor"},
		nil, nil, nil, nil,
		nil, nil, 0, "UN",
		"active", nil, nil, nil, nil, nil,
		1, nil, 0,
		nil, time.Now(), time.Now())
	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(id).
		WillReturnRows(rows)

	p, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "configurable", p.Type)
	assert.Equal(t, []string{"tamanho", "cor"}, p.VariantVariationAttributes)
	assert.JSONEq(t, `{"brand":"acme"}`, string(p.Attributes))
}

func TestCreate_DBError(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO products")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	_, err := repo.Create(context.Background(), &models.Product{Name: "X", PriceCents: 100})
	assert.Error(t, err)
}

func TestFindByCategoryID(t *testing.T) {
	db, repo := newMock(t)
	catID, prodID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE category_id = $1")).
		WithArgs(catID, 20, 0).
		WillReturnRows(addProductRow(productCols(), prodID, "Cat Product", 4900))

	list, err := repo.FindByCategoryID(context.Background(), catID, 20, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestUpdate_Success(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE products")).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), id, pgxmock.AnyArg()).
		WillReturnRows(addProductRow(productCols(), id, "Updated", 9900))

	updated, err := repo.Update(context.Background(), id, &models.Product{Name: "Updated", PriceCents: 9900, Version: 1})
	require.NoError(t, err)
	assert.Equal(t, "Updated", updated.Name)
}

func TestUpdate_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE products")).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), id, pgxmock.AnyArg()).
		WillReturnRows(productCols())
	// The optimistic UPDATE missed; existsLive probes whether the row is truly gone (here it
	// is) so Update surfaces ErrProductNotFound rather than a version conflict.
	db.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	_, err := repo.Update(context.Background(), id, &models.Product{Name: "X", PriceCents: 100, Version: 1})
	assert.ErrorIs(t, err, ErrProductNotFound)
}

func TestUpdate_VersionConflict(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE products")).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), id, pgxmock.AnyArg()).
		WillReturnRows(productCols())
	// The UPDATE missed but a live row still exists: a concurrent edit bumped the version, so
	// Update reports ErrProductVersionConflict (mapped to HTTP 409 by the handler).
	db.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	_, err := repo.Update(context.Background(), id, &models.Product{Name: "X", PriceCents: 100, Version: 1})
	assert.ErrorIs(t, err, ErrProductVersionConflict)
}

func TestFindAll_DBError(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(20, 0).
		WillReturnError(errors.New("db error"))

	_, err := repo.FindAll(context.Background(), 20, 0)
	assert.Error(t, err)
}

func TestFindByID_DBError(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description")).
		WithArgs(id).
		WillReturnError(errors.New("connection reset"))

	_, err := repo.FindByID(context.Background(), id)
	assert.Error(t, err)
}

// TestSetCategories replaces the N:N membership set: one DELETE followed by an INSERT per id.
func TestSetCategories(t *testing.T) {
	db, repo := newMock(t)
	prodID, cat1, cat2 := uuid.New(), uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM product_categories WHERE product_id = $1")).
		WithArgs(prodID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO product_categories")).
		WithArgs(prodID, cat1).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	db.ExpectExec(regexp.QuoteMeta("INSERT INTO product_categories")).
		WithArgs(prodID, cat2).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, repo.SetCategories(context.Background(), prodID, []uuid.UUID{cat1, cat2}))
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestSetCategories_ClearOnly verifies an empty slice just clears the memberships.
func TestSetCategories_ClearOnly(t *testing.T) {
	db, repo := newMock(t)
	prodID := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM product_categories WHERE product_id = $1")).
		WithArgs(prodID).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))

	require.NoError(t, repo.SetCategories(context.Background(), prodID, nil))
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindCategoryIDs(t *testing.T) {
	db, repo := newMock(t)
	prodID, cat1, cat2 := uuid.New(), uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT category_id FROM product_categories WHERE product_id = $1")).
		WithArgs(prodID).
		WillReturnRows(pgxmock.NewRows([]string{"category_id"}).AddRow(cat1).AddRow(cat2))

	ids, err := repo.FindCategoryIDs(context.Background(), prodID)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{cat1, cat2}, ids)
}
