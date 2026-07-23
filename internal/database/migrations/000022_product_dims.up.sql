-- Shipping dimensions on the product. All nullable: freight (/shipping/calculate) only
-- uses them when present, so pre-existing products keep working with no dimensions until
-- an admin fills them. Grams and millimetres are integers to keep the freight math exact.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS weight_grams INT,
    ADD COLUMN IF NOT EXISTS length_mm INT,
    ADD COLUMN IF NOT EXISTS width_mm INT,
    ADD COLUMN IF NOT EXISTS height_mm INT;
