-- Flamingo model: an order total can be settled by several charges at once. Gift card
-- and loyalty are ChargeTypes (money moving against a balance), NOT discounts, so each
-- lives as its own row and the order total is reconstructed by summing the charges.
CREATE TABLE IF NOT EXISTS payment_charges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('main', 'giftcard', 'loyalty')),
    method TEXT,
    amount_cents BIGINT NOT NULL,
    reference TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Every "charges of an order" read filters by order_id.
CREATE INDEX IF NOT EXISTS idx_payment_charges_order_id ON payment_charges(order_id);
