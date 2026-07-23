-- Idempotency keys for POST /api/orders. A key is scoped to the user who sent it,
-- so two different users may present the same client-generated key without collision.
-- The row is first inserted "in-flight" (response_status NULL) to reserve the key,
-- then finalized with the stored response so later replays are served without
-- creating a second order or reserving stock again.
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key             TEXT NOT NULL,
    user_id         UUID NOT NULL,
    request_hash    TEXT,
    response_status INT,
    response_body   JSONB,
    order_id        UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, key)
);
