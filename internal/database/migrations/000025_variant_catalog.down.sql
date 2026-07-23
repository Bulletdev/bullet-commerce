DROP INDEX IF EXISTS idx_product_variants_product_position;

-- NOSONAR Only relax the NOT NULL; the pre-materialization NULLs cannot be reconstructed (we no longer
-- know which prices were inherited once the values were backfilled), so leave price_cents as-is.
ALTER TABLE product_variants ALTER COLUMN price_cents DROP NOT NULL;

ALTER TABLE product_variants
    DROP COLUMN IF EXISTS price_inherited,
    DROP COLUMN IF EXISTS stock_policy,
    DROP COLUMN IF EXISTS compare_at_price_cents,
    DROP COLUMN IF EXISTS position,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS barcode,
    DROP COLUMN IF EXISTS height_mm,
    DROP COLUMN IF EXISTS width_mm,
    DROP COLUMN IF EXISTS length_mm,
    DROP COLUMN IF EXISTS weight_grams;
