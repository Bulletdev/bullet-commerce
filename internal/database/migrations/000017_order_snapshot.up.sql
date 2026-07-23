-- Snapshot the product name and variant SKU onto each order line at purchase time.
-- WHY freeze here (not join to products/product_variants at read time): a catalog rename,
-- a SKU change, or a soft-delete must never rewrite historical orders — a line must always
-- show what the customer actually bought. Columns are nullable with no backfill: existing
-- lines keep NULL (their catalog rows still resolve for lookup), new lines are frozen on
-- INSERT via the INSERT ... SELECT in the order repository.
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS product_name TEXT;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS variant_sku TEXT;
