package ai

import (
	"context"

	"github.com/google/uuid"
)

// userCtxKey is the private context key under which the authenticated user id
// travels from the HTTP layer down to the order tool. WHY here (package ai) and
// not in package tools: handler.go (package ai) sets it and the tools package
// reads it, and tools already imports ai — putting the key here avoids an import
// cycle while keeping a single source of truth for user scoping.
type userCtxKey struct{}

// WithUserID returns a context carrying the authenticated user id. The handler
// derives the id from the validated JWT and injects it here; tools never trust a
// user id supplied by the model, only this context value.
func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userCtxKey{}, id)
}

// UserIDFrom extracts the authenticated user id. ok is false when no id was
// injected, which the order tool treats as "not found" rather than leaking data.
func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userCtxKey{}).(uuid.UUID)
	return id, ok
}
