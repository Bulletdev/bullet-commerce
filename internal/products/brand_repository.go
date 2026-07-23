package products

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// brandColumns is the canonical SELECT/scan order for brands so the column list and the
// scan destinations never drift apart.
const brandColumns = `id, name, slug, logo_url, created_at, updated_at`

var ErrBrandNotFound = errors.New("brand not found")

// BrandRepository is the minimal surface the catalog needs today: create a brand, look one
// up, and list them. Update/Delete are intentionally deferred until a route needs them.
type BrandRepository interface {
	Create(ctx context.Context, brand *models.Brand) (*models.Brand, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.Brand, error)
	List(ctx context.Context) ([]models.Brand, error)
}

type postgresBrandRepository struct {
	db DBPool
}

func NewPostgresBrandRepository(db *pgxpool.Pool) BrandRepository {
	return &postgresBrandRepository{db: db}
}

func (r *postgresBrandRepository) Create(ctx context.Context, brand *models.Brand) (*models.Brand, error) {
	query := `
		INSERT INTO brands (name, slug, logo_url)
		VALUES ($1, $2, $3)
		RETURNING ` + brandColumns + `
	`
	b := &models.Brand{}
	err := r.db.QueryRow(ctx, query, brand.Name, brand.Slug, brand.LogoURL).Scan(
		&b.ID, &b.Name, &b.Slug, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *postgresBrandRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Brand, error) {
	query := `SELECT ` + brandColumns + ` FROM brands WHERE id = $1`
	b := &models.Brand{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.Name, &b.Slug, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBrandNotFound
		}
		return nil, err
	}
	return b, nil
}

func (r *postgresBrandRepository) List(ctx context.Context) ([]models.Brand, error) {
	query := `SELECT ` + brandColumns + ` FROM brands ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Brand
	for rows.Next() {
		var b models.Brand
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}
