package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Product types drive how the product is composed and sold:
//   - simple: sold as a single unit (one implicit variant).
//   - configurable: sold through selectable variants (size/color); the keys that
//     drive the selection UI live in VariantVariationAttributes.
//   - bundle: sold as a set of BundleChoices the customer fills.
const (
	ProductTypeSimple       = "simple"
	ProductTypeConfigurable = "configurable"
	ProductTypeBundle       = "bundle"
)

type Product struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	PriceCents  int64      `json:"price_cents" db:"price_cents"`
	Currency    string     `json:"currency" db:"currency"`
	CategoryID  *uuid.UUID `json:"category_id" db:"category_id"`
	Stock       int        `json:"stock" db:"stock"`
	Featured    bool       `json:"featured" db:"featured"`
	// Type is one of ProductType*; it defaults to "simple" at the persistence layer.
	Type string `json:"type,omitempty" db:"type"`
	// Attributes holds product-level metadata (JSONB) that is not variant-specific.
	Attributes json.RawMessage `json:"attributes,omitempty" db:"attributes"`
	// VariantVariationAttributes lists which variant attribute keys generate the
	// selection UI (e.g. ["tamanho","cor"]) for a configurable product.
	VariantVariationAttributes []string `json:"variant_variation_attributes,omitempty" db:"variant_variation_attributes"`

	// Shipping dimensions (migration 000022). Nullable: freight uses them only when the
	// admin has filled them, so a product without dimensions still reads/writes cleanly.
	WeightGrams *int `json:"weight_grams,omitempty" db:"weight_grams"`
	LengthMM    *int `json:"length_mm,omitempty" db:"length_mm"`
	WidthMM     *int `json:"width_mm,omitempty" db:"width_mm"`
	HeightMM    *int `json:"height_mm,omitempty" db:"height_mm"`

	// Fiscal cadastro (migration 000023) - the subset the NF-e needs stored on the product.
	// NCM/CEST are nullable; Origem (mercadoria SEFAZ, 0..8) and Unit default at the DB.
	NCM    *string `json:"ncm,omitempty" db:"ncm"`
	CEST   *string `json:"cest,omitempty" db:"cest"`
	Origem int     `json:"origem" db:"origem"`
	Unit   string  `json:"unit" db:"unit"`

	// Merchandising & publication (migration 000024). Status is the publish lifecycle,
	// distinct from Featured and from DeletedAt; the public catalog shows only 'active'.
	Status              string     `json:"status" db:"status"`
	Slug                *string    `json:"slug,omitempty" db:"slug"`
	MetaTitle           *string    `json:"meta_title,omitempty" db:"meta_title"`
	MetaDescription     *string    `json:"meta_description,omitempty" db:"meta_description"`
	BrandID             *uuid.UUID `json:"brand_id,omitempty" db:"brand_id"`
	CompareAtPriceCents *int64     `json:"compare_at_price_cents,omitempty" db:"compare_at_price_cents"`

	// Version drives optimistic concurrency (migration 000027). The repository bumps it on
	// every update and guards the write with the caller's expected value; a mismatch means a
	// concurrent edit landed first (ErrProductVersionConflict / HTTP 409).
	Version int `json:"version" db:"version"`

	// Rating aggregates (columns via migration 000028, owned by the reviews agent). RatingAvg is
	// nullable - NULL until the product has at least one review; RatingCount reads COALESCE'd to 0.
	RatingAvg   *float64 `json:"rating_avg" db:"rating_avg"`
	RatingCount int      `json:"rating_count" db:"rating_count"`

	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// Product publish statuses. Only ProductStatusActive is shown in the public catalog.
const (
	ProductStatusDraft    = "draft"
	ProductStatusActive   = "active"
	ProductStatusArchived = "archived"
)
