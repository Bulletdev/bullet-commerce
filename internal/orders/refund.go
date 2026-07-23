package orders

import (
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/payment"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Refund sentinels - handlers map these to 4xx/5xx.
var (
	ErrOrderNotRefundable      = errors.New("order cannot be refunded in its current payment status")
	ErrRefundNotSupported      = errors.New("payment provider does not support refunds")
	ErrRefundAmountInvalid     = errors.New("refund amount exceeds the refundable balance")
	ErrRefundItemNotFound      = errors.New("refund item variant is not part of the order")
	ErrMissingPaymentReference = errors.New("order has no payment reference to refund against")
)

// RefundItem targets one order line for refund. Restock is opt-in per line: only flagged
// lines return physical stock (inverse of the Claim done at payment confirmation).
type RefundItem struct {
	VariantID uuid.UUID
	Qty       int
	Restock   bool
}

// OrderRefundedEvent is emitted once a refund is durably committed. Full reports whether the
// order's whole balance was refunded (vs a partial refund).
type OrderRefundedEvent struct {
	OrderID     uuid.UUID
	AmountCents int64
	Full        bool
}

func (OrderRefundedEvent) Name() string { return "order.refunded" }

// RefundOrder refunds a Paid order. In ONE transaction it (1) calls the financial Refunder
// on the PSP, (2) flips payment_status to refunded/partially_refunded and stamps
// refunded_at + refund_amount_cents, (3) marks the main charge refunded, and (4) for each
// item flagged restock returns physical stock at the (variant, source) it was Claimed from -
// the inverse of ConfirmOrderPayment's Claim. WHY one tx: money-state and stock-state must
// never diverge, exactly as ConfirmOrderPayment couples the claim and the charge flip.
//
// Guard: only a Paid order can be refunded (a single refund op per order; a partial refund
// leaves the order 'partially_refunded' and is not refundable again by this method). The
// row is locked FOR UPDATE so a concurrent refund/confirm cannot race the guard.
func (r *postgresOrderRepository) RefundOrder(ctx context.Context, refunder payment.Refunder, orderID uuid.UUID, items []RefundItem, amountCents int64) error {
	if refunder == nil {
		return ErrRefundNotSupported
	}
	if amountCents < 0 {
		return ErrRefundAmountInvalid
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	amountCents, full, paymentRef, err := r.resolveRefund(ctx, tx, orderID, amountCents)
	if err != nil {
		return err
	}

	// Call the PSP refund first: if the money movement fails, the tx rolls back and neither
	// the status flip nor the restock is applied.
	if _, err := refunder.Refund(ctx, paymentRef, payment.Money(amountCents)); err != nil {
		return err
	}

	if err := r.applyRefundStatus(ctx, tx, orderID, amountCents, full); err != nil {
		return err
	}

	if err := r.restockRefundItems(ctx, tx, orderID, items); err != nil {
		return err
	}

	// Flip the main charge to refunded in the same tx (mirrors ConfirmOrderPayment). A missing
	// main charge is not fatal - older orders may have none.
	if _, err := tx.Exec(ctx, `
		UPDATE payment_charges SET status = 'refunded'
		WHERE order_id = $1 AND type = 'main'
	`, orderID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if r.bus != nil {
		r.bus.Publish(ctx, OrderRefundedEvent{OrderID: orderID, AmountCents: amountCents, Full: full})
	}
	return nil
}

// resolveRefund locks the order FOR UPDATE, enforces the refund guard (only a settled Paid
// order with a payment reference is refundable), and resolves the amount: a zero request
// means a full refund of the remaining balance. It returns the resolved amount, whether that
// clears the whole balance (full), and the PSP payment reference. The lock is what stops a
// concurrent refund/confirm from racing the guard.
func (r *postgresOrderRepository) resolveRefund(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, amountCents int64) (int64, bool, string, error) {
	var paymentStatus models.PaymentStatus
	var paymentRef *string
	var totalCents, alreadyRefunded int64
	err := tx.QueryRow(ctx, `
		SELECT payment_status, payment_reference, total_cents, refund_amount_cents
		FROM orders
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, orderID).Scan(&paymentStatus, &paymentRef, &totalCents, &alreadyRefunded)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, "", ErrOrderNotFound
		}
		return 0, false, "", err
	}

	// Only a settled (Paid) order can be refunded.
	if paymentStatus != models.PaymentPaid {
		return 0, false, "", ErrOrderNotRefundable
	}
	if paymentRef == nil || *paymentRef == "" {
		return 0, false, "", ErrMissingPaymentReference
	}

	// Resolve the amount: 0 means a full refund of the remaining balance.
	refundable := totalCents - alreadyRefunded
	if amountCents == 0 {
		amountCents = refundable
	}
	if amountCents <= 0 || amountCents > refundable {
		return 0, false, "", ErrRefundAmountInvalid
	}
	full := amountCents >= refundable
	return amountCents, full, *paymentRef, nil
}

// applyRefundStatus flips payment_status to refunded (full) or partially_refunded and stamps
// refunded_at plus the accumulated refund_amount_cents.
func (r *postgresOrderRepository) applyRefundStatus(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, amountCents int64, full bool) error {
	newStatus := models.PaymentPartiallyRefunded
	if full {
		newStatus = models.PaymentRefunded
	}
	_, err := tx.Exec(ctx, `
		UPDATE orders
		SET payment_status = $1, refunded_at = NOW(), refund_amount_cents = refund_amount_cents + $2, updated_at = NOW()
		WHERE id = $3
	`, newStatus, amountCents, orderID)
	return err
}

// restockRefundItems returns physical stock for each flagged line at the exact source it was
// claimed from. order_items.source_id (added in 000020) records that pair, so the return
// targets the same (variant, source).
func (r *postgresOrderRepository) restockRefundItems(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, items []RefundItem) error {
	for _, it := range items {
		if !it.Restock {
			continue
		}
		var sourceID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT source_id FROM order_items WHERE order_id = $1 AND variant_id = $2
		`, orderID, it.VariantID).Scan(&sourceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrRefundItemNotFound
			}
			return err
		}
		if err := r.variantRepo.Restock(ctx, tx, it.VariantID, sourceID, it.Qty); err != nil {
			return err
		}
	}
	return nil
}
