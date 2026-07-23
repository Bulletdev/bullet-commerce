// Package sourcing decides WHICH stock location each ordered unit is fulfilled from.
// It is the seam (PRD §3.2 / §4.5) that generalizes the implicit single location into N
// sources without rewriting the stock primitives: variants.Reserve/Claim/Release already act
// per (variant, source), and this package only chooses the source.
//
// V1 ships the SingleSourceAllocator: every unit is allocated from the default source, so with
// one source the sourcing layer is transparent — the order path behaves exactly as before.
// Scale swaps in a MultiSourceAllocator (by proximity/priority) behind the same Allocator port
// with no change to the order repository.
package sourcing

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AllocItem is one line the caller needs sourced: a variant and how many units.
type AllocItem struct {
	VariantID uuid.UUID
	Qty       int
}

// Allocation is the Allocator's verdict for (part of) a line: take Qty units of VariantID from
// SourceID. A single AllocItem may fan out into several Allocations once a multi-source
// allocator splits a line across warehouses; V1 always returns exactly one per item.
type Allocation struct {
	VariantID uuid.UUID
	SourceID  uuid.UUID
	Qty       int
}

// StockProvider reports available stock (physical minus reserved) for a (variant, source) pair.
// It is the read side a smarter allocator consults to decide splits; the SingleSourceAllocator
// does not need it, but the port exists so a MultiSourceAllocator is a drop-in.
type StockProvider interface {
	GetStock(ctx context.Context, variantID, sourceID uuid.UUID) (int, error)
}

// Allocator maps requested items onto sources. Implementations must return, per item, a set of
// Allocations whose Qty sums to the requested Qty (V1: a single Allocation from the default source).
type Allocator interface {
	Allocate(ctx context.Context, items []AllocItem) ([]Allocation, error)
}

// SingleSourceAllocator sends everything to the default source — the transparent V1 behavior.
type SingleSourceAllocator struct {
	defaultSourceID uuid.UUID
}

// NewSingleSourceAllocator builds the default allocator around a resolved default source id.
func NewSingleSourceAllocator(defaultSourceID uuid.UUID) *SingleSourceAllocator {
	return &SingleSourceAllocator{defaultSourceID: defaultSourceID}
}

// Allocate assigns each item's full quantity to the default source. It never consults stock:
// the atomic Reserve on the resulting (variant, source) row is what actually guards availability,
// so the allocator stays a pure routing decision and overselling is still impossible.
func (a *SingleSourceAllocator) Allocate(_ context.Context, items []AllocItem) ([]Allocation, error) {
	allocations := make([]Allocation, 0, len(items))
	for _, it := range items {
		allocations = append(allocations, Allocation{
			VariantID: it.VariantID,
			SourceID:  a.defaultSourceID,
			Qty:       it.Qty,
		})
	}
	return allocations, nil
}

// querier is the subset of pgx the read-only stock provider needs. Both *pgxpool.Pool and pgx.Tx
// satisfy it.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// postgresStockProvider reads availability from variant_stock.
type postgresStockProvider struct {
	db querier
}

// NewPostgresStockProvider builds the variant_stock-backed StockProvider.
func NewPostgresStockProvider(db *pgxpool.Pool) StockProvider {
	return &postgresStockProvider{db: db}
}

// GetStock returns available (stock - stock_reserved) for the pair, 0 if the pair has no row.
func (p *postgresStockProvider) GetStock(ctx context.Context, variantID, sourceID uuid.UUID) (int, error) {
	var available int
	err := p.db.QueryRow(ctx,
		`SELECT COALESCE(stock - stock_reserved, 0) FROM variant_stock WHERE variant_id = $1 AND source_id = $2`,
		variantID, sourceID,
	).Scan(&available)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return available, nil
}

// ErrNoDefaultSource means the sources table has no is_default row — the 000020 seed is missing.
var (
	ErrNoDefaultSource = errors.New("no default source configured")
	// ErrSourceNotFound is returned by GetByID when the id matches no source row — the caller
	// (admin stock write) maps it to 404 rather than creating a source implicitly.
	ErrSourceNotFound = errors.New("source not found")
)

// SourceRepository resolves stock locations. V1 needs the default source (to build the
// SingleSourceAllocator at wiring time) and GetByID to validate an explicit source on a
// per-source stock write.
type SourceRepository interface {
	GetDefault(ctx context.Context) (*models.Source, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Source, error)
}

type postgresSourceRepository struct {
	db querier
}

// NewPostgresSourceRepository builds the sources-table-backed repository.
func NewPostgresSourceRepository(db *pgxpool.Pool) SourceRepository {
	return &postgresSourceRepository{db: db}
}

// GetDefault returns the single is_default source (guaranteed unique by the partial index).
func (r *postgresSourceRepository) GetDefault(ctx context.Context) (*models.Source, error) {
	s := &models.Source{}
	err := r.db.QueryRow(ctx,
		`SELECT id, code, name, is_default, created_at, updated_at FROM sources WHERE is_default = TRUE`,
	).Scan(&s.ID, &s.Code, &s.Name, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoDefaultSource
		}
		return nil, err
	}
	return s, nil
}

// GetByID resolves a source by id, returning ErrSourceNotFound when it does not exist.
func (r *postgresSourceRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Source, error) {
	s := &models.Source{}
	err := r.db.QueryRow(ctx,
		`SELECT id, code, name, is_default, created_at, updated_at FROM sources WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Code, &s.Name, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSourceNotFound
		}
		return nil, err
	}
	return s, nil
}
