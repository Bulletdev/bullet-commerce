package models

import (
	"time"

	"github.com/google/uuid"
)

// ChargeType distinguishes how a slice of the order total is settled. WHY a type (not a
// discount): gift card and loyalty redemptions still MOVE money against a balance, so
// they are modeled as their own charges alongside the "main" payment rather than as a
// price reduction - the order total stays intact and is split across charges.
type ChargeType string

const (
	ChargeMain     ChargeType = "main"
	ChargeGiftCard ChargeType = "giftcard"
	ChargeLoyalty  ChargeType = "loyalty"
)

type Charge struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	OrderID     uuid.UUID  `json:"order_id" db:"order_id"`
	Type        ChargeType `json:"type" db:"type"`
	Method      string     `json:"method" db:"method"`
	AmountCents int64      `json:"amount_cents" db:"amount_cents"`
	Reference   string     `json:"reference" db:"reference"`
	Status      string     `json:"status" db:"status"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}
