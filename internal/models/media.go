package models

import (
	"time"

	"github.com/google/uuid"
)

// Media kinds. The DB CHECK mirrors these, so validation lives in both places: the
// handler rejects unknown kinds early, the column keeps the invariant if a write
// ever bypasses the handler.
const (
	MediaKindImage = "image"
	MediaKindVideo = "video"
)

// ProductMedia is an image or video attached to a product or one of its variants.
// A NULL VariantID means the media belongs to the product as a whole; a set VariantID
// scopes it to a specific variant (e.g. the photo of the "blue" colorway). The file
// itself is never stored here — Url points at an external CDN/bucket object.
type ProductMedia struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	ProductID uuid.UUID  `json:"product_id" db:"product_id"`
	VariantID *uuid.UUID `json:"variant_id,omitempty" db:"variant_id"`
	URL       string     `json:"url" db:"url"`
	Alt       *string    `json:"alt,omitempty" db:"alt"`
	Kind      string     `json:"kind" db:"kind"`
	Position  int        `json:"position" db:"position"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// ValidMediaKind reports whether k is a kind the schema accepts. An empty kind is
// caller's cue to default to image (handled where the media is built), so it is not
// accepted here — this guards an explicitly supplied value.
func ValidMediaKind(k string) bool {
	return k == MediaKindImage || k == MediaKindVideo
}
