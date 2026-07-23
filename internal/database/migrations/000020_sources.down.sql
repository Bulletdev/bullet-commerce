ALTER TABLE order_items DROP COLUMN IF EXISTS source_id;
DROP TABLE IF EXISTS variant_stock;
DROP INDEX IF EXISTS idx_sources_single_default;
DROP TABLE IF EXISTS sources;
