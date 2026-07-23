package products

import (
	"bullet-commerce/internal/models"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// productColumns is the canonical SELECT/scan order shared by every read query so
// the column list and the scan destinations never drift apart.
const productColumns = `id, name, description, price_cents, currency, category_id, ` +
	// `stock` (top-level) is a live rollup of real sellable inventory: the sum of
	// (stock - stock_reserved) across every variant_stock row of the product's variants.
	// The deprecated products.stock column is no longer read here (it drifted from the
	// per-(variant, source) truth Reserve/Claim/Release operate on).
	`COALESCE((SELECT SUM(vs.stock - vs.stock_reserved) FROM variant_stock vs ` +
	`JOIN product_variants pv ON pv.id = vs.variant_id ` +
	`WHERE pv.product_id = products.id AND pv.deleted_at IS NULL), 0) AS stock, ` +
	`featured, type, attributes, variant_variation_attributes, ` +
	`weight_grams, length_mm, width_mm, height_mm, ` +
	`ncm, cest, origem, unit, ` +
	`status, slug, meta_title, meta_description, brand_id, compare_at_price_cents, ` +
	`version, rating_avg, COALESCE(rating_count, 0), ` +
	`deleted_at, created_at, updated_at`

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	ErrProductNotFound = errors.New("product not found")
	// ErrProductVersionConflict is returned by Update when the row still exists but its
	// version no longer matches the caller's expected value — a concurrent edit won the race.
	ErrProductVersionConflict = errors.New("product version conflict")
)

type ProductRepository interface {
	Create(ctx context.Context, product *models.Product) (*models.Product, error)
	// FindByID is the PUBLIC read: it hides non-active (draft/archived) and soft-deleted
	// products. Admin paths that must reach drafts use FindByIDAdmin.
	FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error)
	// FindByIDAdmin is the admin read: it returns any non-soft-deleted product regardless of
	// publish status, so drafts/archived items can be managed.
	FindByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Product, error)
	FindAll(ctx context.Context, limit, offset int) ([]models.Product, error)
	FindFeatured(ctx context.Context) ([]models.Product, error)
	FindByCategoryID(ctx context.Context, categoryID uuid.UUID, limit, offset int) ([]models.Product, error)
	Search(ctx context.Context, query string, limit, offset int) ([]models.Product, error)
	Update(ctx context.Context, id uuid.UUID, product *models.Product) (*models.Product, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStock(ctx context.Context, id uuid.UUID, stock int) error
	// SetCategories replaces the product's N:N (secondary) category memberships. The
	// primary category stays on products.category_id.
	SetCategories(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error
	FindCategoryIDs(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error)
}

type postgresProductRepository struct {
	db DBPool
}

func NewPostgresProductRepository(db *pgxpool.Pool) ProductRepository {
	return &postgresProductRepository{db: db}
}

func (r *postgresProductRepository) Create(ctx context.Context, product *models.Product) (*models.Product, error) {
	normalizeProduct(product)
	query := `
		INSERT INTO products (
			name, description, price_cents, currency, category_id, stock, featured, type, attributes, variant_variation_attributes,
			weight_grams, length_mm, width_mm, height_mm,
			ncm, cest, origem, unit,
			status, slug, meta_title, meta_description, brand_id, compare_at_price_cents
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
		RETURNING id, type, attributes, variant_variation_attributes, stock, featured, version, deleted_at, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		product.Name, product.Description, product.PriceCents, product.Currency,
		product.CategoryID, product.Stock, product.Featured,
		product.Type, product.Attributes, product.VariantVariationAttributes,
		product.WeightGrams, product.LengthMM, product.WidthMM, product.HeightMM,
		product.NCM, product.CEST, product.Origem, product.Unit,
		product.Status, product.Slug, product.MetaTitle, product.MetaDescription,
		product.BrandID, product.CompareAtPriceCents,
	).Scan(&product.ID, &product.Type, &product.Attributes, &product.VariantVariationAttributes,
		&product.Stock, &product.Featured, &product.Version, &product.DeletedAt,
		&product.CreatedAt, &product.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return product, nil
}

// normalizeProduct fills catalog fields with their defaults so the DB CHECK/NOT NULL
// constraints are satisfied even when the caller left them zero-valued. It mirrors the
// column defaults in migration 000012.
func normalizeProduct(product *models.Product) {
	if product.Type == "" {
		product.Type = models.ProductTypeSimple
	}
	if len(product.Attributes) == 0 {
		product.Attributes = json.RawMessage(`{}`)
	}
	if product.VariantVariationAttributes == nil {
		// A nil slice would encode as SQL NULL, violating the NOT NULL column; an
		// empty slice encodes as the empty array the column defaults to.
		product.VariantVariationAttributes = []string{}
	}
	// Status and Unit are NOT NULL with a CHECK/default at the DB; since Create passes them
	// explicitly, a zero value would fail the constraint, so mirror the column defaults here.
	if product.Status == "" {
		product.Status = models.ProductStatusActive
	}
	if product.Unit == "" {
		product.Unit = "UN"
	}
}

func (r *postgresProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	// Public detail: only published products, so a draft/archived id reads as "not found".
	query := `
		SELECT ` + productColumns + `
		FROM products
		WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
	`
	return r.scanOne(ctx, query, id)
}

// FindByIDAdmin is the admin-facing read: it drops the status filter so drafts and archived
// products remain reachable for management, keeping only the soft-delete guard.
func (r *postgresProductRepository) FindByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	query := `
		SELECT ` + productColumns + `
		FROM products
		WHERE id = $1 AND deleted_at IS NULL
	`
	return r.scanOne(ctx, query, id)
}

func (r *postgresProductRepository) FindAll(ctx context.Context, limit, offset int) ([]models.Product, error) {
	// Public catalog listing: only published products, on top of the soft-delete filter.
	query := `
		SELECT ` + productColumns + `
		FROM products
		WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	return r.scanMany(ctx, query, limit, offset)
}

func (r *postgresProductRepository) FindFeatured(ctx context.Context) ([]models.Product, error) {
	query := `
		SELECT ` + productColumns + `
		FROM products
		WHERE featured = TRUE AND deleted_at IS NULL AND status = 'active'
		ORDER BY created_at DESC
	`
	return r.scanMany(ctx, query)
}

func (r *postgresProductRepository) FindByCategoryID(ctx context.Context, categoryID uuid.UUID, limit, offset int) ([]models.Product, error) {
	query := `
		SELECT ` + productColumns + `
		FROM products
		WHERE category_id = $1 AND deleted_at IS NULL AND status = 'active'
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	return r.scanMany(ctx, query, categoryID, limit, offset)
}

func (r *postgresProductRepository) Search(ctx context.Context, query string, limit, offset int) ([]models.Product, error) {
	q := `
		SELECT ` + productColumns + `
		FROM products
		WHERE (name ILIKE $1 OR description ILIKE $1) AND deleted_at IS NULL AND status = 'active'
		ORDER BY
			CASE WHEN name ILIKE $1 THEN 1 ELSE 2 END,
			created_at DESC
		LIMIT $2 OFFSET $3
	`
	return r.scanMany(ctx, q, "%"+query+"%", limit, offset)
}

func (r *postgresProductRepository) Update(ctx context.Context, id uuid.UUID, product *models.Product) (*models.Product, error) {
	normalizeProduct(product)
	// Optimistic concurrency: the write only lands on the row whose version still matches the
	// caller's expected value ($25), and it bumps version so the next stale write misses. A
	// missed write returns ErrNoRows here; we then distinguish a genuine 404 from a version
	// conflict by checking whether a live row still exists.
	query := `
		UPDATE products
		SET name = $1, description = $2, price_cents = $3, currency = $4, category_id = $5, featured = $6,
			type = $7, attributes = $8, variant_variation_attributes = $9,
			weight_grams = $10, length_mm = $11, width_mm = $12, height_mm = $13,
			ncm = $14, cest = $15, origem = $16, unit = $17,
			status = $18, slug = $19, meta_title = $20, meta_description = $21,
			brand_id = $22, compare_at_price_cents = $23, version = version + 1, updated_at = NOW()
		WHERE id = $24 AND deleted_at IS NULL AND version = $25
		RETURNING ` + productColumns + `
	`
	updated, err := r.scanOne(ctx, query,
		product.Name, product.Description, product.PriceCents, product.Currency,
		product.CategoryID, product.Featured,
		product.Type, product.Attributes, product.VariantVariationAttributes,
		product.WeightGrams, product.LengthMM, product.WidthMM, product.HeightMM,
		product.NCM, product.CEST, product.Origem, product.Unit,
		product.Status, product.Slug, product.MetaTitle, product.MetaDescription,
		product.BrandID, product.CompareAtPriceCents, id, product.Version)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			// The UPDATE matched nothing: either the row is gone/soft-deleted (real 404) or
			// its version moved on under us (conflict). A live-row probe tells the two apart.
			live, existsErr := r.existsLive(ctx, id)
			if existsErr != nil {
				return nil, existsErr
			}
			if live {
				return nil, ErrProductVersionConflict
			}
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return updated, nil
}

// existsLive reports whether a non-soft-deleted product row exists for id, used to tell a
// version conflict apart from a genuine not-found after an optimistic UPDATE misses.
func (r *postgresProductRepository) existsLive(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM products WHERE id = $1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *postgresProductRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE products SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	return nil
}

func (r *postgresProductRepository) UpdateStock(ctx context.Context, id uuid.UUID, stock int) error {
	query := `UPDATE products SET stock = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	result, err := r.db.Exec(ctx, query, stock, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	return nil
}

// SetCategories replaces the product's secondary category memberships in one shot:
// clear the current set, then insert the given ids. The primary category on
// products.category_id is untouched. An empty/nil slice just clears the memberships.
func (r *postgresProductRepository) SetCategories(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM product_categories WHERE product_id = $1`, productID); err != nil {
		return err
	}
	for _, catID := range categoryIDs {
		// ON CONFLICT DO NOTHING keeps the call idempotent if the same id repeats.
		if _, err := r.db.Exec(ctx,
			`INSERT INTO product_categories (product_id, category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			productID, catID); err != nil {
			return err
		}
	}
	return nil
}

func (r *postgresProductRepository) FindCategoryIDs(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx,
		`SELECT category_id FROM product_categories WHERE product_id = $1 ORDER BY category_id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *postgresProductRepository) scanOne(ctx context.Context, query string, args ...any) (*models.Product, error) {
	p := &models.Product{}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Currency, &p.CategoryID,
		&p.Stock, &p.Featured, &p.Type, &p.Attributes, &p.VariantVariationAttributes,
		&p.WeightGrams, &p.LengthMM, &p.WidthMM, &p.HeightMM,
		&p.NCM, &p.CEST, &p.Origem, &p.Unit,
		&p.Status, &p.Slug, &p.MetaTitle, &p.MetaDescription, &p.BrandID, &p.CompareAtPriceCents,
		&p.Version, &p.RatingAvg, &p.RatingCount,
		&p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *postgresProductRepository) scanMany(ctx context.Context, query string, args ...any) ([]models.Product, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Product
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Currency, &p.CategoryID,
			&p.Stock, &p.Featured, &p.Type, &p.Attributes, &p.VariantVariationAttributes,
			&p.WeightGrams, &p.LengthMM, &p.WidthMM, &p.HeightMM,
			&p.NCM, &p.CEST, &p.Origem, &p.Unit,
			&p.Status, &p.Slug, &p.MetaTitle, &p.MetaDescription, &p.BrandID, &p.CompareAtPriceCents,
			&p.Version, &p.RatingAvg, &p.RatingCount,
			&p.DeletedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
