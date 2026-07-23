// Package media owns product_media: images/videos attached to a product or one of its
// variants. It is a thin persistence layer — the API never stores files, only rows that
// reference external CDN/bucket URLs (PRD Catalog v2 §3.1). Two ingestion flows converge
// here with the same row shape: an admin referencing an existing CDN URL, or an admin
// registering the public URL produced by a presigned upload (see internal/storage).
package media

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMediaNotFound = errors.New("media not found")

// dbExecutor is the subset of pgx used here. Both *pgxpool.Pool and pgxmock's pool iface
// satisfy it, so the repository runs against the real pool in production and against
// pgxmock in unit tests without a live database.
type dbExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type MediaRepository interface {
	Create(ctx context.Context, m *models.ProductMedia) (*models.ProductMedia, error)
	ListByProduct(ctx context.Context, productID uuid.UUID) ([]models.ProductMedia, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type postgresMediaRepository struct {
	db dbExecutor
}

func NewPostgresMediaRepository(db *pgxpool.Pool) MediaRepository {
	return &postgresMediaRepository{db: db}
}

func (r *postgresMediaRepository) Create(ctx context.Context, m *models.ProductMedia) (*models.ProductMedia, error) {
	query := `
		INSERT INTO product_media (product_id, variant_id, url, alt, kind, position)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		m.ProductID, m.VariantID, m.URL, m.Alt, m.Kind, m.Position,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ListByProduct returns a product's whole gallery — product-level media (variant_id NULL)
// and every variant's media — ordered so the primary (lowest position) comes first.
func (r *postgresMediaRepository) ListByProduct(ctx context.Context, productID uuid.UUID) ([]models.ProductMedia, error) {
	query := `
		SELECT id, product_id, variant_id, url, alt, kind, position, created_at, updated_at
		FROM product_media
		WHERE product_id = $1
		ORDER BY position ASC, created_at ASC
	`
	rows, err := r.db.Query(ctx, query, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.ProductMedia
	for rows.Next() {
		var m models.ProductMedia
		if err := rows.Scan(
			&m.ID, &m.ProductID, &m.VariantID, &m.URL, &m.Alt, &m.Kind, &m.Position,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (r *postgresMediaRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// Hard delete: media rows carry no history worth soft-deleting, and the DB CASCADE
	// already removes them when the owning product/variant goes away.
	result, err := r.db.Exec(ctx, `DELETE FROM product_media WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrMediaNotFound
	}
	return nil
}
