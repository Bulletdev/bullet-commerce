-- Restore the pre-refund payment_status CHECK. Any rows already in a refund state
-- must be reconciled before rolling back, or this constraint add will fail.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_payment_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_payment_status_check
  CHECK (payment_status IN ('unpaid', 'pending_payment', 'paid', 'failed'));

ALTER TABLE orders
  DROP COLUMN IF EXISTS refunded_at,
  DROP COLUMN IF EXISTS refund_amount_cents;
