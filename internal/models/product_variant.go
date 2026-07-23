package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ProductVariant is an entity inside the Product aggregate. The stock invariant
// (available = stock - reserved, never negative) lives here, not on Product: a product
// is only ever sellable through one of its variants.
type ProductVariant struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	ProductID  uuid.UUID       `json:"product_id" db:"product_id"`
	SKU        string          `json:"sku" db:"sku"`
	Attributes json.RawMessage `json:"attributes" db:"attributes"`
	// PriceCents is materialized (NOT NULL since migration 000025): every variant carries an
	// explicit price, ending the `variant.price ?? product.price` fallback in consumers.
	PriceCents int64 `json:"price_cents" db:"price_cents"`
	// PriceInherited signals the price was copied from the parent product (not admin-set), so a
	// future product-price change fans out to it. The fan-out is a follow-up (PRD §4); this is
	// only the signal.
	PriceInherited bool   `json:"price_inherited" db:"price_inherited"`
	Currency       string `json:"currency" db:"currency"`
	// CompareAtPriceCents drives "de/por" display when > PriceCents. Nullable = no promo.
	CompareAtPriceCents *int64 `json:"compare_at_price_cents,omitempty" db:"compare_at_price_cents"`
	// Shipping overrides: when set they take precedence over the product's own dimensions for
	// freight; NULL means "inherit the product's".
	WeightGrams *int `json:"weight_grams,omitempty" db:"weight_grams"`
	LengthMM    *int `json:"length_mm,omitempty" db:"length_mm"`
	WidthMM     *int `json:"width_mm,omitempty" db:"width_mm"`
	HeightMM    *int `json:"height_mm,omitempty" db:"height_mm"`
	// Barcode is the GTIN/EAN (NF-e, marketplace, scanning). Nullable.
	Barcode *string `json:"barcode,omitempty" db:"barcode"`
	// Active hides/stops selling a variant without soft-deleting it.
	Active bool `json:"active" db:"active"`
	// Position is the display order among a product's variants.
	Position int `json:"position" db:"position"`
	// StockPolicy is 'deny' (never oversell) or 'backorder' (sell without stock). The reserve-path
	// enforcement of 'backorder' is a documented follow-up; this only carries the recorded policy.
	StockPolicy string `json:"stock_policy" db:"stock_policy"`
	// Stock/StockReserved are the DEPRECATED per-variant columns (real stock is per-source in
	// variant_stock). Kept on the read path until it is fully migrated.
	Stock         int `json:"stock" db:"stock"`
	StockReserved int `json:"stock_reserved" db:"stock_reserved"`
	// Available is DERIVED, not a stored column: summed from variant_stock (stock - stock_reserved
	// across every source) at read time by FindByID/FindByProductID - the same formula as
	// AvailableForVariant. It is zero on the write path (Create), where no read has populated it.
	Available int        `json:"stock_available" db:"-"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}
