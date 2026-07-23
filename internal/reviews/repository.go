package reviews

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// reviewColumns is the canonical SELECT/scan order shared by the read queries so the column
// list and the scan destinations never drift apart.
const reviewColumns = `id, product_id, user_id, rating, title, body, status, created_at, updated_at`

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	ErrReviewNotFound = errors.New("review not found")
	// ErrDuplicateReview maps the (product_id, user_id) unique violation: a user may review a
	// product only once.
	ErrDuplicateReview = errors.New("user has already reviewed this product")
)

type ReviewRepository interface {
	// Create inserts a review. A second review by the same user for the same product maps the
	// unique violation to ErrDuplicateReview. Generated fields (id/status/timestamps) are
	// written back onto the passed review.
	Create(ctx context.Context, review *Review) error
	// ListByProduct returns a product's reviews newest-first, paginated. When onlyApproved is
	// true (the public view) rejected/pending reviews are excluded.
	ListByProduct(ctx context.Context, productID uuid.UUID, onlyApproved bool, limit, offset int) ([]Review, error)
	// RecomputeAggregate refreshes products.rating_avg / rating_count from the approved reviews
	// of the given product. Callers invoke it after any change to the approved set.
	RecomputeAggregate(ctx context.Context, productID uuid.UUID) error
	// Moderate sets a review's status and returns the affected product_id so the caller can
	// recompute that product's aggregate. Returns ErrReviewNotFound when no row matches.
	Moderate(ctx context.Context, reviewID uuid.UUID, status string) (uuid.UUID, error)
}

type postgresReviewRepository struct {
	db DBPool
}

func NewPostgresReviewRepository(db *pgxpool.Pool) ReviewRepository {
	return &postgresReviewRepository{db: db}
}

// normalizeReview mirrors the column defaults so an explicitly-passed zero value still
// satisfies the DB CHECK/NOT NULL constraints.
func normalizeReview(review *Review) {
	if review.Status == "" {
		review.Status = ReviewStatusApproved
	}
}

func (r *postgresReviewRepository) Create(ctx context.Context, review *Review) error {
	normalizeReview(review)
	query := `
		INSERT INTO product_reviews (product_id, user_id, rating, title, body, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, status, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		review.ProductID, review.UserID, review.Rating, review.Title, review.Body, review.Status,
	).Scan(&review.ID, &review.Status, &review.CreatedAt, &review.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrDuplicateReview
		}
		return err
	}
	return nil
}

func (r *postgresReviewRepository) ListByProduct(ctx context.Context, productID uuid.UUID, onlyApproved bool, limit, offset int) ([]Review, error) {
	query := `SELECT ` + reviewColumns + ` FROM product_reviews WHERE product_id = $1`
	if onlyApproved {
		query += ` AND status = 'approved'`
	}
	query += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, productID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Review
	for rows.Next() {
		var rv Review
		if err := rows.Scan(
			&rv.ID, &rv.ProductID, &rv.UserID, &rv.Rating, &rv.Title, &rv.Body,
			&rv.Status, &rv.CreatedAt, &rv.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, rv)
	}
	return list, rows.Err()
}

func (r *postgresReviewRepository) RecomputeAggregate(ctx context.Context, productID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE products
		SET rating_avg = (SELECT ROUND(AVG(rating)::numeric, 2) FROM product_reviews WHERE product_id = $1 AND status = 'approved'),
			rating_count = (SELECT COUNT(*) FROM product_reviews WHERE product_id = $1 AND status = 'approved')
		WHERE id = $1
	`, productID)
	return err
}

func (r *postgresReviewRepository) Moderate(ctx context.Context, reviewID uuid.UUID, status string) (uuid.UUID, error) {
	var productID uuid.UUID
	err := r.db.QueryRow(ctx, `
		UPDATE product_reviews
		SET status = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING product_id
	`, status, reviewID).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrReviewNotFound
		}
		return uuid.Nil, err
	}
	return productID, nil
}
