package events

import (
	"context"
	"log/slog"
	"sync"
)

// Handler reacts to a published Event. Handlers run synchronously inside
// Publish, in subscription order.
type Handler func(ctx context.Context, e Event)

// Bus is an in-process publish/subscribe dispatcher.
//
// WHY the caller owns the transaction: Publish does NOT open, join, or commit
// any DB transaction. It is meant to be called by the caller AFTER the relevant
// transaction has already committed, so that handlers only ever observe durable
// facts. If a handler needs its own persistence, it must manage its own tx.
type Bus interface {
	Subscribe(name string, h Handler)
	Publish(ctx context.Context, e Event)
}

type inProcessBus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewInProcessBus returns a Bus that dispatches events synchronously within the
// calling goroutine.
func NewInProcessBus() Bus {
	return &inProcessBus{handlers: make(map[string][]Handler)}
}

func (b *inProcessBus) Subscribe(name string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = append(b.handlers[name], h)
}

// Publish invokes every handler registered for the event's name. Publishing an
// event with no subscribers is a silent no-op.
func (b *inProcessBus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	// Copy the slice so a handler that (un)subscribes cannot mutate the list
	// mid-dispatch, and so we do not hold the lock while running handlers.
	hs := make([]Handler, len(b.handlers[e.Name()]))
	copy(hs, b.handlers[e.Name()])
	b.mu.RUnlock()

	for _, h := range hs {
		b.dispatch(ctx, h, e)
	}
}

// dispatch runs a single handler with panic isolation. WHY recover: one
// misbehaving subscriber must not abort the remaining handlers nor propagate a
// panic up into the caller that published the (already-committed) event.
func (b *inProcessBus) dispatch(ctx context.Context, h Handler, e Event) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("event handler panicked", "event", e.Name(), "panic", r)
		}
	}()
	h(ctx, e)
}
