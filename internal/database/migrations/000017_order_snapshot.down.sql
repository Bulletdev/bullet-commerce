-- Reverses 000017: drops the frozen snapshot columns from order_items.
ALTER TABLE order_items DROP COLUMN IF EXISTS variant_sku;
ALTER TABLE order_items DROP COLUMN IF EXISTS product_name;
