package search

import (
	"context"
	"strconv"

	"github.com/google/uuid"
)

// QueryBuilder is the mutable accumulator that Filters compose into. Each Filter
// contributes WHERE conditions, args, sort or paging without knowing about the others,
// so the caller can pass any subset in any order and still get one coherent query.
type QueryBuilder struct {
	conditions []string
	args       []any

	// rankExpr is the ts_rank(...) expression when a text QueryFilter is present; it
	// doubles as the relevance default sort when no explicit SortFilter is given.
	rankExpr string

	// orderBy is set by a SortFilter (already whitelisted + direction-suffixed).
	orderBy string

	limit  int
	offset int

	// applied records exact-match filters so facets can flag which items are selected.
	applied map[string]string
}

func newQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		limit:   20,
		offset:  0,
		applied: map[string]string{},
	}
}

// AddArg appends a query argument and returns its positional placeholder ($1, $2, ...).
// Filters use it so placeholder numbering stays correct regardless of application order.
func (qb *QueryBuilder) AddArg(v any) string {
	qb.args = append(qb.args, v)
	return "$" + strconv.Itoa(len(qb.args))
}

// AddCondition appends a WHERE fragment (already using placeholders from AddArg).
func (qb *QueryBuilder) AddCondition(cond string) {
	qb.conditions = append(qb.conditions, cond)
}

// Filter is the polymorphic unit of a search: each implementation mutates the builder.
// Passing them variadically lets the query be assembled from an open set of concerns.
type Filter interface {
	Apply(*QueryBuilder)
}

// KeyValueFilter narrows results by an exact column match (e.g. category_id = <uuid>).
type KeyValueFilter struct {
	Field string
	Value string
}

func (f KeyValueFilter) Apply(qb *QueryBuilder) {
	// A UUID column won't compare against a text param ("operator does not exist:
	// uuid = text"), so parse when the value looks like a UUID and bind the typed value.
	var arg any = f.Value
	if id, err := uuid.Parse(f.Value); err == nil {
		arg = id
	}
	ph := qb.AddArg(arg)
	qb.AddCondition(f.Field + " = " + ph)
	qb.applied[f.Field] = f.Value
}

// QueryFilter is a full-text search term matched against the products.search_tsv column.
type QueryFilter struct {
	Text string
}

func (f QueryFilter) Apply(qb *QueryBuilder) {
	if f.Text == "" {
		return
	}
	ph := qb.AddArg(f.Text)
	qb.AddCondition("search_tsv @@ to_tsquery('portuguese', " + ph + ")")
	qb.rankExpr = "ts_rank(search_tsv, to_tsquery('portuguese', " + ph + "))"
}

// sortable whitelists user-facing sort keys to real columns; interpolating anything
// else into ORDER BY would be a SQL injection vector.
var sortable = map[string]string{
	"name":        "name",
	"price":       "price_cents",
	"price_cents": "price_cents",
	"created_at":  "created_at",
}

// SortFilter orders results by a whitelisted field; unknown fields are ignored so the
// adapter falls back to its default (relevance when searching, newest otherwise).
type SortFilter struct {
	Field string
	Desc  bool
}

func (f SortFilter) Apply(qb *QueryBuilder) {
	col, ok := sortable[f.Field]
	if !ok {
		return
	}
	dir := "ASC"
	if f.Desc {
		dir = "DESC"
	}
	qb.orderBy = col + " " + dir
}

// PaginationFilter sets the page window.
type PaginationFilter struct {
	Limit  int
	Offset int
}

func (f PaginationFilter) Apply(qb *QueryBuilder) {
	if f.Limit > 0 {
		qb.limit = f.Limit
	}
	if f.Offset >= 0 {
		qb.offset = f.Offset
	}
}

// FacetItem is one bucket of a facet with its match count and whether it's active.
type FacetItem struct {
	Value    string `json:"value"`
	Count    int    `json:"count"`
	Selected bool   `json:"selected"`
}

// Facet is an aggregated breakdown of the result set over one field.
type Facet struct {
	Field string      `json:"field"`
	Kind  string      `json:"kind"` // "list" | "range"
	Items []FacetItem `json:"items"`
}

// Result is the search response: the page of product IDs plus facets and paging totals.
type Result struct {
	ProductIDs []uuid.UUID `json:"product_ids"`
	Facets     []Facet     `json:"facets"`
	NumResults int         `json:"num_results"`
	NumPages   int         `json:"num_pages"`
}

// Service runs a composed search. Filters are variadic so callers assemble exactly the
// concerns they need.
type Service interface {
	Search(ctx context.Context, filters ...Filter) (Result, error)
}
