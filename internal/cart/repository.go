package cart

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var ErrProductNotInCart = errors.New("product not found in cart")

// CartRepository persists cart lines. The line identity is the VARIANT (not the
// product): the same product in two variants are distinct lines, so Update/Remove/Find
// key by variantID and AddItem upserts on (cart_id, variant_id).
type CartRepository interface {
	GetOrCreateCartByUserID(ctx context.Context, userID uuid.UUID) (*models.Cart, error)
	GetCartItems(ctx context.Context, cartID uuid.UUID) ([]models.CartItem, error)
	AddItem(ctx context.Context, cartID, productID, variantID uuid.UUID, quantity int, priceCents int64) (*models.CartItem, error)
	UpdateItemQuantity(ctx context.Context, cartID, variantID uuid.UUID, quantity int) (*models.CartItem, error)
	RemoveItem(ctx context.Context, cartID, variantID uuid.UUID) error
	FindCartItem(ctx context.Context, cartID, variantID uuid.UUID) (*models.CartItem, error)
	ClearCart(ctx context.Context, cartID uuid.UUID) error
	// AddCouponCode/RemoveCouponCode persist the promo codes attached to a cart. Both are
	// idempotent (add de-dupes, remove is a no-op for an absent code) and return the
	// refreshed cart so the caller can re-price immediately.
	AddCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error)
	RemoveCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error)
}

type postgresCartRepository struct {
	db DBPool
}

func NewPostgresCartRepository(db *pgxpool.Pool) CartRepository {
	return &postgresCartRepository{db: db}
}

func (r *postgresCartRepository) GetOrCreateCartByUserID(ctx context.Context, userID uuid.UUID) (*models.Cart, error) {
	// Upsert prevents a race condition where two concurrent requests for the same
	// user both see no cart and attempt to INSERT, causing a unique-constraint violation.
	query := `
		INSERT INTO carts (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
		RETURNING id, user_id, applied_coupon_codes, created_at, updated_at
	`
	cart := &models.Cart{}
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&cart.ID, &cart.UserID, &cart.AppliedCouponCodes, &cart.CreatedAt, &cart.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	// Every cart owns a transparent default delivery so the single-shipment case needs no
	// client action and AddItem always has a delivery to attach lines to.
	if err := r.ensureDefaultDelivery(ctx, cart.ID); err != nil {
		return nil, err
	}
	return cart, nil
}

// ensureDefaultDelivery guarantees the cart has its transparent single-shipment delivery.
// Idempotent via the UNIQUE (cart_id, code) partial index: concurrent GetOrCreate calls for
// the same user race safely and the loser's insert is a no-op.
func (r *postgresCartRepository) ensureDefaultDelivery(ctx context.Context, cartID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO deliveries (cart_id, code, location_type)
		VALUES ($1, $2, 'address')
		ON CONFLICT (cart_id, code) WHERE cart_id IS NOT NULL DO NOTHING
	`, cartID, models.DefaultDeliveryCode)
	return err
}

func (r *postgresCartRepository) AddCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	// Append only if absent so the same code attached twice stays a single entry - the
	// CASE keeps the write idempotent without a read-modify-write round trip.
	query := `
		UPDATE carts
		SET applied_coupon_codes = CASE
			WHEN $2 = ANY(applied_coupon_codes) THEN applied_coupon_codes
			ELSE array_append(applied_coupon_codes, $2)
		END,
		updated_at = NOW()
		WHERE id = $1
		RETURNING id, user_id, applied_coupon_codes, created_at, updated_at
	`
	return r.scanCart(ctx, query, cartID, code)
}

func (r *postgresCartRepository) RemoveCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	// array_remove is a no-op when the code is absent, so removing an unattached code
	// still returns the (unchanged) cart rather than erroring.
	query := `
		UPDATE carts
		SET applied_coupon_codes = array_remove(applied_coupon_codes, $2), updated_at = NOW()
		WHERE id = $1
		RETURNING id, user_id, applied_coupon_codes, created_at, updated_at
	`
	return r.scanCart(ctx, query, cartID, code)
}

func (r *postgresCartRepository) scanCart(ctx context.Context, query string, args ...any) (*models.Cart, error) {
	cart := &models.Cart{}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&cart.ID, &cart.UserID, &cart.AppliedCouponCodes, &cart.CreatedAt, &cart.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cart, nil
}

func (r *postgresCartRepository) GetCartItems(ctx context.Context, cartID uuid.UUID) ([]models.CartItem, error) {
	query := `
		SELECT id, cart_id, product_id, variant_id, delivery_id, quantity, price_cents, created_at, updated_at
		FROM cart_items
		WHERE cart_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.CartItem
	for rows.Next() {
		var item models.CartItem
		if err := rows.Scan(
			&item.ID, &item.CartID, &item.ProductID, &item.VariantID, &item.DeliveryID,
			&item.Quantity, &item.PriceCents, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *postgresCartRepository) AddItem(ctx context.Context, cartID, productID, variantID uuid.UUID, quantity int, priceCents int64) (*models.CartItem, error) {
	// Upsert keyed on (cart_id, variant_id): adding the same variant again bumps quantity.
	// The subquery attaches the line to the cart's default delivery when the client does not
	// choose one - the transparent single-shipment path (GetOrCreateCartByUserID guarantees
	// the default delivery exists before any AddItem runs).
	query := `
		INSERT INTO cart_items (cart_id, product_id, variant_id, delivery_id, quantity, price_cents)
		VALUES ($1, $2, $3, (SELECT id FROM deliveries WHERE cart_id = $1 AND code = 'default'), $4, $5)
		ON CONFLICT (cart_id, variant_id)
		DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, updated_at = NOW()
		RETURNING id, cart_id, product_id, variant_id, delivery_id, quantity, price_cents, created_at, updated_at
	`
	item := &models.CartItem{}
	err := r.db.QueryRow(ctx, query, cartID, productID, variantID, quantity, priceCents).Scan(
		&item.ID, &item.CartID, &item.ProductID, &item.VariantID, &item.DeliveryID,
		&item.Quantity, &item.PriceCents, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *postgresCartRepository) UpdateItemQuantity(ctx context.Context, cartID, variantID uuid.UUID, quantity int) (*models.CartItem, error) {
	query := `
		UPDATE cart_items
		SET quantity = $1, updated_at = NOW()
		WHERE cart_id = $2 AND variant_id = $3
		RETURNING id, cart_id, product_id, variant_id, delivery_id, quantity, price_cents, created_at, updated_at
	`
	item := &models.CartItem{}
	err := r.db.QueryRow(ctx, query, quantity, cartID, variantID).Scan(
		&item.ID, &item.CartID, &item.ProductID, &item.VariantID, &item.DeliveryID,
		&item.Quantity, &item.PriceCents, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotInCart
		}
		return nil, err
	}
	return item, nil
}

func (r *postgresCartRepository) RemoveItem(ctx context.Context, cartID, variantID uuid.UUID) error {
	query := `DELETE FROM cart_items WHERE cart_id = $1 AND variant_id = $2`
	result, err := r.db.Exec(ctx, query, cartID, variantID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrProductNotInCart
	}
	return nil
}

func (r *postgresCartRepository) FindCartItem(ctx context.Context, cartID, variantID uuid.UUID) (*models.CartItem, error) {
	query := `
		SELECT id, cart_id, product_id, variant_id, delivery_id, quantity, price_cents, created_at, updated_at
		FROM cart_items
		WHERE cart_id = $1 AND variant_id = $2
	`
	item := &models.CartItem{}
	err := r.db.QueryRow(ctx, query, cartID, variantID).Scan(
		&item.ID, &item.CartID, &item.ProductID, &item.VariantID, &item.DeliveryID,
		&item.Quantity, &item.PriceCents, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotInCart
		}
		return nil, err
	}
	return item, nil
}

func (r *postgresCartRepository) ClearCart(ctx context.Context, cartID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cartID)
	return err
}
