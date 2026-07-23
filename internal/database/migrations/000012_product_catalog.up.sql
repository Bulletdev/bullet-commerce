-- Enriches the product aggregate so a product can be more than a single sellable
-- unit: a `type` discriminates simple/configurable/bundle products, `attributes`
-- carries product-level JSONB metadata, and `variant_variation_attributes` records
-- which variant attribute keys drive the selection UI (e.g. ["tamanho","cor"]).
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'simple'
        CHECK (type IN ('simple', 'configurable', 'bundle')),
    ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS variant_variation_attributes TEXT[] NOT NULL DEFAULT '{}';

-- A bundle's composition lives in its own tables because it references OTHER
-- products rather than size/color permutations of itself. A choice is a slot the
-- customer fills; min/max bound the pick count and required forbids an empty slot.
CREATE TABLE IF NOT EXISTS product_bundle_choices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    min_qty INT NOT NULL DEFAULT 1 CHECK (min_qty >= 0),
    -- max must be able to satisfy min, otherwise the slot is unfillable.
    max_qty INT NOT NULL DEFAULT 1 CHECK (max_qty >= min_qty),
    required BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_bundle_choices_product_id
    ON product_bundle_choices(product_id);

-- Each option is one product eligible for a choice; default_qty is what's
-- pre-selected when the bundle is first rendered.
CREATE TABLE IF NOT EXISTS product_bundle_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    choice_id UUID NOT NULL REFERENCES product_bundle_choices(id) ON DELETE CASCADE,
    option_product_id UUID NOT NULL REFERENCES products(id),
    default_qty INT NOT NULL DEFAULT 0 CHECK (default_qty >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_bundle_options_choice_id
    ON product_bundle_options(choice_id);
