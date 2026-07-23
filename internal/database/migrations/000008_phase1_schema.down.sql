ALTER TABLE users DROP COLUMN IF EXISTS role;
ALTER TABLE users DROP COLUMN IF EXISTS cpf;

DROP INDEX IF EXISTS idx_products_featured;
DROP INDEX IF EXISTS idx_products_deleted_at;
ALTER TABLE products DROP COLUMN IF EXISTS stock;
ALTER TABLE products DROP COLUMN IF EXISTS featured;
ALTER TABLE products DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_orders_payment_status;
DROP INDEX IF EXISTS idx_orders_deleted_at;
DROP INDEX IF EXISTS idx_orders_payment_reference;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_status;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_method;
ALTER TABLE orders DROP COLUMN IF EXISTS payment_reference;
ALTER TABLE orders DROP COLUMN IF EXISTS deleted_at;
