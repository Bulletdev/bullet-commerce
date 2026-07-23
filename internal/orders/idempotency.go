package orders

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IdempotencyRecord is a stored (or in-flight) result of a keyed POST /api/orders.
// A row exists in two phases: reserved (Completed == false, no response yet) and
// finalized (Completed == true, ResponseStatus/ResponseBody populated). Replaying a
// finalized record returns the original response verbatim; encountering a reserved
// record means an identical request is still being processed.
type IdempotencyRecord struct {
	Key            string
	RequestHash    string
	ResponseStatus int
	ResponseBody   []byte
	OrderID        *uuid.UUID
	Completed      bool
}

// LookupIdempotencyKey returns the record for (userID, key), or nil when the key was
// never seen. A returned record may still be in-flight (Completed == false).
func (r *postgresOrderRepository) LookupIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*IdempotencyRecord, error) {
	var rec IdempotencyRecord
	var hash *string
	var status *int
	var body []byte
	err := r.db.QueryRow(ctx, `
		SELECT request_hash, response_status, response_body, order_id
		FROM idempotency_keys
		WHERE user_id = $1 AND key = $2
	`, userID, key).Scan(&hash, &status, &body, &rec.OrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec.Key = key
	if hash != nil {
		rec.RequestHash = *hash
	}
	if status != nil {
		rec.ResponseStatus = *status
		rec.ResponseBody = body
		rec.Completed = true
	}
	return &rec, nil
}

// ClaimIdempotencyKey atomically reserves the key by inserting an in-flight row. It
// reports isNew == true only when THIS caller won the insert - the signal to proceed
// with order creation. A false return means a concurrent or prior request already owns
// the key, so the caller must Lookup and either replay (finalized) or 409 (in-flight).
// WHY the INSERT gate (not just Lookup): two concurrent Lookups both miss, so without an
// atomic claim a double-click would create two orders and reserve stock twice. The
// (user_id, key) primary key makes the ON CONFLICT the single serialization point.
func (r *postgresOrderRepository) ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, requestHash string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, request_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, key) DO NOTHING
	`, userID, key, requestHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// SaveIdempotencyKey finalizes the reserved row with the response so later replays are
// served without re-creating the order. It targets the row this caller claimed.
func (r *postgresOrderRepository) SaveIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, status int, body []byte, orderID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE idempotency_keys
		SET response_status = $3, response_body = $4, order_id = $5
		WHERE user_id = $1 AND key = $2
	`, userID, key, status, body, orderID)
	return err
}

// ReleaseIdempotencyKey drops a still-in-flight reservation so a retry may proceed. It is
// called only when order creation FAILED (the tx rolled back, so no stock was reserved) -
// caching a failed attempt would wrongly block the user from retrying. A finalized row is
// never released (only in-flight ones are deleted).
func (r *postgresOrderRepository) ReleaseIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM idempotency_keys
		WHERE user_id = $1 AND key = $2 AND response_status IS NULL
	`, userID, key)
	return err
}
