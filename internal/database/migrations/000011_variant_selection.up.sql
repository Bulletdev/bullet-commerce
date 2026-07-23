-- Variant selection at the cart/order boundary + independent shipping pricing.
-- The sellable unit is the VARIANT, so cart/order line identity moves from product to
-- variant: the same product in two variants (M vs G) are distinct lines.

-- cart_items: add variant_id, backfill to each product's default variant, then make it
-- required and move the uniqueness/upsert key from (cart_id, product_id) to (cart_id, variant_id).
ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS variant_id UUID REFERENCES product_variants(id);

UPDATE cart_items ci
SET variant_id = v.id
FROM product_variants v
WHERE v.product_id = ci.product_id
  AND v.sku = 'default-' || ci.product_id
  AND ci.variant_id IS NULL;

ALTER TABLE cart_items ALTER COLUMN variant_id SET NOT NULL;

ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS unique_cart_product;
ALTER TABLE cart_items ADD CONSTRAINT unique_cart_variant UNIQUE (cart_id, variant_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_variant_id ON cart_items(variant_id);

-- order_items: record which variant was actually sold (stock invariant lives on the variant).
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS variant_id UUID REFERENCES product_variants(id);

UPDATE order_items oi
SET variant_id = v.id
FROM product_variants v
WHERE v.product_id = oi.product_id
  AND v.sku = 'default-' || oi.product_id
  AND oi.variant_id IS NULL;

ALTER TABLE order_items ALTER COLUMN variant_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_order_items_variant_id ON order_items(variant_id);

-- orders: shipping is priced independently of the item subtotal. total_cents becomes
-- subtotal(items) + shipping_cost_cents.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_cost_cents BIGINT NOT NULL DEFAULT 0
    CHECK (shipping_cost_cents >= 0);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_method TEXT;
