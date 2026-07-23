-- Reverses 000019: drops the item->delivery wiring and the deliveries table.
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS fk_order_items_delivery;
ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS fk_cart_items_delivery;
ALTER TABLE order_items DROP COLUMN IF EXISTS delivery_id;
ALTER TABLE cart_items DROP COLUMN IF EXISTS delivery_id;
DROP TRIGGER IF EXISTS update_deliveries_updated_at ON deliveries;
DROP INDEX IF EXISTS idx_deliveries_order_code;
DROP INDEX IF EXISTS idx_deliveries_cart_code;
DROP TABLE IF EXISTS deliveries;
