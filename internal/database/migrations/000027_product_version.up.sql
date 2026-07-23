-- Optimistic concurrency control on products. `version` starts at 1 and the repository
-- bumps it on every UPDATE, guarding the write with the caller's expected version so two
-- concurrent admin edits can't silently clobber each other (a stale write hits 0 rows and
-- surfaces as a 409 conflict instead of a lost update).
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
