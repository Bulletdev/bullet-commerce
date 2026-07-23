package reviews

import (
	"time"

	"github.com/google/uuid"
)

// Review moderation lifecycle. Only ReviewStatusApproved reviews are shown publicly and
// counted in the product's denormalized rating aggregate (products.rating_avg / rating_count).
const (
	ReviewStatusPending  = "pending"
	ReviewStatusApproved = "approved"
	ReviewStatusRejected = "rejected"
)

// Review mirrors a row of product_reviews (migration 000028). Title/Body are nullable so a
// star-only rating is a valid review.
type Review struct {
	ID        uuid.UUID `json:"id" db:"id"`
	ProductID uuid.UUID `json:"product_id" db:"product_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	Rating    int       `json:"rating" db:"rating"`
	Title     *string   `json:"title,omitempty" db:"title"`
	Body      *string   `json:"body,omitempty" db:"body"`
	Status    string    `json:"status" db:"status"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
