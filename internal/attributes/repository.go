// Package attributes owns the normalized VARIATION attributes of the catalog (§3.7 hybrid):
// the attribute / attribute_value / variant_attribute_value tables that make the variation
// subset of a variant's free-form JSONB queryable - facetable, validated, ordered, swatchable.
// The JSONB stays the source of truth for free-form metadata; these tables are a projection of
// its variation keys. Integration into /search and the product read model is a follow-up.
package attributes

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBExecutor is the subset of pgx used here. Both *pgxpool.Pool and pgx.Tx satisfy it, so a
// LinkVariant issued while building a variant can run inside that variant's transaction.
type DBExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var ErrAttributeNotFound = errors.New("attribute not found")

type AttributeRepository interface {
	FindByCode(ctx context.Context, code string) (*models.Attribute, error)
	ListValues(ctx context.Context, attributeID uuid.UUID) ([]models.AttributeValue, error)
	LinkVariant(ctx context.Context, variantID, attributeValueID uuid.UUID) error
	ValuesForVariant(ctx context.Context, variantID uuid.UUID) ([]models.AttributeValue, error)
	// FacetCounts returns, per attribute value, how many non-deleted variants of the product
	// carry it - the raw material for a faceted /search filter.
	FacetCounts(ctx context.Context, productID uuid.UUID) ([]models.AttributeFacet, error)
}

type postgresAttributeRepository struct {
	db DBExecutor
}

func NewPostgresAttributeRepository(db *pgxpool.Pool) AttributeRepository {
	return &postgresAttributeRepository{db: db}
}

func (r *postgresAttributeRepository) FindByCode(ctx context.Context, code string) (*models.Attribute, error) {
	query := `
		SELECT id, code, label, kind, created_at, updated_at
		FROM attribute
		WHERE code = $1
	`
	a := &models.Attribute{}
	err := r.db.QueryRow(ctx, query, code).Scan(
		&a.ID, &a.Code, &a.Label, &a.Kind, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttributeNotFound
		}
		return nil, err
	}
	return a, nil
}

func (r *postgresAttributeRepository) ListValues(ctx context.Context, attributeID uuid.UUID) ([]models.AttributeValue, error) {
	// Ordered by position so the UI shows M/G/GG in intended order; value is the stable
	// tiebreaker for values that share a position (e.g. the un-curated backfill default).
	query := `
		SELECT id, attribute_id, value, label, hex, position
		FROM attribute_value
		WHERE attribute_id = $1
		ORDER BY position ASC, value ASC
	`
	rows, err := r.db.Query(ctx, query, attributeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AttributeValue
	for rows.Next() {
		var v models.AttributeValue
		if err := rows.Scan(&v.ID, &v.AttributeID, &v.Value, &v.Label, &v.Hex, &v.Position); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (r *postgresAttributeRepository) LinkVariant(ctx context.Context, variantID, attributeValueID uuid.UUID) error {
	// ON CONFLICT DO NOTHING makes linking idempotent: the composite PK means re-linking the
	// same pair is a no-op rather than a duplicate-key error.
	_, err := r.db.Exec(ctx, `
		INSERT INTO variant_attribute_value (variant_id, attribute_value_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, variantID, attributeValueID)
	return err
}

func (r *postgresAttributeRepository) ValuesForVariant(ctx context.Context, variantID uuid.UUID) ([]models.AttributeValue, error) {
	query := `
		SELECT av.id, av.attribute_id, av.value, av.label, av.hex, av.position
		FROM attribute_value av
		JOIN variant_attribute_value vav ON vav.attribute_value_id = av.id
		WHERE vav.variant_id = $1
		ORDER BY av.position ASC, av.value ASC
	`
	rows, err := r.db.Query(ctx, query, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AttributeValue
	for rows.Next() {
		var v models.AttributeValue
		if err := rows.Scan(&v.ID, &v.AttributeID, &v.Value, &v.Label, &v.Hex, &v.Position); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (r *postgresAttributeRepository) FacetCounts(ctx context.Context, productID uuid.UUID) ([]models.AttributeFacet, error) {
	// COUNT(DISTINCT variant) guards against double counting if a variant were ever linked to
	// the same value twice; soft-deleted variants are excluded so facets match sellable stock.
	query := `
		SELECT av.id, a.code, av.value, av.label, av.hex, av.position,
		       COUNT(DISTINCT vav.variant_id) AS count
		FROM attribute_value av
		JOIN attribute a ON a.id = av.attribute_id
		JOIN variant_attribute_value vav ON vav.attribute_value_id = av.id
		JOIN product_variants v ON v.id = vav.variant_id
		WHERE v.product_id = $1 AND v.deleted_at IS NULL
		GROUP BY av.id, a.code, av.value, av.label, av.hex, av.position
		ORDER BY a.code ASC, av.position ASC, av.value ASC
	`
	rows, err := r.db.Query(ctx, query, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.AttributeFacet
	for rows.Next() {
		var f models.AttributeFacet
		if err := rows.Scan(
			&f.AttributeValueID, &f.AttributeCode, &f.Value, &f.Label, &f.Hex, &f.Position, &f.Count,
		); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}
