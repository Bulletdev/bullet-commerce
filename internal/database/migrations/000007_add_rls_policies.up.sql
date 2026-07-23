-- Enable Row Level Security for all relevant tables
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE addresses ENABLE ROW LEVEL SECURITY;
ALTER TABLE carts ENABLE ROW LEVEL SECURITY;
ALTER TABLE cart_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories ENABLE ROW LEVEL SECURITY;

-- Force RLS for table owners (recommended by Supabase)
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE addresses FORCE ROW LEVEL SECURITY;
ALTER TABLE carts FORCE ROW LEVEL SECURITY;
ALTER TABLE cart_items FORCE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;
ALTER TABLE order_items FORCE ROW LEVEL SECURITY;
ALTER TABLE products FORCE ROW LEVEL SECURITY;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;


-- Policies for 'users' table
-- Users can select their own data
CREATE POLICY "Allow individual select access" ON users FOR SELECT
    USING (auth.uid() = id);
-- Users can update their own data
CREATE POLICY "Allow individual update access" ON users FOR UPDATE
    USING (auth.uid() = id)
    WITH CHECK (auth.uid() = id);

-- Policies for 'addresses' table
-- Users can manage their own addresses fully
CREATE POLICY "Allow full access to owner" ON addresses FOR ALL
    USING (auth.uid() = user_id)
    WITH CHECK (auth.uid() = user_id);

-- Policies for 'carts' table
-- Users can manage their own cart fully
CREATE POLICY "Allow full access to owner" ON carts FOR ALL
    USING (auth.uid() = user_id)
    WITH CHECK (auth.uid() = user_id);

-- Policies for 'cart_items' table
-- Users can manage items only if they own the corresponding cart
CREATE POLICY "Allow access based on cart owner" ON cart_items FOR ALL
    USING ( EXISTS (SELECT 1 FROM carts WHERE carts.id = cart_items.cart_id AND carts.user_id = auth.uid()) )
    WITH CHECK ( EXISTS (SELECT 1 FROM carts WHERE carts.id = cart_items.cart_id AND carts.user_id = auth.uid()) );

-- Policies for 'orders' table
-- Users can select their own orders
CREATE POLICY "Allow select access to owner" ON orders FOR SELECT
    USING (auth.uid() = user_id);
-- Users can insert orders (user_id check ensures they insert for themselves)
CREATE POLICY "Allow insert for authenticated users" ON orders FOR INSERT
    WITH CHECK (auth.uid() = user_id);
-- (No UPDATE/DELETE policies initially - managed by API logic)

-- Policies for 'order_items' table
-- Users can select items belonging to their own orders
CREATE POLICY "Allow select based on order owner" ON order_items FOR SELECT
    USING ( EXISTS (SELECT 1 FROM orders WHERE orders.id = order_items.order_id AND orders.user_id = auth.uid()) );
-- (No INSERT/UPDATE/DELETE policies initially)

-- Policies for 'products' table
-- Allow public read access to products
CREATE POLICY "Allow public select access" ON products FOR SELECT
    USING (true);
-- Allow authenticated users to manage products (can be restricted to admin later)
CREATE POLICY "Allow modification for authenticated users" ON products FOR ALL
    USING (auth.role() = 'authenticated') -- NOSONAR Allow reading existing rows if authenticated
    WITH CHECK (auth.role() = 'authenticated'); -- Check applies to INSERT/UPDATE

-- Policies for 'categories' table
-- Allow public read access to categories
CREATE POLICY "Allow public select access" ON categories FOR SELECT
    USING (true);
-- Allow authenticated users to manage categories (can be restricted to admin later)
CREATE POLICY "Allow modification for authenticated users" ON categories FOR ALL
    USING (auth.role() = 'authenticated') -- Allow reading existing rows if authenticated
    WITH CHECK (auth.role() = 'authenticated'); -- Check applies to INSERT/UPDATE