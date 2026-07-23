-- Reverses 000018: drops the cart coupon-codes column and the coupons table.
ALTER TABLE carts DROP COLUMN IF EXISTS applied_coupon_codes;
DROP TABLE IF EXISTS coupons;
