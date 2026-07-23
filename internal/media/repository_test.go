package media

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

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresMediaRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresMediaRepository{db: db}
}

func mediaCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "product_id", "variant_id", "url", "alt", "kind", "position", "created_at", "updated_at",
	})
}

func TestCreate_ReturnsGenerated(t *testing.T) {
	db, repo := newMock(t)
	id, productID := uuid.New(), uuid.New()
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO product_media")).
		WithArgs(productID, (*uuid.UUID)(nil), "https://cdn.example/x.jpg", (*string)(nil), models.MediaKindImage, 0).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(id, now, now))

	m, err := repo.Create(context.Background(), &models.ProductMedia{
		ProductID: productID,
		URL:       "https://cdn.example/x.jpg",
		Kind:      models.MediaKindImage,
	})
	require.NoError(t, err)
	assert.Equal(t, id, m.ID)
	assert.NoError(t, db.ExpectationsWereMet())
}

// A media row scoped to a variant carries a non-null variant_id straight through to the insert.
func TestCreate_WithVariant(t *testing.T) {
	db, repo := newMock(t)
	id, productID, variantID := uuid.New(), uuid.New(), uuid.New()
	alt := "blue colorway"
	now := time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO product_media")).
		WithArgs(productID, &variantID, "https://cdn.example/blue.jpg", &alt, models.MediaKindImage, 2).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(id, now, now))

	m, err := repo.Create(context.Background(), &models.ProductMedia{
		ProductID: productID,
		VariantID: &variantID,
		URL:       "https://cdn.example/blue.jpg",
		Alt:       &alt,
		Kind:      models.MediaKindImage,
		Position:  2,
	})
	require.NoError(t, err)
	assert.Equal(t, id, m.ID)
	assert.NoError(t, db.ExpectationsWereMet())
}

// ListByProduct returns the whole gallery ordered by position; the primary (lowest position)
// comes first regardless of insertion order.
func TestListByProduct_OrdersByPosition(t *testing.T) {
	db, repo := newMock(t)
	productID, variantID := uuid.New(), uuid.New()
	now := time.Now()

	rows := mediaCols().
		AddRow(uuid.New(), productID, (*uuid.UUID)(nil), "https://cdn/a.jpg", (*string)(nil), models.MediaKindImage, 0, now, now).
		AddRow(uuid.New(), productID, &variantID, "https://cdn/b.jpg", (*string)(nil), models.MediaKindImage, 1, now, now)

	db.ExpectQuery(regexp.QuoteMeta("WHERE product_id = $1")).
		WithArgs(productID).
		WillReturnRows(rows)

	list, err := repo.ListByProduct(context.Background(), productID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, 0, list[0].Position)
	assert.Nil(t, list[0].VariantID)
	assert.Equal(t, &variantID, list[1].VariantID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestDelete_Succeeds(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM product_media WHERE id = $1")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	require.NoError(t, repo.Delete(context.Background(), id))
	assert.NoError(t, db.ExpectationsWereMet())
}

// Deleting a row that does not exist (0 rows affected) surfaces ErrMediaNotFound so the
// handler can answer 404 rather than a silent success.
func TestDelete_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM product_media")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	err := repo.Delete(context.Background(), id)
	assert.ErrorIs(t, err, ErrMediaNotFound)
	assert.NoError(t, db.ExpectationsWereMet())
}
