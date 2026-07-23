package users

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBPool is the subset of pgxpool.Pool used by this package — satisfied by
// both *pgxpool.Pool in production and pgxmock.PgxPoolIface in tests.
type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

type UserRepository interface {
	Create(ctx context.Context, name, email, passwordHash string) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	Update(ctx context.Context, id uuid.UUID, name, email string, cpf *string) (*models.User, error)
}

type postgresUserRepository struct {
	db DBPool
}

func NewPostgresUserRepository(db *pgxpool.Pool) UserRepository {
	return &postgresUserRepository{db: db}
}

func (r *postgresUserRepository) Create(ctx context.Context, name, email, passwordHash string) (*models.User, error) {
	query := `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, name, email, cpf, role, created_at, updated_at
	`
	user := &models.User{}
	err := r.db.QueryRow(ctx, query, name, email, passwordHash).Scan(
		&user.ID, &user.Name, &user.Email, &user.CPF, &user.Role,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailAlreadyExists
		}
		return nil, err
	}
	return user, nil
}

func (r *postgresUserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, name, email, cpf, role, password_hash, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	user := &models.User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Name, &user.Email, &user.CPF, &user.Role,
		&user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *postgresUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, name, email, cpf, role, password_hash, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	user := &models.User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Name, &user.Email, &user.CPF, &user.Role,
		&user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *postgresUserRepository) Update(ctx context.Context, id uuid.UUID, name, email string, cpf *string) (*models.User, error) {
	query := `
		UPDATE users
		SET name = $1, email = $2, cpf = $3, updated_at = NOW()
		WHERE id = $4
		RETURNING id, name, email, cpf, role, created_at, updated_at
	`
	user := &models.User{}
	err := r.db.QueryRow(ctx, query, name, email, cpf, id).Scan(
		&user.ID, &user.Name, &user.Email, &user.CPF, &user.Role,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailAlreadyExists
		}
		return nil, err
	}
	return user, nil
}
