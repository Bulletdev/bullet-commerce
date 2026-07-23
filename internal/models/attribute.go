package models

import (
	"time"

	"github.com/google/uuid"
)

// Attribute kinds. WHY they exist as constants: the selection UI branches on kind — 'color'
// renders a swatch from AttributeValue.Hex, 'select' an ordered list, 'text' a free entry.
const (
	AttributeKindSelect = "select"
	AttributeKindColor  = "color"
	AttributeKindText   = "text"
)

// Attribute is a normalized VARIATION key (e.g. code='tamanho', code='cor'). Only the keys that
// drive variant selection — those listed in products.variant_variation_attributes — are promoted
// here; free-form product metadata stays in the ProductVariant.Attributes JSONB. Code is what
// products.variant_variation_attributes references, turning a loose string into a validated key.
type Attribute struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Label     string    `json:"label" db:"label"`
	Kind      string    `json:"kind" db:"kind"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// AttributeValue is one selectable value of an Attribute (M/G/GG; preto/branco). Hex is the
// swatch color for kind='color' values and is nil until an admin sets it (the backfill never
// infers it). Position gives an explicit display order so 'M' can precede 'G'.
type AttributeValue struct {
	ID          uuid.UUID `json:"id" db:"id"`
	AttributeID uuid.UUID `json:"attribute_id" db:"attribute_id"`
	Value       string    `json:"value" db:"value"`
	Label       string    `json:"label" db:"label"`
	Hex         *string   `json:"hex,omitempty" db:"hex"`
	Position    int       `json:"position" db:"position"`
}

// AttributeFacet is one facet bucket: an attribute value plus how many variants of a product
// carry it. It carries the parent attribute code and the value's display fields so /search can
// render a faceted filter (e.g. "Cor: preto (3), branco (2)") without a second lookup.
type AttributeFacet struct {
	AttributeValueID uuid.UUID `json:"attribute_value_id" db:"attribute_value_id"`
	AttributeCode    string    `json:"attribute_code" db:"attribute_code"`
	Value            string    `json:"value" db:"value"`
	Label            string    `json:"label" db:"label"`
	Hex              *string   `json:"hex,omitempty" db:"hex"`
	Position         int       `json:"position" db:"position"`
	Count            int       `json:"count" db:"count"`
}
