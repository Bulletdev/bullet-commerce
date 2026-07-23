-- Reverts money-as-cents back to NUMERIC(10,2). Precision restored from cents / 100.

ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS price NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (price >= 0);
UPDATE cart_items SET price = price_cents / 100.0; -- NOSONAR: intentional full-table update reverting a data migration
ALTER TABLE cart_items DROP COLUMN IF EXISTS price_cents;

ALTER TABLE order_items ADD COLUMN IF NOT EXISTS price NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (price >= 0);
UPDATE order_items SET price = price_cents / 100.0; -- NOSONAR: intentional full-table update reverting a data migration
ALTER TABLE order_items DROP COLUMN IF EXISTS price_cents;

ALTER TABLE orders ADD COLUMN IF NOT EXISTS total NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (total >= 0);
UPDATE orders SET total = total_cents / 100.0; -- NOSONAR: intentional full-table update reverting a data migration
ALTER TABLE orders DROP COLUMN IF EXISTS total_cents;
ALTER TABLE orders DROP COLUMN IF EXISTS currency;

ALTER TABLE products ADD COLUMN IF NOT EXISTS price NUMERIC(10, 2) NOT NULL DEFAULT 0 CHECK (price >= 0);
UPDATE products SET price = price_cents / 100.0; -- NOSONAR: intentional full-table update reverting a data migration
ALTER TABLE products DROP COLUMN IF EXISTS price_cents;
ALTER TABLE products DROP COLUMN IF EXISTS currency;
