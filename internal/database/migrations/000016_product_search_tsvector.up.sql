-- Full-text search over products. ILIKE '%term%' can't use an index and can't rank, so
-- searches degrade to full scans with no relevance ordering. A tsvector column indexed
-- with GIN lets Postgres rank matches (ts_rank) against a 'portuguese' dictionary
-- (stemming + stop words) at index speed.
--
-- GENERATED ... STORED is chosen over a trigger: the column is always consistent with
-- name+description with nothing to maintain, and adding the column computes the value for
-- every existing row in place, so the backfill of current products is automatic.
-- name is weighted above description so a term in the title outranks one in the body.

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('portuguese', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('portuguese', coalesce(description, '')), 'B')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_products_search_tsv ON products USING GIN (search_tsv);
