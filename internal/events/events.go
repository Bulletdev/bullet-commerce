package events

import "github.com/google/uuid"

// Event is any domain fact that already happened. WHY past-tense names: the bus
// is fired by the caller AFTER a transaction commits, so an event always
// represents a durable, non-reversible fact - never a request to do something.
type Event interface { // NOSONAR domain event marker, not a behavioral interface
	Name() string
}

// OrderPlacedEvent is emitted once an order row is durably committed.
type OrderPlacedEvent struct {
	OrderID uuid.UUID
}

func (OrderPlacedEvent) Name() string { return "order.placed" }

// PaymentConfirmedEvent is emitted once a charge is confirmed and persisted.
// ChargeRef is the PSP-side reference so handlers avoid a DB round-trip.
type PaymentConfirmedEvent struct {
	OrderID   uuid.UUID
	ChargeRef string
}

func (PaymentConfirmedEvent) Name() string { return "payment.confirmed" }

// AddToCartEvent is emitted after a cart item is committed.
type AddToCartEvent struct {
	CartID    uuid.UUID
	VariantID uuid.UUID
	Qty       int
}

func (AddToCartEvent) Name() string { return "cart.item_added" }
