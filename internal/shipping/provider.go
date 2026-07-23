// Package shipping is the domain of freight/shipping quoting.
//
// WHY genericity: quoting is behind the Provider interface so a fork can swap
// the implementation (fixed region table -> Correios/MelhorEnvio) without
// touching callers (handlers/services). Callers depend only on QuoteRequest,
// Quote and Provider - never on a concrete implementation.
//
// This is a pure domain service: no HTTP, no DB, no framework. Money is always
// int64 in cents. Errors are domain errors that do not leak implementation
// details (no CEP-range internals, no provider vendor errors).
//
// ACCEPTANCE CRITERIA (covered by table_test.go):
//   - Given a valid destination CEP in the Sudeste region, When Quote, Then it
//     returns the cost and estimated days of the matching range.
//   - Given a well-formed 8-digit CEP outside every range, When Quote, Then
//     ErrDestinationUnavailable.
//   - Given a malformed CEP ("123", "abcdefgh"), When Quote, Then ErrInvalidCEP.
//   - Given a CEP with a hyphen ("01310-100"), When Quote, Then it normalizes
//     and quotes normally.
//   - The Provider interface allows plugging a future Correios implementation
//     without changing QuoteRequest/Quote.
package shipping

import (
	"context"
	"errors"
)

// Domain errors. WHY sentinels: callers compare with errors.Is and map to their
// own transport concerns (e.g. HTTP status) without knowing the implementation.
var (
	// ErrInvalidCEP means the destination CEP is not 8 numeric digits (after
	// normalizing an optional hyphen).
	ErrInvalidCEP = errors.New("shipping: invalid CEP")
	// ErrDestinationUnavailable means the CEP is well-formed but no provider
	// rule covers it, so no quote can be produced.
	ErrDestinationUnavailable = errors.New("shipping: destination unavailable")
)

// QuoteRequest is the input of a quote. WHY a stable shape: keeping this struct
// implementation-agnostic is what lets a future Correios provider reuse it
// unchanged. Weight is in grams, subtotal in cents.
type QuoteRequest struct {
	DestCEP          string
	TotalWeightGrams int
	SubtotalCents    int64
}

// Quote is the output of a quote. CostCents is int64 cents; Method names the
// provider/rule that produced it.
type Quote struct {
	CostCents     int64
	EstimatedDays int
	Method        string
}

// Provider quotes shipping for a request. WHY interface: the single seam a fork
// re-implements. Any provider (table, Correios, MelhorEnvio) satisfies it.
type Provider interface {
	Quote(ctx context.Context, req QuoteRequest) (Quote, error)
}
