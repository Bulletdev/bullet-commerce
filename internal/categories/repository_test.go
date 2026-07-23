package categories

import (
	"bullet-commerce/internal/models"
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresCategoryRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresCategoryRepository{db: db}
}

func catCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "name", "created_at", "updated_at"})
}

func addCatRow(rows *pgxmock.Rows, id uuid.UUID, name string) *pgxmock.Rows {
	return rows.AddRow(id, name, time.Now(), time.Now())
}

func TestCreate_Success(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO categories")).
		WithArgs("Go").
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(id, now, now))

	cat, err := repo.Create(context.Background(), &models.Category{Name: "Go"})
	require.NoError(t, err)
	assert.Equal(t, id, cat.ID)
}

func TestCreate_DuplicateName(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO categories")).
		WithArgs("Dup").
		WillReturnError(&pgconn.PgError{Code: "23505"})

	_, err := repo.Create(context.Background(), &models.Category{Name: "Dup"})
	assert.ErrorIs(t, err, ErrCategoryNameExists)
}

func TestFindByID_Found(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, created_at, updated_at FROM categories WHERE id")).
		WithArgs(id).
		WillReturnRows(addCatRow(catCols(), id, "Java"))

	cat, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "Java", cat.Name)
}

func TestFindByID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name")).
		WithArgs(id).
		WillReturnRows(catCols())

	_, err := repo.FindByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrCategoryNotFound)
}

func TestFindByName_Found(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("WHERE name = $1")).
		WithArgs("Go").
		WillReturnRows(addCatRow(catCols(), id, "Go"))

	cat, err := repo.FindByName(context.Background(), "Go")
	require.NoError(t, err)
	assert.Equal(t, "Go", cat.Name)
}

func TestFindByName_NotFound(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("WHERE name = $1")).
		WithArgs("Missing").
		WillReturnRows(catCols())

	_, err := repo.FindByName(context.Background(), "Missing")
	assert.ErrorIs(t, err, ErrCategoryNotFound)
}

func TestFindAll(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, created_at, updated_at FROM categories ORDER BY name")).
		WillReturnRows(addCatRow(catCols(), id, "Java"))

	list, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestFindAll_Empty(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name")).
		WillReturnRows(catCols())

	list, err := repo.FindAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUpdate_Success(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE categories")).
		WithArgs("Updated", id).
		WillReturnRows(pgxmock.NewRows([]string{"updated_at"}).AddRow(now))

	cat, err := repo.Update(context.Background(), id, &models.Category{Name: "Updated"})
	require.NoError(t, err)
	assert.Equal(t, id, cat.ID)
}

func TestUpdate_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE categories")).
		WithArgs("X", id).
		WillReturnRows(pgxmock.NewRows([]string{"updated_at"}))

	_, err := repo.Update(context.Background(), id, &models.Category{Name: "X"})
	assert.ErrorIs(t, err, ErrCategoryNotFound)
}

func TestUpdate_DuplicateName(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE categories")).
		WithArgs("Dup", id).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	_, err := repo.Update(context.Background(), id, &models.Category{Name: "Dup"})
	assert.ErrorIs(t, err, ErrCategoryNameExists)
}

func TestDelete_Success(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM categories WHERE id")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	assert.NoError(t, repo.Delete(context.Background(), id))
}

func TestDelete_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectExec(regexp.QuoteMeta("DELETE FROM categories WHERE id")).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	assert.ErrorIs(t, repo.Delete(context.Background(), id), ErrCategoryNotFound)
}

func TestHandlePgError_NonUnique(t *testing.T) {
	err := handlePgError(&pgconn.PgError{Code: "99999", Message: "other error"})
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrCategoryNameExists)
}

func TestHandlePgError_NonPgError(t *testing.T) {
	regular := assert.AnError
	assert.Equal(t, regular, handlePgError(regular))
}
