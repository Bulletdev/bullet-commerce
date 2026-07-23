-- Reverses 000013: drops the independent billing/shipping default flags and their indexes.
DROP INDEX IF EXISTS idx_addresses_user_default_shipping;
DROP INDEX IF EXISTS idx_addresses_user_default_billing;

ALTER TABLE addresses
    DROP COLUMN IF EXISTS is_default_shipping,
    DROP COLUMN IF EXISTS is_default_billing;
