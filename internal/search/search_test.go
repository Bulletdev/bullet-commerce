package search

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresService) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresService{db: db}
}

func countRows(n int) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"count"}).AddRow(n)
}

// TestSearch_VariadicComposition proves KeyValue + Sort + Pagination compose into one
// query: the category condition binds its arg, the sort drives ORDER BY, and pagination
// supplies LIMIT/OFFSET.
func TestSearch_VariadicComposition(t *testing.T) {
	db, svc := newMock(t)
	catID := uuid.New()
	prodID := uuid.New()

	// count over the filtered set
	db.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM products WHERE deleted_at IS NULL AND status = 'active' AND category_id = $1")).
		WithArgs(catID).
		WillReturnRows(countRows(2))

	// id page: sort whitelisted to price_cents DESC, paged with LIMIT/OFFSET args
	db.ExpectQuery(regexp.QuoteMeta("SELECT id FROM products WHERE deleted_at IS NULL AND status = 'active' AND category_id = $1 ORDER BY price_cents DESC LIMIT $2 OFFSET $3")).
		WithArgs(catID, 10, 0).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(prodID))

	// facets over the same filtered set
	db.ExpectQuery(regexp.QuoteMeta("SELECT category_id::text, COUNT(*) FROM products")).
		WithArgs(catID).
		WillReturnRows(pgxmock.NewRows([]string{"category_id", "count"}).AddRow(catID.String(), 2))
	db.ExpectQuery(regexp.QuoteMeta("AS bucket, COUNT(*) FROM products")).
		WithArgs(catID).
		WillReturnRows(pgxmock.NewRows([]string{"bucket", "count"}).AddRow("0-5000", 2))

	res, err := svc.Search(context.Background(),
		KeyValueFilter{Field: "category_id", Value: catID.String()},
		SortFilter{Field: "price", Desc: true},
		PaginationFilter{Limit: 10, Offset: 0},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, res.NumResults)
	assert.Equal(t, 1, res.NumPages)
	assert.Equal(t, []uuid.UUID{prodID}, res.ProductIDs)
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestSearch_FacetsHaveCounts verifies facet items carry counts and that the actively
// filtered category is flagged Selected.
func TestSearch_FacetsHaveCounts(t *testing.T) {
	db, svc := newMock(t)
	catID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM products")).
		WithArgs(catID).
		WillReturnRows(countRows(5))
	db.ExpectQuery(regexp.QuoteMeta("SELECT id FROM products")).
		WithArgs(catID, 20, 0).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	db.ExpectQuery(regexp.QuoteMeta("SELECT category_id::text, COUNT(*) FROM products")).
		WithArgs(catID).
		WillReturnRows(pgxmock.NewRows([]string{"category_id", "count"}).AddRow(catID.String(), 5))
	db.ExpectQuery(regexp.QuoteMeta("AS bucket, COUNT(*) FROM products")).
		WithArgs(catID).
		WillReturnRows(pgxmock.NewRows([]string{"bucket", "count"}).
			AddRow("0-5000", 3).AddRow("5000-10000", 2))

	res, err := svc.Search(context.Background(),
		KeyValueFilter{Field: "category_id", Value: catID.String()},
	)
	require.NoError(t, err)
	require.Len(t, res.Facets, 2)

	category := res.Facets[0]
	assert.Equal(t, "category_id", category.Field)
	assert.Equal(t, "list", category.Kind)
	require.Len(t, category.Items, 1)
	assert.Equal(t, 5, category.Items[0].Count)
	assert.True(t, category.Items[0].Selected)

	price := res.Facets[1]
	assert.Equal(t, "range", price.Kind)
	require.Len(t, price.Items, 2)
	assert.Equal(t, 3, price.Items[0].Count)
	assert.False(t, price.Items[0].Selected)
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestSearch_TextQueryUsesRank confirms a QueryFilter drives a tsvector match and orders
// by ts_rank when no explicit sort is given.
func TestSearch_TextQueryUsesRank(t *testing.T) {
	db, svc := newMock(t)
	prodID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("search_tsv @@ to_tsquery('portuguese', $1)")).
		WithArgs("caneca").
		WillReturnRows(countRows(1))
	db.ExpectQuery(regexp.QuoteMeta("ORDER BY ts_rank(search_tsv, to_tsquery('portuguese', $1)) DESC")).
		WithArgs("caneca", 20, 0).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(prodID))
	db.ExpectQuery(regexp.QuoteMeta("category_id::text")).
		WithArgs("caneca").
		WillReturnRows(pgxmock.NewRows([]string{"category_id", "count"}))
	db.ExpectQuery(regexp.QuoteMeta("AS bucket")).
		WithArgs("caneca").
		WillReturnRows(pgxmock.NewRows([]string{"bucket", "count"}).AddRow("0-5000", 1))

	res, err := svc.Search(context.Background(), QueryFilter{Text: "caneca"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.NumResults)
	assert.Equal(t, []uuid.UUID{prodID}, res.ProductIDs)
	assert.NoError(t, db.ExpectationsWereMet())
}

// TestSearch_NoResults confirms a zero count short-circuits to an empty Result without
// running the id-page or facet queries.
func TestSearch_NoResults(t *testing.T) {
	db, svc := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM products")).
		WithArgs("inexistente").
		WillReturnRows(countRows(0))

	res, err := svc.Search(context.Background(), QueryFilter{Text: "inexistente"})
	require.NoError(t, err)
	assert.Equal(t, 0, res.NumResults)
	assert.Equal(t, 0, res.NumPages)
	assert.Empty(t, res.ProductIDs)
	assert.Empty(t, res.Facets)
	assert.NoError(t, db.ExpectationsWereMet())
}
