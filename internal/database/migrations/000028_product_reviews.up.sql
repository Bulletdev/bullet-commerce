-- Product reviews (Catalog v2). Customer-authored ratings (1..5) with optional title/body
-- and a moderation lifecycle (`status`). One review per (product, user) so a customer can
-- rate a product once; the UNIQUE constraint enforces that at the DB. The approved subset
-- feeds the denormalized aggregate columns on products (rating_avg / rating_count) so the
-- catalog can sort/filter/display ratings without a join or an aggregate on every read.
CREATE TABLE IF NOT EXISTS product_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    rating INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    title TEXT,
    body TEXT,
    status TEXT NOT NULL DEFAULT 'approved'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, user_id)
);

-- Public listing reads by product filtered to approved reviews.
CREATE INDEX IF NOT EXISTS idx_product_reviews_product_status
    ON product_reviews(product_id, status);
-- "my reviews" / moderation-by-author lookups.
CREATE INDEX IF NOT EXISTS idx_product_reviews_user_id
    ON product_reviews(user_id);

CREATE TRIGGER update_product_reviews_updated_at
BEFORE UPDATE ON product_reviews
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Denormalized rating aggregate kept in sync by the reviews repository's RecomputeAggregate.
-- rating_avg stays NULL until a product has at least one approved review; rating_count is a
-- NOT NULL running total that defaults to 0 so the catalog never has to coalesce it.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS rating_avg NUMERIC(3, 2),
    ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;
