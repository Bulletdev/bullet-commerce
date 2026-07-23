-- users: role + cpf
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user'
    CHECK (role IN ('user', 'admin')),
  ADD COLUMN IF NOT EXISTS cpf TEXT;

-- products: stock + featured + soft delete
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS stock INT NOT NULL DEFAULT 0
    CHECK (stock >= 0),
  ADD COLUMN IF NOT EXISTS featured BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_products_featured ON products(featured) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_products_deleted_at ON products(deleted_at);

-- orders: payment lifecycle + soft delete
ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS payment_status TEXT NOT NULL DEFAULT 'unpaid'
    CHECK (payment_status IN ('unpaid', 'pending_payment', 'paid', 'failed')),
  ADD COLUMN IF NOT EXISTS payment_method TEXT
    CHECK (payment_method IN ('pix', 'credit_card', 'boleto')),
  ADD COLUMN IF NOT EXISTS payment_reference TEXT UNIQUE,
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_orders_payment_status ON orders(payment_status);
CREATE INDEX IF NOT EXISTS idx_orders_deleted_at ON orders(deleted_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_payment_reference ON orders(payment_reference)
  WHERE payment_reference IS NOT NULL;
