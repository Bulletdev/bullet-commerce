package coupons

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBPool is the subset of pgxpool.Pool the coupon repository needs. Kept as an interface
// so pgxmock can stand in for the real pool in tests.
type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ErrCouponNotFound is returned by FindByCode when no coupon carries the given code, so
// the promotions handler can distinguish "unknown code" from a real query failure.
var ErrCouponNotFound = errors.New("coupon not found")

type CouponRepository interface {
	FindByCode(ctx context.Context, code string) (*models.Coupon, error)
	// IncrementUse bumps used_count by one. Called once per redemption at order creation
	// so max_uses can be enforced across shoppers.
	IncrementUse(ctx context.Context, id uuid.UUID) error
}

type postgresCouponRepository struct {
	db DBPool
}

func NewPostgresCouponRepository(db *pgxpool.Pool) CouponRepository {
	return &postgresCouponRepository{db: db}
}

func (r *postgresCouponRepository) FindByCode(ctx context.Context, code string) (*models.Coupon, error) {
	c := &models.Coupon{}
	err := r.db.QueryRow(ctx, `
		SELECT id, code, discount_type, value, min_cart_cents, max_uses, used_count, expires_at, active, created_at
		FROM coupons
		WHERE code = $1
	`, code).Scan(
		&c.ID, &c.Code, &c.DiscountType, &c.Value, &c.MinCartCents,
		&c.MaxUses, &c.UsedCount, &c.ExpiresAt, &c.Active, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCouponNotFound
		}
		return nil, err
	}
	return c, nil
}

func (r *postgresCouponRepository) IncrementUse(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE coupons SET used_count = used_count + 1 WHERE id = $1`, id)
	return err
}
