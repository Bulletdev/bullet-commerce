package models

import (
	"time"

	"github.com/google/uuid"
)

type CartItem struct {
	ID        uuid.UUID `json:"id" db:"id"`
	CartID    uuid.UUID `json:"cart_id" db:"cart_id"`
	ProductID uuid.UUID `json:"product_id" db:"product_id"`
	// VariantID is the sellable unit this line refers to. The stock invariant lives on
	// the variant, so the cart line identity is the variant, not the product.
	VariantID uuid.UUID `json:"variant_id" db:"variant_id"`
	// DeliveryID is the fulfillment leg this line ships on. It is set transparently to the
	// cart's default delivery unless the client chooses another (multi-delivery, later).
	DeliveryID uuid.UUID `json:"delivery_id" db:"delivery_id"`
	Quantity   int       `json:"quantity" db:"quantity"`
	// PriceCents is the unit price snapshot (minor units) at the moment the item was added.
	PriceCents int64     `json:"price_cents" db:"price_cents"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}
