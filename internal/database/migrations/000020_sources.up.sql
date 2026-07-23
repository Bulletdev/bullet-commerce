-- Sourcing generalizes the implicit single stock location into N named sources
-- (warehouses / stores). WHY a "default" source mirrors the default-variant (000010) and
-- default-delivery (000019) patterns: the single-location case stays invisible to clients,
-- but variant stock now hangs off a (variant, source) pair, so multi-warehouse becomes a
-- data change (add sources + rows) rather than a schema rewrite. With one default source the
-- observable behavior (available, reserved) is identical to the pre-sourcing world.

CREATE TABLE IF NOT EXISTS sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    -- is_default marks the source the SingleSourceAllocator ships everything from. The
    -- partial unique index below guarantees at most one row can carry is_default = TRUE.
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Only one default source may ever exist: the partial unique index enforces the singleton
-- without forbidding many non-default sources.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sources_single_default
    ON sources(is_default) WHERE is_default = TRUE;

-- The transparent default source. Every existing variant's stock is backfilled onto it below.
INSERT INTO sources (code, name, is_default)
VALUES ('default', 'Default', TRUE)
ON CONFLICT (code) DO NOTHING;

-- Per-source stock: the invariant (available = stock - stock_reserved, never negative) now
-- lives per (variant, source). product_variants.stock / stock_reserved become DEPRECATED here
-- (kept, not dropped, so the display read path that still reads them keeps working until it is
-- migrated to sum variant_stock via AvailableForVariant). The write path (Reserve/Claim/Release)
-- moves to variant_stock.
CREATE TABLE IF NOT EXISTS variant_stock (
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES sources(id),
    stock INT NOT NULL DEFAULT 0 CHECK (stock >= 0),
    -- stock_reserved is a subset of physical stock at this source: reservations can never
    -- exceed stock, so available-for-sale stays >= 0.
    stock_reserved INT NOT NULL DEFAULT 0 CHECK (stock_reserved >= 0 AND stock_reserved <= stock),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (variant_id, source_id)
);

-- Backfill one variant_stock row per existing variant on the default source, carrying over the
-- variant's own stock / stock_reserved so no availability is lost by the move.
INSERT INTO variant_stock (variant_id, source_id, stock, stock_reserved)
SELECT v.id, s.id, v.stock, v.stock_reserved
FROM product_variants v
CROSS JOIN sources s
WHERE s.is_default = TRUE
ON CONFLICT (variant_id, source_id) DO NOTHING;

-- order_items record which source each line was sourced from, so a later Release/Claim frees
-- the exact (variant, source) it reserved. Nullable first for the backfill, then SET NOT NULL.
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS source_id UUID REFERENCES sources(id);

UPDATE order_items oi
SET source_id = s.id
FROM sources s
WHERE s.is_default = TRUE AND oi.source_id IS NULL;

ALTER TABLE order_items ALTER COLUMN source_id SET NOT NULL;
