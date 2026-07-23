-- Variants are the real sellable units: a product ("T-shirt") has no stock of its
-- own, its variants ("T-shirt / M / azul") do. The stock invariant therefore lives
-- on the variant, not the product. `products.stock` becomes DEPRECATED here (kept,
-- not dropped, so the existing product/cart/order code that still reads it keeps
-- working until the read path is migrated during integration).

CREATE TABLE IF NOT EXISTS product_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id),
    sku TEXT UNIQUE NOT NULL,
    attributes JSONB NOT NULL DEFAULT '{}',
    -- NULL price_cents means "inherit the parent product's price" so a variant that
    -- only differs by size/color needs no price of its own.
    price_cents BIGINT,
    currency CHAR(3) NOT NULL DEFAULT 'BRL',
    stock INT NOT NULL DEFAULT 0 CHECK (stock >= 0),
    -- stock_reserved can never exceed stock: reservations are a subset of physical
    -- stock, and available-for-sale = stock - stock_reserved must stay >= 0.
    stock_reserved INT NOT NULL DEFAULT 0 CHECK (stock_reserved >= 0 AND stock_reserved <= stock),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial index: every "list variants of a product" query filters deleted_at IS NULL.
CREATE INDEX IF NOT EXISTS idx_product_variants_product_id
    ON product_variants(product_id) WHERE deleted_at IS NULL;

-- Backfill one default variant per existing product so no product is left without a
-- variant (the read path can then always resolve stock through variants). The default
-- variant inherits price (price_cents NULL) and carries over the product's own stock.
INSERT INTO product_variants (product_id, sku, attributes, price_cents, currency, stock)
SELECT p.id, 'default-' || p.id, '{}', NULL, p.currency, p.stock
FROM products p
WHERE NOT EXISTS (
    SELECT 1 FROM product_variants v WHERE v.product_id = p.id
);

-- Assumes update_updated_at_column() exists (created in migration 000001).
CREATE TRIGGER update_product_variants_updated_at
BEFORE UPDATE ON product_variants
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();
