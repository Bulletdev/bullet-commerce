package users

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

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresUserRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresUserRepository{db: db}
}

func userCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "name", "email", "cpf", "role", "created_at", "updated_at"})
}

func userWithHashCols() *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "name", "email", "cpf", "role", "password_hash", "created_at", "updated_at"})
}

func TestCreate_Success(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO users")).
		WithArgs("Alice", "alice@example.com", "hashed").
		WillReturnRows(userCols().AddRow(id, "Alice", "alice@example.com", nil, "user", now, now))

	user, err := repo.Create(context.Background(), "Alice", "alice@example.com", "hashed")
	require.NoError(t, err)
	assert.Equal(t, id, user.ID)
	assert.Equal(t, "Alice", user.Name)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByEmail_Found(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email, cpf, role, password_hash")).
		WithArgs("alice@example.com").
		WillReturnRows(userWithHashCols().AddRow(id, "Alice", "alice@example.com", nil, "user", "hash", now, now))

	user, err := repo.FindByEmail(context.Background(), "alice@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Alice", user.Name)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestFindByEmail_NotFound(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email")).
		WithArgs("ghost@example.com").
		WillReturnRows(userWithHashCols())

	_, err := repo.FindByEmail(context.Background(), "ghost@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestFindByID_Found(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email, cpf, role, password_hash")).
		WithArgs(id).
		WillReturnRows(userWithHashCols().AddRow(id, "Bob", "bob@example.com", nil, "admin", "hash", now, now))

	user, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, models.RoleAdmin, user.Role)
}

func TestFindByID_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email")).
		WithArgs(id).
		WillReturnRows(userWithHashCols())

	_, err := repo.FindByID(context.Background(), id)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUpdate_Success(t *testing.T) {
	db, repo := newMock(t)
	id, now := uuid.New(), time.Now()
	cpf := "12345678901"

	db.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs("New Name", "new@example.com", &cpf, id).
		WillReturnRows(userCols().AddRow(id, "New Name", "new@example.com", &cpf, "user", now, now))

	user, err := repo.Update(context.Background(), id, "New Name", "new@example.com", &cpf)
	require.NoError(t, err)
	assert.Equal(t, "New Name", user.Name)
}
