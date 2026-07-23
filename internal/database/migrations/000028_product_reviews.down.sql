ALTER TABLE products
    DROP COLUMN IF EXISTS rating_count,
    DROP COLUMN IF EXISTS rating_avg;

DROP TRIGGER IF EXISTS update_product_reviews_updated_at ON product_reviews;
DROP INDEX IF EXISTS idx_product_reviews_user_id;
DROP INDEX IF EXISTS idx_product_reviews_product_status;
DROP TABLE IF EXISTS product_reviews;
