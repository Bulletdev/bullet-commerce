package search

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBPool is the subset of pgxpool.Pool the adapter needs; narrowing it lets tests inject
// a pgxmock pool (same interface products/cart use).
type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type postgresService struct {
	db DBPool
}

func NewPostgresService(db *pgxpool.Pool) Service {
	return &postgresService{db: db}
}

// priceBuckets defines the fixed ranges for the price facet. WHY fixed: a range facet
// needs stable, comparable buckets across queries; deriving them per-query would make the
// UI jump around as the result set changes.
const priceBucketExpr = `CASE
		WHEN price_cents < 5000 THEN '0-5000'
		WHEN price_cents < 10000 THEN '5000-10000'
		WHEN price_cents < 20000 THEN '10000-20000'
		ELSE '20000+'
	END`

func (s *postgresService) Search(ctx context.Context, filters ...Filter) (Result, error) {
	qb := newQueryBuilder()
	for _, f := range filters {
		f.Apply(qb)
	}

	where := qb.whereClause()
	args := qb.args

	// Total count drives paging and the empty short-circuit; it ignores LIMIT/OFFSET.
	var num int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM products "+where, args...).Scan(&num); err != nil {
		return Result{}, err
	}

	result := Result{NumResults: num}
	if num == 0 {
		// No matches: skip the id page and facet aggregation entirely.
		return result, nil
	}

	ids, err := s.pageIDs(ctx, qb, where, args)
	if err != nil {
		return Result{}, err
	}
	result.ProductIDs = ids

	facets, err := s.facets(ctx, qb, where, args)
	if err != nil {
		return Result{}, err
	}
	result.Facets = facets

	result.NumPages = (num + qb.limit - 1) / qb.limit
	return result, nil
}

// pageIDs fetches just the id column for the requested window, ordered by relevance when
// searching (ts_rank) or by the explicit/whitelisted sort otherwise.
func (s *postgresService) pageIDs(ctx context.Context, qb *QueryBuilder, where string, args []any) ([]uuid.UUID, error) {
	limitPh := "$" + strconv.Itoa(len(args)+1)
	offsetPh := "$" + strconv.Itoa(len(args)+2)

	sql := "SELECT id FROM products " + where +
		" ORDER BY " + qb.orderClause() +
		" LIMIT " + limitPh + " OFFSET " + offsetPh

	pageArgs := append(append([]any{}, args...), qb.limit, qb.offset)
	rows, err := s.db.Query(ctx, sql, pageArgs...)
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

// facets aggregates the same filtered set (WHY same set, not per-facet drill-down: keeps
// counts consistent with the visible results and the query count small).
func (s *postgresService) facets(ctx context.Context, qb *QueryBuilder, where string, args []any) ([]Facet, error) {
	category, err := s.listFacet(ctx,
		"SELECT category_id::text, COUNT(*) FROM products "+where+
			" AND category_id IS NOT NULL GROUP BY category_id ORDER BY COUNT(*) DESC",
		args, "category_id", qb.applied["category_id"])
	if err != nil {
		return nil, err
	}

	price, err := s.listFacet(ctx,
		"SELECT "+priceBucketExpr+" AS bucket, COUNT(*) FROM products "+where+
			" GROUP BY bucket ORDER BY bucket",
		args, "price", qb.applied["price"])
	if err != nil {
		return nil, err
	}
	price.Kind = "range"

	return []Facet{category, price}, nil
}

func (s *postgresService) listFacet(ctx context.Context, sql string, args []any, field, selected string) (Facet, error) {
	facet := Facet{Field: field, Kind: "list"}
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return facet, err
	}
	defer rows.Close()

	for rows.Next() {
		var value string
		var count int
		if err := rows.Scan(&value, &count); err != nil {
			return facet, err
		}
		facet.Items = append(facet.Items, FacetItem{
			Value:    value,
			Count:    count,
			Selected: value == selected && selected != "",
		})
	}
	return facet, rows.Err()
}

// whereClause always anchors on the soft-delete guard AND the published-status guard so
// every query path (REST search and the AI search_catalog tool) excludes deleted rows and
// non-active (draft/archived) products, then appends the filter conditions. There is no admin
// search path today; if one is added, gate the status clause behind a Filter flag rather than
// dropping it here.
func (qb *QueryBuilder) whereClause() string {
	parts := append([]string{"deleted_at IS NULL", "status = 'active'"}, qb.conditions...)
	return "WHERE " + strings.Join(parts, " AND ")
}

// orderClause prefers an explicit SortFilter, then relevance (ts_rank) when a text query
// is present, and finally newest-first.
func (qb *QueryBuilder) orderClause() string {
	if qb.orderBy != "" {
		return qb.orderBy
	}
	if qb.rankExpr != "" {
		return qb.rankExpr + " DESC"
	}
	return "created_at DESC"
}
