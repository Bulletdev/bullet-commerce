-- Variant enrichment (PRD §3.5/§3.6/§4). Adds the merchandising/logistics columns a real
-- store needs on the sellable line and materializes the variant price so consumers stop
-- carrying the `variant.price ?? product.price` fallback.

ALTER TABLE product_variants
    -- Shipping override: NULL means "fall back to the product's weight/dimensions"; set means
    -- this variant ships differently (e.g. a heavier size) and the freight calc uses these.
    ADD COLUMN IF NOT EXISTS weight_grams INT,
    ADD COLUMN IF NOT EXISTS length_mm INT,
    ADD COLUMN IF NOT EXISTS width_mm INT,
    ADD COLUMN IF NOT EXISTS height_mm INT,
    -- GTIN/EAN for NF-e, marketplace feeds and warehouse scanning.
    ADD COLUMN IF NOT EXISTS barcode TEXT,
    -- Deactivate a variant (hide/stop selling) without soft-deleting it.
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    -- Display order among a product's variants.
    ADD COLUMN IF NOT EXISTS position INT NOT NULL DEFAULT 0,
    -- "De/por": show a strikethrough original price when compare_at > price. Nullable = no promo.
    ADD COLUMN IF NOT EXISTS compare_at_price_cents BIGINT
        CHECK (compare_at_price_cents IS NULL OR compare_at_price_cents >= 0),
    -- 'deny' = never oversell (current behavior); 'backorder' = allow selling with no stock.
    -- The reserve-path enforcement of 'backorder' is a documented follow-up; this only carries
    -- the column so the policy can be recorded now.
    ADD COLUMN IF NOT EXISTS stock_policy TEXT NOT NULL DEFAULT 'deny'
        CHECK (stock_policy IN ('deny','backorder')),
    -- Signals the price was copied from the parent product (not admin-set). A future product
    -- price update fans out to price_inherited = TRUE variants only. Fan-out itself is a
    -- follow-up (PRD §4); this migration just establishes the signal.
    ADD COLUMN IF NOT EXISTS price_inherited BOOLEAN NOT NULL DEFAULT TRUE;

-- Materialize the price. The new column defaults every row to price_inherited = TRUE, so first
-- demote the rows that ALREADY carried an explicit price to non-inherited (admin-set) BEFORE the
-- backfill touches only the NULL/inherited ones.
UPDATE product_variants SET price_inherited = FALSE WHERE price_cents IS NOT NULL;

-- Backfill the inherited (NULL) prices from the parent product. These stay price_inherited = TRUE.
UPDATE product_variants v
SET price_cents = p.price_cents
FROM products p
WHERE p.id = v.product_id AND v.price_cents IS NULL;

-- End of the null-price fallback: money is now always explicit on the sellable line.
ALTER TABLE product_variants ALTER COLUMN price_cents SET NOT NULL;

-- FindByProductID orders by (position, created_at); index the non-deleted rows for it.
CREATE INDEX IF NOT EXISTS idx_product_variants_product_position
    ON product_variants(product_id, position) WHERE deleted_at IS NULL;
