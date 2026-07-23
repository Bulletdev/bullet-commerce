package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"bullet-commerce/internal/ai"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/search"
	"bullet-commerce/internal/variants"

	"github.com/google/uuid"
)

// --- fakes over the narrow repo interfaces ---

type fakeCatalog struct {
	result search.Result
	err    error
	got    []search.Filter
}

func (f *fakeCatalog) Search(_ context.Context, filters ...search.Filter) (search.Result, error) {
	f.got = filters
	return f.result, f.err
}

type fakeVariants struct {
	variant *models.ProductVariant
	err     error
}

func (f *fakeVariants) FindPublishedByID(_ context.Context, _ uuid.UUID) (*models.ProductVariant, error) {
	return f.variant, f.err
}

type fakeOrders struct {
	order *models.Order
	err   error
}

func (f *fakeOrders) FindOrderByID(_ context.Context, _ uuid.UUID) (*models.Order, []models.OrderItem, error) {
	return f.order, nil, f.err
}

func raw(m map[string]any) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

// Unknown tool must be rejected by the allowlist, never executed.
func TestRegistryRejectsUnknownTool(t *testing.T) {
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{}, &fakeOrders{})
	res := reg.Execute(context.Background(), "drop_table", raw(map[string]any{}))
	if !res.IsError {
		t.Fatal("expected unknown tool to be an is_error result")
	}
}

func TestRegistryAdvertisesAllowlistedTools(t *testing.T) {
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{}, &fakeOrders{})
	names := map[string]bool{}
	for _, s := range reg.Schemas() {
		names[s.Name] = true
	}
	for _, want := range []string{"search_catalog", "check_variant_stock", "get_my_order_status"} {
		if !names[want] {
			t.Fatalf("missing tool schema %q", want)
		}
	}
}

func TestSearchCatalog(t *testing.T) {
	id := uuid.New()
	reg := NewRegistry(&fakeCatalog{result: search.Result{NumResults: 1, ProductIDs: []uuid.UUID{id}}}, &fakeVariants{}, &fakeOrders{})
	res := reg.Execute(context.Background(), "search_catalog", raw(map[string]any{"query": "caneca azul"}))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, id.String()) {
		t.Fatalf("expected product id in result, got %s", res.Content)
	}
}

func TestCheckVariantStock(t *testing.T) {
	v := &models.ProductVariant{ID: uuid.New(), Available: 3}
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{variant: v}, &fakeOrders{})
	res := reg.Execute(context.Background(), "check_variant_stock", raw(map[string]any{"variant_id": v.ID.String()}))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// available = 5 - 2 = 3
	if !strings.Contains(res.Content, `"available":3`) || !strings.Contains(res.Content, `"in_stock":true`) {
		t.Fatalf("unexpected stock payload: %s", res.Content)
	}
}

func TestCheckVariantStockNotFound(t *testing.T) {
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{err: variants.ErrVariantNotFound}, &fakeOrders{})
	res := reg.Execute(context.Background(), "check_variant_stock", raw(map[string]any{"variant_id": uuid.New().String()}))
	if !res.IsError {
		t.Fatal("expected is_error for a missing variant")
	}
}

// The order tool must derive identity from context, never from input.
func TestGetMyOrderStatusOwner(t *testing.T) {
	userID := uuid.New()
	order := &models.Order{ID: uuid.New(), UserID: userID, Status: models.StatusShipped, PaymentStatus: models.PaymentPaid}
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{}, &fakeOrders{order: order})

	ctx := ai.WithUserID(context.Background(), userID)
	res := reg.Execute(ctx, "get_my_order_status", raw(map[string]any{"order_id": order.ID.String()}))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, string(models.StatusShipped)) || !strings.Contains(res.Content, string(models.PaymentPaid)) {
		t.Fatalf("expected owner to see status, got %s", res.Content)
	}
}

// USER ISOLATION: user A must NOT see user B's order — collapsed to "not found",
// leaking neither the status nor the existence of the order.
func TestGetMyOrderStatusUserIsolation(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	ordersOfB := &models.Order{ID: uuid.New(), UserID: userB, Status: models.StatusDelivered, PaymentStatus: models.PaymentPaid}
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{}, &fakeOrders{order: ordersOfB})

	ctx := ai.WithUserID(context.Background(), userA)
	res := reg.Execute(ctx, "get_my_order_status", raw(map[string]any{"order_id": ordersOfB.ID.String()}))

	if strings.Contains(res.Content, string(models.StatusDelivered)) {
		t.Fatalf("LEAK: user A saw user B's order status: %s", res.Content)
	}
	if !strings.Contains(res.Content, "não encontrado") {
		t.Fatalf("expected 'não encontrado', got %s", res.Content)
	}
}

// No authenticated user in context -> not found (no leak).
func TestGetMyOrderStatusNoUser(t *testing.T) {
	order := &models.Order{ID: uuid.New(), UserID: uuid.New(), Status: models.StatusPending}
	reg := NewRegistry(&fakeCatalog{}, &fakeVariants{}, &fakeOrders{order: order})
	res := reg.Execute(context.Background(), "get_my_order_status", raw(map[string]any{"order_id": order.ID.String()}))
	if !strings.Contains(res.Content, "não encontrado") {
		t.Fatalf("expected not-found without a user, got %s", res.Content)
	}
}

// The OrderReader fake must line up with the real repository's method signature
// so the narrow interface stays honest.
var _ OrderReader = (orders.OrderRepository)(nil)
