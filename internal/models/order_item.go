package models

import (
	"time"

	"github.com/google/uuid"
)

type OrderItem struct {
	ID        uuid.UUID `json:"id" db:"id"`
	OrderID   uuid.UUID `json:"order_id" db:"order_id"`
	ProductID uuid.UUID `json:"product_id" db:"product_id"`
	// VariantID records which variant was sold; Reserve/Claim/Release act on it.
	VariantID uuid.UUID `json:"variant_id" db:"variant_id"`
	// DeliveryID is the order's fulfillment leg this line shipped on, mirrored from the
	// cart's delivery at checkout.
	DeliveryID uuid.UUID `json:"delivery_id" db:"delivery_id"`
	// SourceID is the stock location this line was sourced from (chosen by the Allocator at
	// checkout). Release/Claim free the exact (variant, source) that was reserved.
	SourceID uuid.UUID `json:"source_id" db:"source_id"`
	Quantity int       `json:"quantity" db:"quantity"`
	// PriceCents is the unit price snapshot (minor units) at the moment the order was placed.
	PriceCents int64 `json:"price_cents" db:"price_cents"`
	// ProductName and VariantSKU are frozen at purchase time so a later catalog rename,
	// SKU change, or soft-delete never rewrites what this historical line shows.
	ProductName string    `json:"product_name" db:"product_name"`
	VariantSKU  string    `json:"variant_sku" db:"variant_sku"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
