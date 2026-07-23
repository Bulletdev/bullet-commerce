package models

import (
	"time"

	"github.com/google/uuid"
)

type Address struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Street     string    `json:"street" db:"street"`
	City       string    `json:"city" db:"city"`
	State      string    `json:"state" db:"state"`
	PostalCode string    `json:"postal_code" db:"postal_code"`
	Country    string    `json:"country" db:"country"`
	// IsDefault is the legacy single-default flag, kept for backward compatibility.
	// Billing and shipping defaults are now tracked independently below.
	IsDefault bool `json:"is_default" db:"is_default"`
	// Billing and shipping defaults are separate: the address a customer bills to
	// is frequently not the one they ship to, so each has its own per-user default.
	IsDefaultBilling  bool      `json:"is_default_billing" db:"is_default_billing"`
	IsDefaultShipping bool      `json:"is_default_shipping" db:"is_default_shipping"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}
