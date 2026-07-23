package categories

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	ErrCategoryNotFound   = errors.New("category not found")
	ErrCategoryNameExists = errors.New("category name already exists")
)

// CategoryRepository defines the interface for category data operations.
type CategoryRepository interface {
	Create(ctx context.Context, category *models.Category) (*models.Category, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error)
	FindAll(ctx context.Context) ([]models.Category, error)
	Update(ctx context.Context, id uuid.UUID, category *models.Category) (*models.Category, error)
	Delete(ctx context.Context, id uuid.UUID) error
	FindByName(ctx context.Context, name string) (*models.Category, error) // Added for checking uniqueness
}

// postgresCategoryRepository implements CategoryRepository using PostgreSQL.
type postgresCategoryRepository struct {
	db DBPool
}

func NewPostgresCategoryRepository(db *pgxpool.Pool) CategoryRepository {
	return &postgresCategoryRepository{db: db}
}

// handlePgError checks for common PostgreSQL errors like unique constraint violations.
func handlePgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" { // unique_violation
			// Could check pgErr.ConstraintName if more specific errors are needed
			return ErrCategoryNameExists
		}
	}
	return err
}

// Create inserts a new category into the database.
func (r *postgresCategoryRepository) Create(ctx context.Context, category *models.Category) (*models.Category, error) {
	query := `
		INSERT INTO categories (name)
		VALUES ($1)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query, category.Name).Scan(&category.ID, &category.CreatedAt, &category.UpdatedAt)
	if err != nil {
		return nil, handlePgError(err)
	}
	return category, nil
}

// FindByID retrieves a category by its ID.
func (r *postgresCategoryRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	query := `SELECT id, name, created_at, updated_at FROM categories WHERE id = $1`
	category := &models.Category{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&category.ID, &category.Name, &category.CreatedAt, &category.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, err
	}
	return category, nil
}

// FindByName retrieves a category by its name.
func (r *postgresCategoryRepository) FindByName(ctx context.Context, name string) (*models.Category, error) {
	query := `SELECT id, name, created_at, updated_at FROM categories WHERE name = $1`
	category := &models.Category{}
	err := r.db.QueryRow(ctx, query, name).Scan(
		&category.ID, &category.Name, &category.CreatedAt, &category.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, err
	}
	return category, nil
}

// FindAll retrieves all categories.
func (r *postgresCategoryRepository) FindAll(ctx context.Context) ([]models.Category, error) {
	query := `SELECT id, name, created_at, updated_at FROM categories ORDER BY name ASC`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Category])
	if err != nil {
		return nil, err
	}
	return categories, nil
}

// Update modifies an existing category in the database.
func (r *postgresCategoryRepository) Update(ctx context.Context, id uuid.UUID, category *models.Category) (*models.Category, error) {
	query := `
		UPDATE categories
		SET name = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING updated_at
	`
	err := r.db.QueryRow(ctx, query, category.Name, id).Scan(&category.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, handlePgError(err) // Check for unique constraint violation on name
	}
	category.ID = id
	// category.CreatedAt needs separate fetch if needed
	return category, nil
}

// Delete removes a category from the database.
func (r *postgresCategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Note: Products associated with this category will have their category_id set to NULL due to ON DELETE SET NULL
	query := `DELETE FROM categories WHERE id = $1`
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrCategoryNotFound
	}
	return nil
}
