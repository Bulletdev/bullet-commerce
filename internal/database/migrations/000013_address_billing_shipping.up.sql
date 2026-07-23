-- Split the single address default into independent billing and shipping defaults:
-- the address a customer bills to is often not the one they ship to.
ALTER TABLE addresses
    ADD COLUMN is_default_billing BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN is_default_shipping BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill: the previous single default served both purposes, so seed both flags from it.
-- is_default is retained for backward compatibility (deprecated in favor of the pair above).
UPDATE addresses
SET is_default_billing = TRUE, is_default_shipping = TRUE
WHERE is_default = TRUE;

-- Enforce at most one default of each kind per user.
CREATE UNIQUE INDEX idx_addresses_user_default_billing
    ON addresses(user_id) WHERE is_default_billing;
CREATE UNIQUE INDEX idx_addresses_user_default_shipping
    ON addresses(user_id) WHERE is_default_shipping;
