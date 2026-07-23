package models

import (
	"time"

	"github.com/google/uuid"
)

// DefaultDeliveryCode is the code of the transparent single-shipment delivery that every
// cart and order gets automatically. It keeps the one-delivery case invisible to clients,
// mirroring the default-variant pattern.
const DefaultDeliveryCode = "default"

// Delivery is one fulfillment leg of a cart or order. A cart/order can carry N deliveries
// (ship-to-address today, store pickup / pickup point later); the default delivery makes the
// single-shipment case transparent. Exactly one of CartID/OrderID is set over its lifetime:
// the cart's delivery is mirrored onto the order at checkout. Freight belongs to the delivery
// that incurs it, not the order, so the order total stays items - discount + freight.
type Delivery struct {
	ID      uuid.UUID  `json:"id" db:"id"`
	CartID  *uuid.UUID `json:"cart_id,omitempty" db:"cart_id"`
	OrderID *uuid.UUID `json:"order_id,omitempty" db:"order_id"`
	Code    string     `json:"code" db:"code"`
	Method  *string    `json:"method,omitempty" db:"method"`
	Carrier *string    `json:"carrier,omitempty" db:"carrier"`
	// LocationType is one of address|store|pickup_point.
	LocationType      string     `json:"location_type" db:"location_type"`
	AddressID         *uuid.UUID `json:"address_id,omitempty" db:"address_id"`
	ShippingCostCents int64      `json:"shipping_cost_cents" db:"shipping_cost_cents"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}
