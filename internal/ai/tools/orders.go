package tools

import (
	"context"
	"encoding/json"

	"bullet-commerce/internal/ai"
	"bullet-commerce/internal/models"

	"github.com/google/uuid"
)

// OrderReader is the narrow slice of internal/orders the status tool needs.
// FindOrderByID returns any order by id; the OWNERSHIP CHECK is enforced in the
// handler against the JWT-derived user id - the tool never trusts a user id from
// the model, and never leaks the existence of another user's order.
type OrderReader interface {
	FindOrderByID(ctx context.Context, orderID uuid.UUID) (*models.Order, []models.OrderItem, error)
}

func getMyOrderStatusTool(o OrderReader) Handler {
	schema := ai.ToolSchema{
		Name:        "get_my_order_status",
		Description: "Consulta o status e o status de pagamento de um pedido DO PRÓPRIO usuário autenticado. Use antes de afirmar qualquer status de pedido ou pagamento.",
		Properties: map[string]any{
			"order_id": map[string]any{
				"type":        "string",
				"description": "UUID do pedido do usuário.",
			},
		},
		Required: []string{"order_id"},
	}

	return Handler{Schema: schema, Execute: func(ctx context.Context, input json.RawMessage) ai.ToolResult {
		// Identity comes from the context (JWT), never from the input. Absent id
		// means unauthenticated scope -> treat as not found (no data leak).
		userID, ok := ai.UserIDFrom(ctx)
		if !ok {
			return notFound()
		}

		var in struct {
			OrderID string `json:"order_id"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return errResult("input inválido para get_my_order_status")
		}
		orderID, err := uuid.Parse(in.OrderID)
		if err != nil {
			return errResult("order_id inválido")
		}

		order, _, err := o.FindOrderByID(ctx, orderID)
		// WHY collapse "not found" and "belongs to someone else" into one answer:
		// distinguishing them would leak whether another user's order exists (LGPD).
		if err != nil || order == nil || order.UserID != userID {
			return notFound()
		}

		return okJSON(map[string]any{
			"order_id":       order.ID.String(),
			"status":         string(order.Status),
			"payment_status": string(order.PaymentStatus),
		})
	}}
}

// notFound is the deliberately non-error "no such order for you" answer. It is
// is_error:false because it is a legitimate business result the model relays,
// not a tool malfunction - and it never confirms the order exists for anyone.
func notFound() ai.ToolResult {
	return ai.ToolResult{Content: `{"found":false,"message":"pedido não encontrado"}`}
}
