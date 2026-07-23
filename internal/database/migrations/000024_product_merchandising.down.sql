DROP INDEX IF EXISTS idx_product_categories_category_id;
DROP TABLE IF EXISTS product_categories;

DROP INDEX IF EXISTS idx_products_slug;

ALTER TABLE products
    DROP COLUMN IF EXISTS compare_at_price_cents,
    DROP COLUMN IF EXISTS brand_id,
    DROP COLUMN IF EXISTS meta_description,
    DROP COLUMN IF EXISTS meta_title,
    DROP COLUMN IF EXISTS slug,
    DROP COLUMN IF EXISTS status;

DROP TRIGGER IF EXISTS update_brands_updated_at ON brands;
DROP TABLE IF EXISTS brands;
