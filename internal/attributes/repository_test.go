package attributes

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresAttributeRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresAttributeRepository{db: db}
}

func TestFindByCode_Found(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("FROM attribute")).
		WithArgs("cor").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "code", "label", "kind", "created_at", "updated_at",
		}).AddRow(id, "cor", "Cor", "color", time.Now(), time.Now()))

	a, err := repo.FindByCode(context.Background(), "cor")
	require.NoError(t, err)
	assert.Equal(t, "cor", a.Code)
	assert.Equal(t, "color", a.Kind)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByCode_NotFound(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("FROM attribute")).
		WithArgs("missing").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "code", "label", "kind", "created_at", "updated_at",
		}))

	_, err := repo.FindByCode(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrAttributeNotFound)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ListValues returns values ordered by position: the SQL carries ORDER BY position, so 'M'
// (position 0) precedes 'G' (position 1) regardless of alphabetical order.
func TestListValues_OrderedByPosition(t *testing.T) {
	db, repo := newMock(t)
	attrID := uuid.New()
	m, g := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("ORDER BY position ASC, value ASC")).
		WithArgs(attrID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "attribute_id", "value", "label", "hex", "position",
		}).AddRow(m, attrID, "M", "M", nil, 0).
			AddRow(g, attrID, "G", "G", nil, 1))

	values, err := repo.ListValues(context.Background(), attrID)
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "M", values[0].Value)
	assert.Equal(t, "G", values[1].Value)
	assert.Nil(t, values[0].Hex)
	assert.NoError(t, db.ExpectationsWereMet())
}

// A color value carries a hex swatch; the pointer is scanned through so the UI can render it.
func TestListValues_CarriesHex(t *testing.T) {
	db, repo := newMock(t)
	attrID := uuid.New()
	black := uuid.New()
	hex := "#000000"

	db.ExpectQuery(regexp.QuoteMeta("FROM attribute_value")).
		WithArgs(attrID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "attribute_id", "value", "label", "hex", "position",
		}).AddRow(black, attrID, "preto", "Preto", &hex, 0))

	values, err := repo.ListValues(context.Background(), attrID)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.NotNil(t, values[0].Hex)
	assert.Equal(t, "#000000", *values[0].Hex)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestLinkVariant_Inserts(t *testing.T) {
	db, repo := newMock(t)
	variantID, valueID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("INSERT INTO variant_attribute_value")).
		WithArgs(variantID, valueID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := repo.LinkVariant(context.Background(), variantID, valueID)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

// Re-linking the same pair is a no-op (ON CONFLICT DO NOTHING -> 0 rows) and must not error,
// so backfill/admin re-runs are safe.
func TestLinkVariant_Idempotent(t *testing.T) {
	db, repo := newMock(t)
	variantID, valueID := uuid.New(), uuid.New()

	db.ExpectExec(regexp.QuoteMeta("ON CONFLICT DO NOTHING")).
		WithArgs(variantID, valueID).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))

	err := repo.LinkVariant(context.Background(), variantID, valueID)
	require.NoError(t, err)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestValuesForVariant_ReturnsLinkedValues(t *testing.T) {
	db, repo := newMock(t)
	variantID := uuid.New()
	attrID, valueID := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("JOIN variant_attribute_value vav")).
		WithArgs(variantID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "attribute_id", "value", "label", "hex", "position",
		}).AddRow(valueID, attrID, "M", "M", nil, 0))

	values, err := repo.ValuesForVariant(context.Background(), variantID)
	require.NoError(t, err)
	require.Len(t, values, 1)
	assert.Equal(t, "M", values[0].Value)
	assert.Equal(t, attrID, values[0].AttributeID)
	assert.NoError(t, db.ExpectationsWereMet())
}

// FacetCounts groups by attribute value and counts non-deleted variants: it is the data a
// faceted /search filter renders (e.g. preto -> 3 variants, branco -> 2).
func TestFacetCounts_CountsVariantsPerValue(t *testing.T) {
	db, repo := newMock(t)
	productID := uuid.New()
	preto, branco := uuid.New(), uuid.New()
	pretoHex, brancoHex := "#000000", "#FFFFFF"

	db.ExpectQuery(regexp.QuoteMeta("COUNT(DISTINCT vav.variant_id)")).
		WithArgs(productID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "code", "value", "label", "hex", "position", "count",
		}).AddRow(preto, "cor", "preto", "Preto", &pretoHex, 0, 3).
			AddRow(branco, "cor", "branco", "Branco", &brancoHex, 1, 2))

	facets, err := repo.FacetCounts(context.Background(), productID)
	require.NoError(t, err)
	require.Len(t, facets, 2)
	assert.Equal(t, "cor", facets[0].AttributeCode)
	assert.Equal(t, "preto", facets[0].Value)
	assert.Equal(t, 3, facets[0].Count)
	assert.Equal(t, 2, facets[1].Count)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFacetCounts_Empty(t *testing.T) {
	db, repo := newMock(t)
	productID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("COUNT(DISTINCT vav.variant_id)")).
		WithArgs(productID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "code", "value", "label", "hex", "position", "count",
		}))

	facets, err := repo.FacetCounts(context.Background(), productID)
	require.NoError(t, err)
	assert.Empty(t, facets)
	assert.NoError(t, db.ExpectationsWereMet())
}
