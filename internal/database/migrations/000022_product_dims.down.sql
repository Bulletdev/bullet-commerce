ALTER TABLE products
    DROP COLUMN IF EXISTS height_mm,
    DROP COLUMN IF EXISTS width_mm,
    DROP COLUMN IF EXISTS length_mm,
    DROP COLUMN IF EXISTS weight_grams;
