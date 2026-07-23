package models

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string
type PaymentStatus string
type PaymentMethod string

const (
	StatusPending    OrderStatus = "pending"
	StatusProcessing OrderStatus = "processing"
	StatusShipped    OrderStatus = "shipped"
	StatusDelivered  OrderStatus = "delivered"
	StatusCancelled  OrderStatus = "cancelled"
)

const (
	PaymentUnpaid  PaymentStatus = "unpaid"
	PaymentPending PaymentStatus = "pending_payment"
	PaymentPaid    PaymentStatus = "paid"
	PaymentFailed  PaymentStatus = "failed"
	// PaymentRefunded / PaymentPartiallyRefunded record a settled order that was
	// (fully or partially) refunded. See migration 000030_order_refund.
	PaymentRefunded          PaymentStatus = "refunded"
	PaymentPartiallyRefunded PaymentStatus = "partially_refunded"
)

const (
	MethodPIX        PaymentMethod = "pix"
	MethodCreditCard PaymentMethod = "credit_card"
	MethodBoleto     PaymentMethod = "boleto"
)

// validTransitions defines the allowed order status state machine.
// Any transition not listed here is forbidden.
var validTransitions = map[OrderStatus][]OrderStatus{
	StatusPending:    {StatusProcessing, StatusCancelled},
	StatusProcessing: {StatusShipped, StatusCancelled},
	StatusShipped:    {StatusDelivered},
	StatusDelivered:  {},
	StatusCancelled:  {},
}

func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == next {
			return true
		}
	}
	return false
}

type Order struct {
	ID                uuid.UUID      `json:"id" db:"id"`
	UserID            uuid.UUID      `json:"user_id" db:"user_id"`
	ShippingAddressID uuid.UUID      `json:"shipping_address_id" db:"shipping_address_id"`
	Status            OrderStatus    `json:"status" db:"status"`
	PaymentStatus     PaymentStatus  `json:"payment_status" db:"payment_status"`
	PaymentMethod     *PaymentMethod `json:"payment_method,omitempty" db:"payment_method"`
	PaymentReference  *string        `json:"payment_reference,omitempty" db:"payment_reference"`
	// TotalCents is the amount charged to the customer. WHY subtotal + shipping: items
	// and freight are priced independently, so the order total = sum(item price*qty) +
	// ShippingCostCents. Keeping them separate lets the UI and refunds reason about each.
	TotalCents        int64   `json:"total_cents" db:"total_cents"`
	ShippingCostCents int64   `json:"shipping_cost_cents" db:"shipping_cost_cents"`
	ShippingMethod    *string `json:"shipping_method,omitempty" db:"shipping_method"`
	Currency          string  `json:"currency" db:"currency"`
	TrackingNumber    *string `json:"tracking_number,omitempty" db:"tracking_number"`
	// RefundedAt / RefundAmountCents track refunds (migration 000030). RefundAmountCents
	// accumulates across partial refunds; RefundedAt stamps the first refund.
	RefundedAt        *time.Time `json:"refunded_at,omitempty" db:"refunded_at"`
	RefundAmountCents int64      `json:"refund_amount_cents" db:"refund_amount_cents"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}
