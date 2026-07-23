// Package money is a value object for monetary amounts held as int64 minor units
// (cents) plus an ISO-4217 currency code. WHY int64 minor units: float money
// accumulates rounding error and diverges from payment-provider APIs, which all
// transact in integer minor units. All arithmetic here is exact, and any operation
// that could lose a fraction of a unit (splitting, allocating) redistributes the
// remainder so the parts always sum back to the whole.
//
// # Acceptance scenarios (Given/When/Then)
//
// SplitInPayables — Given 1245 cents split into 6 payables, When SplitInPayables(6)
// is called, Then the parts sum exactly to 1245 (five parts of 208 + one of 205,
// i.e. the 3-cent remainder is spread one cent at a time across the first parts).
//
// Allocate — Given 100 cents allocated by weights [3,1], When Allocate is called,
// Then the result is [75,25] and the parts sum exactly to 100.
//
// Add — Given two Money values in different currencies, When Add is called, Then it
// returns ErrCurrencyMismatch and a zero Money.
//
// MulQty — Given 990 cents and quantity 3, When MulQty(3) is called, Then the result
// is 2970 cents in the same currency.
package money

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrCurrencyMismatch is returned by binary operations (Add, Sub) when the two
// operands carry different currency codes. WHY an error rather than a panic:
// mixing currencies is a caller/data bug the request layer can surface cleanly
// instead of crashing the process.
var ErrCurrencyMismatch = errors.New("money: currency mismatch")

// Money is an amount in integer minor units (Cents) of a given Currency.
type Money struct {
	Cents    int64
	Currency string
}

// New constructs a Money from minor units and a currency code.
func New(cents int64, cur string) Money {
	return Money{Cents: cents, Currency: cur}
}

// Add returns m+o, or ErrCurrencyMismatch if the currencies differ.
func (m Money) Add(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{Cents: m.Cents + o.Cents, Currency: m.Currency}, nil
}

// Sub returns m-o, or ErrCurrencyMismatch if the currencies differ.
func (m Money) Sub(o Money) (Money, error) {
	if m.Currency != o.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{Cents: m.Cents - o.Cents, Currency: m.Currency}, nil
}

// MulQty scales the amount by an integer quantity (e.g. line item price × qty).
// WHY int and not Money: a quantity is dimensionless, so multiplying two Money
// values would be meaningless — only scalar scaling is defined.
func (m Money) MulQty(q int) Money {
	return Money{Cents: m.Cents * int64(q), Currency: m.Currency}
}

// SplitInPayables divides the amount into n parts of as-equal-as-possible size
// whose sum is exactly the original. WHY largest-remainder distribution: integer
// division drops the remainder, so n installments of cents/n would under-collect;
// here each of the first r parts (r = cents mod n) gets one extra minor unit,
// guaranteeing the parts sum back to m.Cents with no lost or invented cents.
//
// For n <= 0 it returns an empty slice (nothing to split into).
func (m Money) SplitInPayables(n int) []Money {
	if n <= 0 {
		return []Money{}
	}
	parts := make([]Money, n)
	base := m.Cents / int64(n)
	rem := m.Cents % int64(n)
	// rem carries the sign of m.Cents in Go; treat its magnitude as the number of
	// parts that receive one extra unit, and its sign as the direction of that unit,
	// so negative totals split correctly (e.g. refunds).
	extra := int64(1)
	if rem < 0 {
		extra = -1
		rem = -rem
	}
	for i := range parts {
		c := base
		if int64(i) < rem {
			c += extra
		}
		parts[i] = Money{Cents: c, Currency: m.Currency}
	}
	return parts
}

// Allocate splits the amount proportionally to weights, preserving the total.
// WHY largest-remainder again: proportional shares are rarely whole cents, so each
// share is floored and the leftover cents (total - sum of floors) are handed out one
// at a time to the shares with the largest fractional remainders. This keeps the
// result deterministic and makes the parts sum exactly to m.Cents — the invariant
// needed when spreading an order-level discount or freight across line items.
//
// Empty or all-zero weights yield an empty slice (nothing to allocate against).
func (m Money) Allocate(weights []int64) []Money {
	if len(weights) == 0 {
		return []Money{}
	}
	var total int64
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return []Money{}
	}

	out := make([]Money, len(weights))
	// remainders tracks the dropped fractional numerator (m.Cents*w mod total) per
	// share, used to decide who receives the leftover cents.
	remainders := make([]int64, len(weights))
	var allocated int64
	for i, w := range weights {
		share := m.Cents * w / total
		remainders[i] = m.Cents*w - share*total
		out[i] = Money{Cents: share, Currency: m.Currency}
		allocated += share
	}

	leftover := m.Cents - allocated
	// Hand out one cent at a time to the largest remainder, then exclude that index
	// (set to -1) so no share receives twice in a pass. leftover is < len(weights)
	// by construction, so this terminates quickly and preserves the total exactly.
	for leftover > 0 {
		best := 0
		for i := 1; i < len(remainders); i++ {
			if remainders[i] > remainders[best] {
				best = i
			}
		}
		out[best].Cents++
		remainders[best] = -1
		leftover--
	}
	return out
}

// Format renders the amount as a plain decimal string using the currency's minor-unit
// precision (e.g. BRL 1234 -> "12.34", JPY 500 -> "500", BHD 1234 -> "1.234"). WHY no
// symbol or locale grouping: this is a canonical numeric representation; symbol,
// thousands separators and locale are a presentation concern for the frontend.
func (m Money) Format() string {
	p := precision(m.Currency)
	if p == 0 {
		return strconv.FormatInt(m.Cents, 10)
	}
	scale := pow10(p)
	sign := ""
	c := m.Cents
	if c < 0 {
		sign = "-"
		c = -c
	}
	whole := c / scale
	frac := c % scale
	return fmt.Sprintf("%s%d.%0*d", sign, whole, p, frac)
}
