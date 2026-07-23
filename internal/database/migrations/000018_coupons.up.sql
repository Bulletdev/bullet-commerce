-- Coupons are the RULE behind a cart-level promotion (the applied_discounts row in
-- 000015 is the computed RESULT). WHY a self-contained table + a column on carts:
-- a coupon is validated and priced by the promotions port at request time, and the
-- code the shopper attached lives on their cart so GetCart can re-price it and the
-- order can freeze it at checkout.
CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL,
    -- value carries two meanings by discount_type: a percentage in 0..100 when
    -- 'percent', or an absolute amount in minor units (cents) when 'fixed'.
    discount_type TEXT NOT NULL CHECK (discount_type IN ('percent', 'fixed')),
    value BIGINT NOT NULL,
    min_cart_cents BIGINT NOT NULL DEFAULT 0,
    max_uses INT,
    used_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- FindByCode is the only lookup path; UNIQUE already indexes code, so no extra index.

-- The codes a shopper attached to their cart. TEXT[] (not a join table): the set is
-- small, read on every cart fetch, and re-validated/re-priced by the promotions port,
-- so storing it inline keeps GetCart a single row read.
ALTER TABLE carts
    ADD COLUMN IF NOT EXISTS applied_coupon_codes TEXT[] NOT NULL DEFAULT '{}';
