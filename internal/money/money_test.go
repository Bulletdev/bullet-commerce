package money

import (
	"errors"
	"testing"
)

func sum(parts []Money) int64 {
	var t int64
	for _, p := range parts {
		t += p.Cents
	}
	return t
}

// Given two Money values in different currencies
// When Add is called
// Then it returns ErrCurrencyMismatch.
func TestAdd_CurrencyMismatch(t *testing.T) {
	_, err := New(100, "BRL").Add(New(100, "USD"))
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

func TestAdd_SameCurrency(t *testing.T) {
	got, err := New(100, "BRL").Add(New(45, "BRL"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Cents != 145 || got.Currency != "BRL" {
		t.Fatalf("want 145 BRL, got %d %s", got.Cents, got.Currency)
	}
}

func TestSub_CurrencyMismatch(t *testing.T) {
	_, err := New(100, "BRL").Sub(New(100, "USD"))
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

func TestSub_SameCurrency(t *testing.T) {
	got, err := New(100, "BRL").Sub(New(45, "BRL"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Cents != 55 {
		t.Fatalf("want 55, got %d", got.Cents)
	}
}

// Given 990 cents and quantity 3
// When MulQty(3) is called
// Then the result is 2970.
func TestMulQty(t *testing.T) {
	got := New(990, "BRL").MulQty(3)
	if got.Cents != 2970 {
		t.Fatalf("want 2970, got %d", got.Cents)
	}
	if got.Currency != "BRL" {
		t.Fatalf("want BRL, got %s", got.Currency)
	}
}

// Given 1245 cents split into 6 payables
// When SplitInPayables(6) is called
// Then the parts sum exactly to 1245.
func TestSplitInPayables_SumExact(t *testing.T) {
	m := New(1245, "BRL")
	parts := m.SplitInPayables(6)
	if len(parts) != 6 {
		t.Fatalf("want 6 parts, got %d", len(parts))
	}
	if got := sum(parts); got != 1245 {
		t.Fatalf("parts must sum to 1245, got %d", got)
	}
	// 1245 = 5*208 + 1*205 ; remainder 3 spread across first 3 parts.
	want := []int64{208, 208, 208, 207, 207, 207}
	for i, p := range parts {
		if p.Cents != want[i] {
			t.Fatalf("part %d: want %d, got %d", i, want[i], p.Cents)
		}
	}
}

func TestSplitInPayables_EvenDivision(t *testing.T) {
	parts := New(1000, "BRL").SplitInPayables(4)
	for i, p := range parts {
		if p.Cents != 250 {
			t.Fatalf("part %d: want 250, got %d", i, p.Cents)
		}
	}
	if sum(parts) != 1000 {
		t.Fatalf("want sum 1000, got %d", sum(parts))
	}
}

func TestSplitInPayables_Negative(t *testing.T) {
	m := New(-1245, "BRL")
	parts := m.SplitInPayables(6)
	if got := sum(parts); got != -1245 {
		t.Fatalf("want sum -1245, got %d", got)
	}
}

func TestSplitInPayables_NonPositiveN(t *testing.T) {
	if got := New(100, "BRL").SplitInPayables(0); len(got) != 0 {
		t.Fatalf("want empty for n=0, got %d", len(got))
	}
	if got := New(100, "BRL").SplitInPayables(-3); len(got) != 0 {
		t.Fatalf("want empty for n<0, got %d", len(got))
	}
}

// Given 100 cents allocated by weights [3,1]
// When Allocate is called
// Then the result is [75,25] and sums to 100.
func TestAllocate_Basic(t *testing.T) {
	parts := New(100, "BRL").Allocate([]int64{3, 1})
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d", len(parts))
	}
	if parts[0].Cents != 75 || parts[1].Cents != 25 {
		t.Fatalf("want [75 25], got [%d %d]", parts[0].Cents, parts[1].Cents)
	}
	if sum(parts) != 100 {
		t.Fatalf("want sum 100, got %d", sum(parts))
	}
}

func TestAllocate_RemainderToLargestFraction(t *testing.T) {
	// 100 split by equal thirds: floors are 33 each, 1 cent leftover to first share.
	parts := New(100, "BRL").Allocate([]int64{1, 1, 1})
	if sum(parts) != 100 {
		t.Fatalf("want sum 100, got %d", sum(parts))
	}
	want := []int64{34, 33, 33}
	for i, p := range parts {
		if p.Cents != want[i] {
			t.Fatalf("part %d: want %d, got %d", i, want[i], p.Cents)
		}
	}
}

func TestAllocate_EmptyOrZeroWeights(t *testing.T) {
	if got := New(100, "BRL").Allocate(nil); len(got) != 0 {
		t.Fatalf("want empty for nil weights, got %d", len(got))
	}
	if got := New(100, "BRL").Allocate([]int64{0, 0}); len(got) != 0 {
		t.Fatalf("want empty for zero-total weights, got %d", len(got))
	}
}

func TestAllocate_PreservesCurrency(t *testing.T) {
	parts := New(100, "JPY").Allocate([]int64{1, 1})
	for _, p := range parts {
		if p.Currency != "JPY" {
			t.Fatalf("want JPY, got %s", p.Currency)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		cents int64
		cur   string
		want  string
	}{
		{1234, "BRL", "12.34"},
		{5, "USD", "0.05"},
		{500, "JPY", "500"},    // 0 fraction digits
		{1234, "BHD", "1.234"}, // 3 fraction digits
		{100, "XYZ", "1.00"},   // unknown -> default 2
		{-1234, "BRL", "-12.34"},
		{-5, "JPY", "-5"},
	}
	for _, c := range cases {
		got := New(c.cents, c.cur).Format()
		if got != c.want {
			t.Fatalf("Format(%d,%s): want %q, got %q", c.cents, c.cur, c.want, got)
		}
	}
}

func TestPrecision(t *testing.T) {
	if precision("JPY") != 0 {
		t.Fatalf("JPY precision want 0, got %d", precision("JPY"))
	}
	if precision("BHD") != 3 {
		t.Fatalf("BHD precision want 3, got %d", precision("BHD"))
	}
	if precision("ZZZ") != defaultFraction {
		t.Fatalf("unknown precision want %d, got %d", defaultFraction, precision("ZZZ"))
	}
}
