-- Reverses 000015: drops the standalone applied_discounts table. No cart/orders
-- columns were added by the up migration, so nothing else needs restoring.
DROP INDEX IF EXISTS idx_applied_discounts_campaign_code;
DROP INDEX IF EXISTS idx_applied_discounts_cart_id;
DROP INDEX IF EXISTS idx_applied_discounts_order_id;
DROP TABLE IF EXISTS applied_discounts;
