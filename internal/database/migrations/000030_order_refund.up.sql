-- Order-level refund tracking. refund_amount_cents accumulates across partial
-- refunds; refunded_at stamps the first (or full) refund. A full refund flips
-- payment_status to 'refunded', a partial to 'partially_refunded'.
ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS refunded_at         TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS refund_amount_cents BIGINT NOT NULL DEFAULT 0;

-- Extend the payment_status CHECK (added in 000008 as the auto-named column
-- constraint orders_payment_status_check) to admit the two refund states.
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_payment_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_payment_status_check
  CHECK (payment_status IN ('unpaid', 'pending_payment', 'paid', 'failed', 'refunded', 'partially_refunded'));

-- NOTE: order_items.source_id already exists (added NOT NULL in 000020_sources),
-- so restock has the exact (variant, source) that Claim consumed - no column added here.
