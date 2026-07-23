package sourcing

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default allocator routes every item's full quantity to the default source, preserving
// order (one Allocation per AllocItem) — the transparent V1 behavior.
func TestSingleSourceAllocator_AllocatesFromDefault(t *testing.T) {
	defaultSource := uuid.New()
	alloc := NewSingleSourceAllocator(defaultSource)
	vA, vB := uuid.New(), uuid.New()

	got, err := alloc.Allocate(context.Background(), []AllocItem{
		{VariantID: vA, Qty: 3},
		{VariantID: vB, Qty: 1},
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, Allocation{VariantID: vA, SourceID: defaultSource, Qty: 3}, got[0])
	assert.Equal(t, Allocation{VariantID: vB, SourceID: defaultSource, Qty: 1}, got[1])
}

func TestSingleSourceAllocator_Empty(t *testing.T) {
	alloc := NewSingleSourceAllocator(uuid.New())
	got, err := alloc.Allocate(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStockProvider_GetStock_ReturnsAvailable(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	provider := &postgresStockProvider{db: db}
	variantID, sourceID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(stock - stock_reserved, 0) FROM variant_stock")).
		WithArgs(variantID, sourceID).
		WillReturnRows(pgxmock.NewRows([]string{"available"}).AddRow(7))

	available, err := provider.GetStock(context.Background(), variantID, sourceID)
	require.NoError(t, err)
	assert.Equal(t, 7, available)
	assert.NoError(t, db.ExpectationsWereMet())
}

// A pair with no variant_stock row is not an error: it simply has zero available.
func TestStockProvider_GetStock_NoRowIsZero(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	provider := &postgresStockProvider{db: db}
	variantID, sourceID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(stock - stock_reserved, 0) FROM variant_stock")).
		WithArgs(variantID, sourceID).
		WillReturnError(pgx.ErrNoRows)

	available, err := provider.GetStock(context.Background(), variantID, sourceID)
	require.NoError(t, err)
	assert.Equal(t, 0, available)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestSourceRepository_GetDefault(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	repo := &postgresSourceRepository{db: db}
	sourceID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE is_default = TRUE")).
		WillReturnRows(pgxmock.NewRows([]string{"id", "code", "name", "is_default", "created_at", "updated_at"}).
			AddRow(sourceID, "default", "Default", true, time.Now(), time.Now()))

	s, err := repo.GetDefault(context.Background())
	require.NoError(t, err)
	assert.Equal(t, sourceID, s.ID)
	assert.True(t, s.IsDefault)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestSourceRepository_GetDefault_Missing(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	repo := &postgresSourceRepository{db: db}

	db.ExpectQuery(regexp.QuoteMeta("WHERE is_default = TRUE")).
		WillReturnError(pgx.ErrNoRows)

	_, err = repo.GetDefault(context.Background())
	assert.ErrorIs(t, err, ErrNoDefaultSource)
}
