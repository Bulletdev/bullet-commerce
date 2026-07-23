-- Deliveries make a cart/order fulfillable through N shipments (ship-to-address today,
-- store pickup / pickup point later). WHY a "default" delivery mirrors the default-variant
-- pattern of 000010: the single-shipment case stays invisible to clients, but every
-- cart_item / order_item now hangs off a delivery, so multi-delivery becomes a data change
-- (add rows) rather than a schema rewrite.
CREATE TABLE IF NOT EXISTS deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- A delivery belongs to exactly one of cart OR order over its lifetime: it is created
    -- on the cart and its mirror is re-created on the order at checkout. Both FKs are
    -- nullable and cascade so deleting the parent removes its deliveries.
    cart_id UUID REFERENCES carts(id) ON DELETE CASCADE,
    order_id UUID REFERENCES orders(id) ON DELETE CASCADE,
    code TEXT NOT NULL DEFAULT 'default',
    method TEXT,
    carrier TEXT,
    -- location_type distinguishes ship-to-address from the future store/pickup flows.
    location_type TEXT NOT NULL DEFAULT 'address' CHECK (location_type IN ('address', 'store', 'pickup_point')),
    address_id UUID,
    -- Freight now belongs to the delivery that incurs it, not the order: the order total
    -- still sums items - discount + freight, but the freight is attributed to a shipment.
    shipping_cost_cents BIGINT NOT NULL DEFAULT 0 CHECK (shipping_cost_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partial UNIQUE indexes: a delivery is looked up by its owning cart or order (never both),
-- and (parent, code) is unique so the default delivery stays singular per cart/order — this
-- is what makes ensureDefaultDelivery's ON CONFLICT DO NOTHING race-safe. Two deliveries on
-- the same cart/order are allowed as long as they carry distinct codes (multi-delivery).
CREATE UNIQUE INDEX IF NOT EXISTS idx_deliveries_cart_code ON deliveries(cart_id, code) WHERE cart_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_deliveries_order_code ON deliveries(order_id, code) WHERE order_id IS NOT NULL;

-- Assumes update_updated_at_column() exists (created in migration 000001).
CREATE TRIGGER update_deliveries_updated_at
BEFORE UPDATE ON deliveries
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Attach items to a delivery. Nullable first so the backfill can populate existing rows,
-- then SET NOT NULL once every item points at its cart/order default delivery.
ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS delivery_id UUID;
ALTER TABLE order_items ADD COLUMN IF NOT EXISTS delivery_id UUID;

-- Backfill: one default delivery per existing cart, then point that cart's items at it.
-- WHY per-cart even when the cart has no items: the invariant is "a cart has a default
-- delivery" independent of whether items exist yet, so a later AddItem always finds it ready.
INSERT INTO deliveries (cart_id, code, location_type)
SELECT c.id, 'default', 'address'
FROM carts c
WHERE NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.cart_id = c.id AND d.code = 'default');

UPDATE cart_items ci
SET delivery_id = d.id
FROM deliveries d
WHERE d.cart_id = ci.cart_id AND d.code = 'default' AND ci.delivery_id IS NULL;

-- Same for orders. Carry the order's existing shipping_cost_cents / shipping_method onto its
-- default delivery so freight now "belongs" to the shipment without changing the order total.
INSERT INTO deliveries (order_id, code, location_type, shipping_cost_cents, method)
SELECT o.id, 'default', 'address', o.shipping_cost_cents, o.shipping_method
FROM orders o
WHERE NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.order_id = o.id AND d.code = 'default');

UPDATE order_items oi
SET delivery_id = d.id
FROM deliveries d
WHERE d.order_id = oi.order_id AND d.code = 'default' AND oi.delivery_id IS NULL;

-- Every item now points at a delivery, so enforce it and wire the FKs. CASCADE on both so a
-- deleted delivery (which only happens when its cart/order is deleted) never orphans a row or
-- deadlocks the parent's cascade.
ALTER TABLE cart_items ALTER COLUMN delivery_id SET NOT NULL;
ALTER TABLE order_items ALTER COLUMN delivery_id SET NOT NULL;
ALTER TABLE cart_items ADD CONSTRAINT fk_cart_items_delivery
    FOREIGN KEY (delivery_id) REFERENCES deliveries(id) ON DELETE CASCADE;
ALTER TABLE order_items ADD CONSTRAINT fk_order_items_delivery
    FOREIGN KEY (delivery_id) REFERENCES deliveries(id) ON DELETE CASCADE;
