package users

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
)

func TestCreate_DBError(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO users")).
		WithArgs("X", "x@x.com", "hash").
		WillReturnError(errors.New("db timeout"))

	_, err := repo.Create(context.Background(), "X", "x@x.com", "hash")
	assert.Error(t, err)
}

func TestFindByEmail_DBError(t *testing.T) {
	db, repo := newMock(t)

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email")).
		WithArgs("e@e.com").
		WillReturnError(errors.New("connection lost"))

	_, err := repo.FindByEmail(context.Background(), "e@e.com")
	assert.Error(t, err)
}

func TestFindByID_DBError(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("SELECT id, name, email")).
		WithArgs(id).
		WillReturnError(errors.New("db error"))

	_, err := repo.FindByID(context.Background(), id)
	assert.Error(t, err)
}

func TestUpdate_NotFound(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), id).
		WillReturnRows(userCols())

	_, err := repo.Update(context.Background(), id, "X", "x@x.com", nil)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUpdate_DuplicateEmail(t *testing.T) {
	db, repo := newMock(t)
	id := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("UPDATE users")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), id).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	_, err := repo.Update(context.Background(), id, "X", "dup@dup.com", nil)
	assert.ErrorIs(t, err, ErrEmailAlreadyExists)
}
