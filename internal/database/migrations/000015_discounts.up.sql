-- applied_discounts records the RESULT of a promotion decision, not the rule.
-- WHY a standalone table (no changes to cart/orders): the cart/order wiring is done
-- later; storing the computed reductions on their own keeps this domain shippable in
-- isolation and lets one order/cart carry several stacked discounts (one row each).
-- Exactly one of order_id / cart_id is expected to be set per row; both are NULL-able
-- so a reduction can be attached at either the cart or the order stage.
CREATE TABLE IF NOT EXISTS applied_discounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NULL REFERENCES orders(id) ON DELETE CASCADE,
    cart_id UUID NULL REFERENCES carts(id) ON DELETE CASCADE,
    level TEXT NOT NULL CHECK (level IN ('item', 'delivery', 'cart', 'shipping')),
    type TEXT NOT NULL CHECK (type IN ('percent', 'fixed')),
    -- applied_cents is always negative: a discount subtracts from the total.
    applied_cents BIGINT NOT NULL CHECK (applied_cents <= 0),
    is_item_related BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order INT NOT NULL DEFAULT 0,
    campaign_code TEXT NOT NULL DEFAULT '',
    coupon_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_applied_discounts_order_id ON applied_discounts(order_id);
CREATE INDEX IF NOT EXISTS idx_applied_discounts_cart_id ON applied_discounts(cart_id);
CREATE INDEX IF NOT EXISTS idx_applied_discounts_campaign_code ON applied_discounts(campaign_code);
