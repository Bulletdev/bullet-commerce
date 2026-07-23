package models

import (
	"time"

	"github.com/google/uuid"
)

// Brand is a first-class catalog entity a product may reference via Product.BrandID.
// Slug/LogoURL are nullable so a brand can be created with just a name.
type Brand struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      *string   `json:"slug,omitempty" db:"slug"`
	LogoURL   *string   `json:"logo_url,omitempty" db:"logo_url"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
