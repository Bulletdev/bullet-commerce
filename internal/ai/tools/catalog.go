package tools

import (
	"context"
	"encoding/json"

	"bullet-commerce/internal/ai"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/search"

	"github.com/google/uuid"
)

// CatalogSearcher is the narrow slice of internal/search the catalog tool needs.
// search.Service already satisfies it; the interface keeps the dependency explicit
// and fakeable.
type CatalogSearcher interface {
	Search(ctx context.Context, filters ...search.Filter) (search.Result, error)
}

// VariantReader is the narrow slice of internal/variants the stock tool needs. It uses the
// published-safe read so the assistant never reports stock for an inactive variant or a
// draft/archived/deleted parent product.
type VariantReader interface {
	FindPublishedByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error)
}

// searchCatalogDefaultLimit / Max bound how many results the tool returns. WHY
// capped: the model only needs a handful of candidates to ground a reply, and a
// small page keeps token cost and latency down.
const (
	searchCatalogDefaultLimit = 5
	searchCatalogMaxLimit     = 20
)

func searchCatalogTool(s CatalogSearcher) Handler {
	schema := ai.ToolSchema{
		Name:        "search_catalog",
		Description: "Busca produtos no catálogo da loja por texto em português. Use para encontrar produtos antes de falar sobre eles. Retorna os IDs dos produtos encontrados e a contagem total.",
		Properties: map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Texto de busca em português (ex.: 'caneca de cerâmica azul').",
			},
			"category_id": map[string]any{
				"type":        "string",
				"description": "Filtro opcional por categoria (UUID).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Máximo de resultados (1 a 20). Padrão 5.",
			},
		},
		Required: []string{"query"},
	}

	return Handler{Schema: schema, Execute: func(ctx context.Context, input json.RawMessage) ai.ToolResult {
		var in struct {
			Query      string `json:"query"`
			CategoryID string `json:"category_id"`
			Limit      int    `json:"limit"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return errResult("input inválido para search_catalog")
		}
		if in.Query == "" {
			return errResult("o campo 'query' é obrigatório")
		}

		limit := in.Limit
		if limit <= 0 || limit > searchCatalogMaxLimit {
			limit = searchCatalogDefaultLimit
		}

		filters := []search.Filter{search.QueryFilter{Text: in.Query}}
		if in.CategoryID != "" {
			filters = append(filters, search.KeyValueFilter{Field: "category_id", Value: in.CategoryID})
		}
		filters = append(filters, search.PaginationFilter{Limit: limit, Offset: 0})

		res, err := s.Search(ctx, filters...)
		if err != nil {
			return errResult("erro ao buscar no catálogo")
		}

		// TODO(v2): enrich with product names/prices via the products repo so the
		// model has human-readable candidates. For now we return grounded IDs +
		// counts from the existing search service; prices/stock stay tool-sourced.
		ids := make([]string, 0, len(res.ProductIDs))
		for _, id := range res.ProductIDs {
			ids = append(ids, id.String())
		}
		return okJSON(map[string]any{
			"num_results": res.NumResults,
			"product_ids": ids,
		})
	}}
}

func checkVariantStockTool(v VariantReader) Handler {
	schema := ai.ToolSchema{
		Name:        "check_variant_stock",
		Description: "Consulta o estoque real de uma variante de produto. Use SEMPRE antes de afirmar disponibilidade. O estoque é volátil (reservas em andamento), então trate 'disponível' como 'confirmo no carrinho'.",
		Properties: map[string]any{
			"variant_id": map[string]any{
				"type":        "string",
				"description": "UUID da variante do produto.",
			},
		},
		Required: []string{"variant_id"},
	}

	return Handler{Schema: schema, Execute: func(ctx context.Context, input json.RawMessage) ai.ToolResult {
		var in struct {
			VariantID string `json:"variant_id"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return errResult("input inválido para check_variant_stock")
		}
		id, err := uuid.Parse(in.VariantID)
		if err != nil {
			return errResult("variant_id inválido")
		}

		variant, err := v.FindPublishedByID(ctx, id)
		if err != nil {
			// Includes variants.ErrVariantNotFound - an invalid reference OR a variant whose
			// parent product is unpublished/deleted; both read as "not found" so no stock leaks.
			return errResult("variante não encontrada")
		}

		// Available is the derived per-source sum populated by the variant read path.
		available := variant.Available
		return okJSON(map[string]any{
			"variant_id": variant.ID.String(),
			"available":  available,
			"in_stock":   available > 0,
		})
	}}
}
