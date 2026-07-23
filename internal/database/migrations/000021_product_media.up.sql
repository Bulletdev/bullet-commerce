-- Product media is the catalog's first support for images/videos (PRD Catalog v2 §3.1).
-- The API never hosts files: `url` points at an external CDN/bucket object. Two ingestion
-- flows feed this table with the same shape - (1) an admin references an existing CDN URL,
-- (2) an admin uploads through a presigned URL and then registers the resulting public URL.
-- `variant_id` NULL = media of the product as a whole; set = media of a specific variant
-- (e.g. the photo for the "blue" colorway), so N images per variant are possible without a
-- separate table. ON DELETE CASCADE on both FKs keeps media from outliving its owner.
CREATE TABLE IF NOT EXISTS product_media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id UUID REFERENCES product_variants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    alt TEXT,
    kind TEXT NOT NULL DEFAULT 'image' CHECK (kind IN ('image', 'video')),
    -- position orders the gallery; the lowest position is the primary image.
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- "list media of a product" is the hot read path (product detail).
CREATE INDEX IF NOT EXISTS idx_product_media_product_id
    ON product_media(product_id);

-- "list media of a variant within a product" (per-colorway gallery) filters on both keys.
CREATE INDEX IF NOT EXISTS idx_product_media_product_variant
    ON product_media(product_id, variant_id);

-- Assumes update_updated_at_column() exists (created in migration 000001).
CREATE TRIGGER update_product_media_updated_at
BEFORE UPDATE ON product_media
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
