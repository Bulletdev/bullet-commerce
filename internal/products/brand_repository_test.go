package products

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

func newBrandMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresBrandRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresBrandRepository{db: db}
}

func brandCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "name", "slug", "logo_url", "created_at", "updated_at"})
}

func TestBrandCreate(t *testing.T) {
	db, repo := newBrandMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO brands")).
		WithArgs("Acme", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(brandCols().AddRow(id, "Acme", nil, nil, time.Now(), time.Now()))

	b, err := repo.Create(context.Background(), &models.Brand{Name: "Acme"})
	require.NoError(t, err)
	assert.Equal(t, id, b.ID)
	assert.Equal(t, "Acme", b.Name)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestBrandFindByID_Found(t *testing.T) {
	db, repo := newBrandMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, logo_url")).
		WithArgs(id).
		WillReturnRows(brandCols().AddRow(id, "Acme", nil, nil, time.Now(), time.Now()))

	b, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Acme", b.Name)
}

func TestBrandFindByID_NotFound(t *testing.T) {
	db, repo := newBrandMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, logo_url")).
		WithArgs(id).
		WillReturnRows(brandCols())

	_, err := repo.FindByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrBrandNotFound)
}

func TestBrandList(t *testing.T) {
	db, repo := newBrandMock(t)
	id1, id2 := uuid.New(), uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, logo_url")).
		WillReturnRows(brandCols().
			AddRow(id1, "Acme", nil, nil, time.Now(), time.Now()).
			AddRow(id2, "Globex", nil, nil, time.Now(), time.Now()))

	list, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestBrandList_DBError(t *testing.T) {
	db, repo := newBrandMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, logo_url")).
		WillReturnError(errors.New("db error"))

	_, err := repo.List(context.Background())
	assert.Error(t, err)
}

func TestNewPostgresBrandRepository(t *testing.T) {
	assert.NotNil(t, NewPostgresBrandRepository(nil))
}
