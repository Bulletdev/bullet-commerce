-- Reverses 000011: drops shipping columns and variant_id from cart/order items,
-- restoring the (cart_id, product_id) uniqueness on cart_items.
ALTER TABLE orders DROP COLUMN IF EXISTS shipping_method;
ALTER TABLE orders DROP COLUMN IF EXISTS shipping_cost_cents;

DROP INDEX IF EXISTS idx_order_items_variant_id;
ALTER TABLE order_items DROP COLUMN IF EXISTS variant_id;

DROP INDEX IF EXISTS idx_cart_items_variant_id;
ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS unique_cart_variant;
ALTER TABLE cart_items DROP COLUMN IF EXISTS variant_id;
ALTER TABLE cart_items ADD CONSTRAINT unique_cart_product UNIQUE (cart_id, product_id);
