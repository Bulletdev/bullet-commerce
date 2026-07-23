-- Money as integer minor units (cents).
-- NUMERIC scanned into Go float64 loses precision (0.1+0.2 != 0.3); integer cents
-- is exact, currency-neutral, and matches every payment-provider API (Stripe, OpenPix).
-- The `currency` column makes the API reusable across stores/markets; a fork in a
-- non-BRL market changes only the DEFAULT here and models.DefaultCurrency.

ALTER TABLE products
  ADD COLUMN IF NOT EXISTS price_cents BIGINT NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'BRL';
UPDATE products SET price_cents = ROUND(price * 100) WHERE price IS NOT NULL;
ALTER TABLE products ALTER COLUMN price_cents DROP DEFAULT;
ALTER TABLE products DROP COLUMN IF EXISTS price;

ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS total_cents BIGINT NOT NULL DEFAULT 0 CHECK (total_cents >= 0),
  ADD COLUMN IF NOT EXISTS currency CHAR(3) NOT NULL DEFAULT 'BRL';
UPDATE orders SET total_cents = ROUND(total * 100) WHERE total IS NOT NULL;
ALTER TABLE orders ALTER COLUMN total_cents DROP DEFAULT;
ALTER TABLE orders DROP COLUMN IF EXISTS total;

ALTER TABLE order_items
  ADD COLUMN IF NOT EXISTS price_cents BIGINT NOT NULL DEFAULT 0 CHECK (price_cents >= 0);
UPDATE order_items SET price_cents = ROUND(price * 100) WHERE price IS NOT NULL;
ALTER TABLE order_items ALTER COLUMN price_cents DROP DEFAULT;
ALTER TABLE order_items DROP COLUMN IF EXISTS price;

ALTER TABLE cart_items
  ADD COLUMN IF NOT EXISTS price_cents BIGINT NOT NULL DEFAULT 0 CHECK (price_cents >= 0);
UPDATE cart_items SET price_cents = ROUND(price * 100) WHERE price IS NOT NULL;
ALTER TABLE cart_items ALTER COLUMN price_cents DROP DEFAULT;
ALTER TABLE cart_items DROP COLUMN IF EXISTS price;
