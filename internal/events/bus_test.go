package events

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	bus := NewInProcessBus()

	var got Event
	bus.Subscribe(OrderPlacedEvent{}.Name(), func(_ context.Context, e Event) {
		got = e
	})

	want := OrderPlacedEvent{OrderID: uuid.New()}
	bus.Publish(context.Background(), want)

	if got != want {
		t.Fatalf("handler got %#v, want %#v", got, want)
	}
}

func TestPanicInHandlerIsIsolated(t *testing.T) {
	bus := NewInProcessBus()

	bus.Subscribe(PaymentConfirmedEvent{}.Name(), func(context.Context, Event) {
		panic("boom")
	})

	ran := false
	bus.Subscribe(PaymentConfirmedEvent{}.Name(), func(context.Context, Event) {
		ran = true
	})

	// Must not propagate the panic out of Publish.
	bus.Publish(context.Background(), PaymentConfirmedEvent{OrderID: uuid.New(), ChargeRef: "ch_1"})

	if !ran {
		t.Fatal("second handler did not run after the first handler panicked")
	}
}

func TestPublishWithoutSubscriberIsNoOp(t *testing.T) {
	bus := NewInProcessBus()
	// Should neither panic nor block.
	bus.Publish(context.Background(), AddToCartEvent{CartID: uuid.New(), VariantID: uuid.New(), Qty: 2})
}
