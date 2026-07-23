// Package variants owns the ProductVariant entity of the Product aggregate and,
// with it, the stock invariant. Stock moves through three states so an order can
// hold inventory before payment without overselling:
//
//	Reserve  - at order creation: moves qty into stock_reserved iff available >= qty.
//	Claim    - at payment confirmation: converts the reservation into a real sale
//	           (stock -= qty, stock_reserved -= qty).
//	Release  - at cancellation/expiration: frees the reservation (stock_reserved -= qty)
//	           without touching physical stock.
//
// Since migration 000020 stock is held PER (variant, source): the three operations take a
// sourceID and act on the variant_stock row for that pair. With a single default source the
// observable behavior is identical to the pre-sourcing world (one implicit location). The
// deprecated product_variants.stock / stock_reserved columns stay for the display read path
// (FindByID/FindByProductID still return them) until it is migrated to AvailableForVariant,
// which sums live availability across every source.
//
// Reserve/Claim/Release each take a DBExecutor so they can run on the pool OR inside
// the order's transaction - the atomic UPDATE ... WHERE available >= qty is what makes
// concurrent checkouts safe (no read-modify-write race).
//
// Acceptance criteria (covered by pgxmock tests in repository_test.go):
//   - stock=10, reserved=0, Reserve(3, source)    -> reserved=3, RowsAffected=1.
//   - available=2, Reserve(3, source)             -> ErrInsufficientStock, no change.
//   - reserved=3, Release(3, source)              -> reserved=0.
//   - stock=10, reserved=3, Claim(3, source)      -> stock=7, reserved=0.
//   - AvailableForVariant sums (stock - stock_reserved) across all sources.
//   - FindByProductID never returns soft-deleted variants.
package variants

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBExecutor is the subset of pgx used by the stock operations. Both *pgxpool.Pool
// and pgx.Tx satisfy it, so Reserve/Claim/Release run identically whether called
// standalone or inside the order transaction that must commit them atomically.
type DBExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	ErrVariantNotFound    = errors.New("variant not found")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrStockClaimConflict = errors.New("stock claim conflict")
)

// StockBelowReservedError is returned by SetStock when an absolute stock write would fall below
// the units already reserved at that source (which would make available negative). It carries
// the current reservation so the handler can tell the admin exactly why the write was refused.
type StockBelowReservedError struct{ Reserved int }

func (e *StockBelowReservedError) Error() string {
	return fmt.Sprintf("cannot set stock below the %d unit(s) reserved at this source", e.Reserved)
}

type VariantRepository interface {
	Create(ctx context.Context, variant *models.ProductVariant) (*models.ProductVariant, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error)
	// FindPublishedByID is the PUBLIC-safe read: it returns the variant only when it is active
	// and non-deleted AND its parent product is active and non-deleted, so no stock leaks for
	// inactive variants or draft/archived/deleted products.
	FindPublishedByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error)
	FindByProductID(ctx context.Context, productID uuid.UUID) ([]models.ProductVariant, error)
	Reserve(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Release(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	Claim(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	// Restock is the inverse of Claim: it returns physical stock to a (variant, source) row
	// (e.g. a refund/cancellation) without touching stock_reserved.
	Restock(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error
	// AvailableForVariant sums available stock (stock - stock_reserved) across every source
	// for a variant - the value the display/read path shows now that stock is per-source.
	AvailableForVariant(ctx context.Context, variantID uuid.UUID) (int, error)
	// SetStock overwrites physical stock for a (variant, source) as an ABSOLUTE value (not a
	// delta), UPSERTing the variant_stock row Reserve/Claim/Release actually read. It does NOT
	// touch stock_reserved, so open reservations are preserved, and refuses to drop below the
	// reserved units (StockBelowReservedError). Returns the post-write stock and reserved at the
	// touched source.
	SetStock(ctx context.Context, variantID, sourceID uuid.UUID, stock int) (int, int, error)
}

type postgresVariantRepository struct {
	db DBExecutor
}

func NewPostgresVariantRepository(db *pgxpool.Pool) VariantRepository {
	return &postgresVariantRepository{db: db}
}

func (r *postgresVariantRepository) Create(ctx context.Context, variant *models.ProductVariant) (*models.ProductVariant, error) {
	// active / position / stock_policy are deliberately NOT in the INSERT column list: a zero-value
	// bool (false) or empty stock_policy ("" - outside the CHECK) would fight the DB defaults, so we
	// let the defaults apply and RETURN them back into the struct. Available stays derived (read-only),
	// so it is never written or returned here.
	query := `
		INSERT INTO product_variants
			(product_id, sku, attributes, price_cents, price_inherited, currency, stock, stock_reserved,
			 weight_grams, length_mm, width_mm, height_mm, barcode, compare_at_price_cents)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, active, position, stock_policy, deleted_at, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		variant.ProductID, variant.SKU, variant.Attributes, variant.PriceCents, variant.PriceInherited,
		variant.Currency, variant.Stock, variant.StockReserved,
		variant.WeightGrams, variant.LengthMM, variant.WidthMM, variant.HeightMM,
		variant.Barcode, variant.CompareAtPriceCents,
	).Scan(&variant.ID, &variant.Active, &variant.Position, &variant.StockPolicy,
		&variant.DeletedAt, &variant.CreatedAt, &variant.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return variant, nil
}

// variantSelectColumns is the shared read projection. The trailing correlated subquery derives
// stock_available with the SAME formula as AvailableForVariant (Σ stock - stock_reserved across
// sources, COALESCE 0), so the read path shows live per-source availability without an N+1 of
// AvailableForVariant calls. It is a computed value, not a column on product_variants.
const variantSelectColumns = `
	id, product_id, sku, attributes, price_cents, price_inherited, currency,
	weight_grams, length_mm, width_mm, height_mm, barcode, active, position,
	compare_at_price_cents, stock_policy, stock, stock_reserved, deleted_at, created_at, updated_at,
	COALESCE((SELECT SUM(vs.stock - vs.stock_reserved) FROM variant_stock vs
	          WHERE vs.variant_id = product_variants.id), 0) AS stock_available
`

// scanVariant reads one row in variantSelectColumns order, including the derived stock_available.
func scanVariant(row pgx.Row, v *models.ProductVariant) error {
	return row.Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Attributes, &v.PriceCents, &v.PriceInherited, &v.Currency,
		&v.WeightGrams, &v.LengthMM, &v.WidthMM, &v.HeightMM, &v.Barcode, &v.Active, &v.Position,
		&v.CompareAtPriceCents, &v.StockPolicy, &v.Stock, &v.StockReserved, &v.DeletedAt,
		&v.CreatedAt, &v.UpdatedAt, &v.Available,
	)
}

func (r *postgresVariantRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error) {
	query := `SELECT ` + variantSelectColumns + `
		FROM product_variants
		WHERE id = $1 AND deleted_at IS NULL`
	v := &models.ProductVariant{}
	if err := scanVariant(r.db.QueryRow(ctx, query, id), v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVariantNotFound
		}
		return nil, err
	}
	return v, nil
}

// FindPublishedByID returns the variant only if it is itself sellable (active, not soft-deleted)
// AND its parent product is published (active, not soft-deleted). WHY EXISTS instead of a JOIN:
// variantSelectColumns is unqualified, so joining products would make shared column names (id,
// price_cents, currency, dims, timestamps…) ambiguous; a correlated EXISTS enforces the same
// parent gate while letting the projection be reused verbatim. A miss (variant gone/inactive or
// parent unpublished) reads as ErrVariantNotFound - the caller can't tell the reasons apart, which
// is the point (no leak of why).
func (r *postgresVariantRepository) FindPublishedByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error) {
	query := `SELECT ` + variantSelectColumns + `
		FROM product_variants
		WHERE product_variants.id = $1
		  AND product_variants.active = TRUE
		  AND product_variants.deleted_at IS NULL
		  AND EXISTS (
		      SELECT 1 FROM products p
		      WHERE p.id = product_variants.product_id
		        AND p.deleted_at IS NULL
		        AND p.status = 'active'
		  )`
	v := &models.ProductVariant{}
	if err := scanVariant(r.db.QueryRow(ctx, query, id), v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVariantNotFound
		}
		return nil, err
	}
	return v, nil
}

func (r *postgresVariantRepository) FindByProductID(ctx context.Context, productID uuid.UUID) ([]models.ProductVariant, error) {
	query := `SELECT ` + variantSelectColumns + `
		FROM product_variants
		WHERE product_id = $1 AND deleted_at IS NULL
		ORDER BY position ASC, created_at ASC`
	rows, err := r.db.Query(ctx, query, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.ProductVariant
	for rows.Next() {
		var v models.ProductVariant
		if err := scanVariant(rows, &v); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (r *postgresVariantRepository) Reserve(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	// The `(stock - stock_reserved) >= $1` guard is the invariant enforced atomically on the
	// (variant, source) row: only rows with enough available stock at THIS source are updated,
	// so two concurrent checkouts can never both succeed past the last unit. Zero rows means
	// it was not available at this source (or the pair has no stock row).
	query := `
		UPDATE variant_stock
		SET stock_reserved = stock_reserved + $1, updated_at = NOW()
		WHERE variant_id = $2 AND source_id = $3 AND (stock - stock_reserved) >= $1
	`
	result, err := exec.Exec(ctx, query, qty, variantID, sourceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrInsufficientStock
	}
	return nil
}

func (r *postgresVariantRepository) Release(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	// GREATEST(..., 0) keeps stock_reserved from underflowing if a release is ever
	// replayed (idempotent-ish): freeing more than is reserved just lands at zero.
	query := `
		UPDATE variant_stock
		SET stock_reserved = GREATEST(stock_reserved - $1, 0), updated_at = NOW()
		WHERE variant_id = $2 AND source_id = $3
	`
	result, err := exec.Exec(ctx, query, qty, variantID, sourceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrVariantNotFound
	}
	return nil
}

func (r *postgresVariantRepository) SetStock(ctx context.Context, variantID, sourceID uuid.UUID, stock int) (int, int, error) {
	// Absolute SET (not a delta), UPSERTing the variant_stock row for this (variant, source).
	// stock_reserved is left untouched - an admin correction must never drop live reservations.
	// The ON CONFLICT guard refuses to lower stock below what is already reserved at this source
	// (that would drive available negative): a refused conflict updates nothing, so RETURNING
	// yields no row and we surface StockBelowReservedError rather than a raw CHECK violation.
	var outStock, outReserved int
	err := r.db.QueryRow(ctx, `
		INSERT INTO variant_stock (variant_id, source_id, stock, stock_reserved)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (variant_id, source_id) DO UPDATE
			SET stock = EXCLUDED.stock, updated_at = NOW()
			WHERE variant_stock.stock_reserved <= EXCLUDED.stock
		RETURNING stock, stock_reserved
	`, variantID, sourceID, stock).Scan(&outStock, &outReserved)
	if err == nil {
		return outStock, outReserved, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, err
	}
	// No row returned: the guard refused the write because the new stock is below what is
	// reserved at this source. Read the current reservation to explain the refusal precisely.
	var reserved int
	if e := r.db.QueryRow(ctx,
		`SELECT stock_reserved FROM variant_stock WHERE variant_id = $1 AND source_id = $2`,
		variantID, sourceID).Scan(&reserved); e != nil {
		return 0, 0, e
	}
	return 0, 0, &StockBelowReservedError{Reserved: reserved}
}

func (r *postgresVariantRepository) Claim(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	// The `stock >= $1` guard prevents claiming more than physically exists at this source;
	// the reservation is consumed alongside the physical decrement so the two stay in sync.
	query := `
		UPDATE variant_stock
		SET stock = stock - $1, stock_reserved = GREATEST(stock_reserved - $1, 0), updated_at = NOW()
		WHERE variant_id = $2 AND source_id = $3 AND stock >= $1
	`
	result, err := exec.Exec(ctx, query, qty, variantID, sourceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrStockClaimConflict
	}
	return nil
}

// Restock is the inverse of Claim's physical decrement: it returns qty to the on-hand stock of a
// (variant, source) row without touching stock_reserved - a refund/cancellation restores goods that
// have no live reservation to free. Like Claim/Release it takes a DBExecutor so it can run inside the
// order transaction that owns the refund. Zero rows means the (variant, source) pair has no stock row.
func (r *postgresVariantRepository) Restock(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	query := `
		UPDATE variant_stock
		SET stock = stock + $1, updated_at = NOW()
		WHERE variant_id = $2 AND source_id = $3
	`
	result, err := exec.Exec(ctx, query, qty, variantID, sourceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrVariantNotFound
	}
	return nil
}

// AvailableForVariant sums live availability across every source for a variant. WHY on the pool
// (not an executor arg): it is a read for display, never part of the reserve/claim transaction.
func (r *postgresVariantRepository) AvailableForVariant(ctx context.Context, variantID uuid.UUID) (int, error) {
	var available int
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(stock - stock_reserved), 0) FROM variant_stock WHERE variant_id = $1`,
		variantID,
	).Scan(&available)
	if err != nil {
		return 0, err
	}
	return available, nil
}
