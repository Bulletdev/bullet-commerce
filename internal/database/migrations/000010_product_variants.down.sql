DROP TRIGGER IF EXISTS update_product_variants_updated_at ON product_variants;
DROP INDEX IF EXISTS idx_product_variants_product_id;
DROP TABLE IF EXISTS product_variants;
