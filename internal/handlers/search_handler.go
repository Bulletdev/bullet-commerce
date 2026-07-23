package handlers

import (
	"bullet-commerce/internal/search"
	"bullet-commerce/internal/webutils"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type SearchHandler struct {
	Service search.Service
}

func NewSearchHandler(service search.Service) *SearchHandler {
	return &SearchHandler{Service: service}
}

// Search handles GET /api/search, translating query-string params into the polymorphic
// Filter set the search Service composes. Route registration is intentionally left to
// main.go so this package stays decoupled from the router wiring.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var filters []search.Filter

	if text := strings.TrimSpace(q.Get("q")); text != "" {
		filters = append(filters, search.QueryFilter{Text: text})
	}

	if cat := q.Get("category_id"); cat != "" {
		// Reject a malformed id early rather than letting it surface as a DB error.
		if _, err := uuid.Parse(cat); err != nil {
			webutils.ErrorJSON(w, errors.New("invalid category_id format"), http.StatusBadRequest)
			return
		}
		filters = append(filters, search.KeyValueFilter{Field: "category_id", Value: cat})
	}

	if sort := q.Get("sort"); sort != "" {
		field, desc := parseSort(sort)
		filters = append(filters, search.SortFilter{Field: field, Desc: desc})
	}

	limit, offset := parsePagination(r)
	filters = append(filters, search.PaginationFilter{Limit: limit, Offset: offset})

	result, err := h.Service.Search(r.Context(), filters...)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to run search"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, result)
}

// parseSort reads "field" or "field:desc" / "field:asc" from the sort param.
func parseSort(raw string) (field string, desc bool) {
	parts := strings.SplitN(raw, ":", 2)
	field = parts[0]
	if len(parts) == 2 && strings.EqualFold(parts[1], "desc") {
		desc = true
	}
	return field, desc
}
