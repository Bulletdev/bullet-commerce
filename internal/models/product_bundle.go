package models

import "github.com/google/uuid"

// A bundle product is composed of one or more BundleChoices. Each choice is a slot
// the customer fills (e.g. "pick a mug"), and each choice exposes a set of
// BundleOptions (the concrete products eligible for that slot). Choices/options are
// their own entities inside the Product aggregate rather than variants because a
// bundle references OTHER products, not size/color permutations of itself.

// BundleChoice constrains how a slot may be filled: Min/MaxQty bound the number of
// units the customer may pick, and Required marks a slot that cannot be left empty.
type BundleChoice struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ProductID uuid.UUID `json:"product_id" db:"product_id"`
	MinQty    int       `json:"min_qty" db:"min_qty"`
	MaxQty    int       `json:"max_qty" db:"max_qty"`
	Required  bool      `json:"required" db:"required"`
}

// BundleOption is one product a customer may select for a BundleChoice. DefaultQty
// is the quantity pre-selected when the bundle is first rendered.
type BundleOption struct {
	ID              uuid.UUID `json:"id" db:"id"`
	ChoiceID        uuid.UUID `json:"choice_id" db:"choice_id"`
	OptionProductID uuid.UUID `json:"option_product_id" db:"option_product_id"`
	DefaultQty      int       `json:"default_qty" db:"default_qty"`
}
