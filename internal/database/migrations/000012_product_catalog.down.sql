-- Reverses 000012: drops the bundle tables (options first, they FK the choices) and
-- the product catalog columns.
DROP TABLE IF EXISTS product_bundle_options;
DROP TABLE IF EXISTS product_bundle_choices;

ALTER TABLE products
    DROP COLUMN IF EXISTS variant_variation_attributes,
    DROP COLUMN IF EXISTS attributes,
    DROP COLUMN IF EXISTS type;
