-- Merchandising & publication layer (Tier 2). Separates three orthogonal states that
-- were previously conflated: `status` (published lifecycle) vs `featured` (highlight) vs
-- `deleted_at` (soft delete / LGPD). The public catalog filters status = 'active'.

-- Brands are a first-class entity so products can share one (name/logo reused across SKUs).
CREATE TABLE IF NOT EXISTS brands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE,
    logo_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_brands_updated_at
BEFORE UPDATE ON brands
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('draft', 'active', 'archived')),
    ADD COLUMN IF NOT EXISTS slug TEXT,
    ADD COLUMN IF NOT EXISTS meta_title TEXT,
    ADD COLUMN IF NOT EXISTS meta_description TEXT,
    ADD COLUMN IF NOT EXISTS brand_id UUID REFERENCES brands(id),
    ADD COLUMN IF NOT EXISTS compare_at_price_cents BIGINT
        CHECK (compare_at_price_cents IS NULL OR compare_at_price_cents >= 0);

-- Backfill slug from the name (lowercased, non-alphanumerics collapsed to '-') plus a short
-- id suffix so two products with the same name never collide. Only fills rows still NULL.
UPDATE products
SET slug = trim(both '-' from regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g'))
           || '-' || substr(id::text, 1, 8)
WHERE slug IS NULL;

-- Unique per live product: a soft-deleted product frees its slug for reuse.
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_slug
    ON products(slug) WHERE deleted_at IS NULL;

-- N:N categories. `products.category_id` stays the PRIMARY category (breadcrumb / URL);
-- this table holds the additional secondary categories a product also lives under.
CREATE TABLE IF NOT EXISTS product_categories (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_product_categories_category_id
    ON product_categories(category_id);

-- Backfill the N:N from the existing single primary category so no membership is lost.
INSERT INTO product_categories (product_id, category_id)
SELECT id, category_id FROM products WHERE category_id IS NOT NULL
ON CONFLICT DO NOTHING;
