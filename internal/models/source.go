package models

import (
	"time"

	"github.com/google/uuid"
)

// DefaultSourceCode is the code of the transparent single stock location every install gets
// automatically. It keeps the single-warehouse case invisible to clients, mirroring the
// default-variant and default-delivery patterns.
const DefaultSourceCode = "default"

// Source is a named stock location (warehouse / store) a variant can be fulfilled from. Stock
// lives per (variant, source); a Source with IsDefault=true is the one the SingleSourceAllocator
// ships everything from, so with a single default source the sourcing layer is transparent.
type Source struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Code      string    `json:"code" db:"code"`
	Name      string    `json:"name" db:"name"`
	IsDefault bool      `json:"is_default" db:"is_default"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
