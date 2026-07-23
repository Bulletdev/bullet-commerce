package charges

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBPool is the subset of pgxpool.Pool the charge repository needs. Kept as an interface
// so pgxmock can stand in for the real pool in tests.
type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type ChargeRepository interface {
	Create(ctx context.Context, charge *models.Charge) error
	FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.Charge, error)
}

type postgresChargeRepository struct {
	db DBPool
}

func NewPostgresChargeRepository(db *pgxpool.Pool) ChargeRepository {
	return &postgresChargeRepository{db: db}
}

func (r *postgresChargeRepository) Create(ctx context.Context, charge *models.Charge) error {
	// RETURNING id/created_at so the DB-generated identity and timestamp flow back into
	// the caller's struct without a second read.
	return r.db.QueryRow(ctx, `
		INSERT INTO payment_charges (order_id, type, method, amount_cents, reference, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, charge.OrderID, charge.Type, charge.Method, charge.AmountCents, charge.Reference, charge.Status,
	).Scan(&charge.ID, &charge.CreatedAt)
}

func (r *postgresChargeRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.Charge, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, order_id, type, method, amount_cents, reference, status, created_at
		FROM payment_charges
		WHERE order_id = $1
		ORDER BY created_at ASC
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var charges []models.Charge
	for rows.Next() {
		var c models.Charge
		if err := rows.Scan(
			&c.ID, &c.OrderID, &c.Type, &c.Method, &c.AmountCents,
			&c.Reference, &c.Status, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		charges = append(charges, c)
	}
	return charges, rows.Err()
}
