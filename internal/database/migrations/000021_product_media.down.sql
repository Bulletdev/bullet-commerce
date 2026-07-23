DROP TRIGGER IF EXISTS update_product_media_updated_at ON product_media;
DROP INDEX IF EXISTS idx_product_media_product_variant;
DROP INDEX IF EXISTS idx_product_media_product_id;
DROP TABLE IF EXISTS product_media;
