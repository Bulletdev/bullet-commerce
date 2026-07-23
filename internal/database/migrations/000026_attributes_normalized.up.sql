-- Normalized attributes: the §3.7 hybrid. Only the VARIATION keys (the ones that drive
-- variant selection, listed in products.variant_variation_attributes) get promoted into
-- normalized tables so they can be faceted, validated, ordered and given a color swatch.
-- The free-form product_variants.attributes JSONB STAYS as the source of truth for
-- everything else (material, care instructions, ...) — it is never emptied here; these
-- tables are a queryable projection of its variation subset, not a replacement.

CREATE TABLE IF NOT EXISTS attribute (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- code matches products.variant_variation_attributes entries (e.g. 'tamanho', 'cor'),
    -- so a variation key becomes a validated reference instead of a loose string.
    code TEXT UNIQUE NOT NULL,
    label TEXT NOT NULL,
    -- kind drives the selection UI: 'color' renders a swatch (values carry hex), 'select'
    -- an ordered list, 'text' a free entry.
    kind TEXT NOT NULL DEFAULT 'select' CHECK (kind IN ('select', 'color', 'text')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS attribute_value (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attribute_id UUID NOT NULL REFERENCES attribute(id) ON DELETE CASCADE,
    value TEXT NOT NULL,
    label TEXT NOT NULL,
    -- hex is the swatch color for kind='color' values (e.g. '#000000'). NULL until the admin
    -- fills it: the backfill below deliberately does NOT infer it from a value like 'preto'.
    hex TEXT NULL,
    -- position orders values for display (so 'M' can precede 'G' instead of sorting alpha).
    position INT NOT NULL DEFAULT 0,
    UNIQUE (attribute_id, value)
);
CREATE INDEX IF NOT EXISTS idx_attribute_value_attribute_id ON attribute_value(attribute_id);

-- variant_attribute_value links a variant to the normalized value it carries for a variation
-- key. The composite PK makes the link idempotent (LinkVariant can re-run harmlessly).
CREATE TABLE IF NOT EXISTS variant_attribute_value (
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    attribute_value_id UUID NOT NULL REFERENCES attribute_value(id) ON DELETE CASCADE,
    PRIMARY KEY (variant_id, attribute_value_id)
);
CREATE INDEX IF NOT EXISTS idx_variant_attribute_value_value_id
    ON variant_attribute_value(attribute_value_id);

-- ── BACKFILL ────────────────────────────────────────────────────────────────────────────
-- Only known variation keys are promoted; free-form JSONB keys are left where they are.

-- 1) One attribute per distinct variation code across all products. A code containing 'cor'
--    or 'color' becomes kind='color' (swatch), everything else 'select'. label defaults to the
--    code — a human label is left for the admin to refine later.
INSERT INTO attribute (code, label, kind)
SELECT DISTINCT
    code,
    code AS label,
    CASE WHEN code ILIKE '%cor%' OR code ILIKE '%color%' THEN 'color' ELSE 'select' END AS kind
FROM (
    SELECT unnest(variant_variation_attributes) AS code FROM products
) codes
WHERE code IS NOT NULL AND code <> ''
ON CONFLICT (code) DO NOTHING;

-- 2) One attribute_value per distinct (variation key, value) seen on a non-deleted variant.
--    position is assigned by order of first appearance (earliest-created variant carrying the
--    value comes first), so display order reflects the catalog's own ordering rather than
--    alphabetical. hex stays NULL — not inferred from the value text.
WITH variant_attrs AS (
    SELECT v.id AS variant_id, v.created_at, kv.key AS attr_code, kv.value AS attr_value
    FROM product_variants v
    JOIN products p ON p.id = v.product_id
    CROSS JOIN LATERAL jsonb_each_text(v.attributes) AS kv(key, value)
    WHERE v.deleted_at IS NULL
      AND kv.key = ANY(p.variant_variation_attributes)
),
value_positions AS (
    SELECT
        attr_code,
        attr_value,
        ROW_NUMBER() OVER (PARTITION BY attr_code ORDER BY MIN(created_at), attr_value) - 1 AS position
    FROM variant_attrs
    GROUP BY attr_code, attr_value
)
INSERT INTO attribute_value (attribute_id, value, label, position)
SELECT a.id, vp.attr_value, vp.attr_value, vp.position
FROM value_positions vp
JOIN attribute a ON a.code = vp.attr_code
ON CONFLICT (attribute_id, value) DO NOTHING;

-- 3) Link each non-deleted variant to the normalized values it already carries in its JSONB.
INSERT INTO variant_attribute_value (variant_id, attribute_value_id)
SELECT DISTINCT v.id, av.id
FROM product_variants v
JOIN products p ON p.id = v.product_id
CROSS JOIN LATERAL jsonb_each_text(v.attributes) AS kv(key, value)
JOIN attribute a ON a.code = kv.key
JOIN attribute_value av ON av.attribute_id = a.id AND av.value = kv.value
WHERE v.deleted_at IS NULL
  AND kv.key = ANY(p.variant_variation_attributes)
ON CONFLICT DO NOTHING;
