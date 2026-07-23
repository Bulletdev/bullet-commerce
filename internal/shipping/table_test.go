package shipping

import (
	"context"
	"errors"
	"testing"
)

// compile-time proof the acceptance criterion "TableProvider satisfies Provider"
// holds; a future Correios impl would add its own line here.
var _ Provider = (*TableProvider)(nil)

func newDefaultProvider() *TableProvider {
	return NewTableProvider("01001000", DefaultBrazilRules())
}

// Criterion: valid Sudeste CEP -> cost and days of the matching range.
func TestQuote_SudesteReturnsRangeCostAndDays(t *testing.T) {
	p := newDefaultProvider()

	q, err := p.Quote(context.Background(), QuoteRequest{
		DestCEP:          "01310100", // Av. Paulista, SP (Sudeste)
		TotalWeightGrams: 500,
		SubtotalCents:    10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.CostCents != 1500 {
		t.Errorf("CostCents = %d, want 1500", q.CostCents)
	}
	if q.EstimatedDays != 3 {
		t.Errorf("EstimatedDays = %d, want 3", q.EstimatedDays)
	}
	if q.Method != "table-sudeste" {
		t.Errorf("Method = %q, want %q", q.Method, "table-sudeste")
	}
}

// Criterion: well-formed 8-digit CEP outside every range -> ErrDestinationUnavailable.
func TestQuote_OutOfRangeReturnsDestinationUnavailable(t *testing.T) {
	// A table with a single narrow rule; "05000000" is well-formed but uncovered.
	p := NewTableProvider("01001000", []Rule{
		{CEPPrefixFrom: "10", CEPPrefixTo: "19", CostCents: 1000, EstimatedDays: 2},
	})

	_, err := p.Quote(context.Background(), QuoteRequest{DestCEP: "05000000"})
	if !errors.Is(err, ErrDestinationUnavailable) {
		t.Fatalf("err = %v, want ErrDestinationUnavailable", err)
	}
}

// Criterion: malformed CEP -> ErrInvalidCEP.
func TestQuote_MalformedReturnsInvalidCEP(t *testing.T) {
	p := newDefaultProvider()

	cases := []string{
		"123",        // too short
		"abcdefgh",   // 8 non-numeric
		"",           // empty
		"0131010",    // 7 digits
		"013101000",  // 9 digits
		"01310-10a",  // hyphen normalizes to 8 but non-numeric remains
		"12 34 5678", // spaces are not stripped
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := p.Quote(context.Background(), QuoteRequest{DestCEP: c})
			if !errors.Is(err, ErrInvalidCEP) {
				t.Fatalf("DestCEP=%q err = %v, want ErrInvalidCEP", c, err)
			}
		})
	}
}

// Criterion: hyphenated CEP normalizes and quotes normally.
func TestQuote_HyphenNormalizes(t *testing.T) {
	p := newDefaultProvider()

	withHyphen, err := p.Quote(context.Background(), QuoteRequest{DestCEP: "01310-100"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	without, err := p.Quote(context.Background(), QuoteRequest{DestCEP: "01310100"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withHyphen != without {
		t.Errorf("hyphen quote %+v != plain quote %+v", withHyphen, without)
	}
}

// Region table: each macro-region resolves to its documented placeholder rule.
func TestQuote_RegionTable(t *testing.T) {
	p := newDefaultProvider()

	cases := []struct {
		name       string
		cep        string
		wantCost   int64
		wantDays   int
		wantMethod string
	}{
		{"sudeste-sp-low", "01001000", 1500, 3, "table-sudeste"},
		{"sudeste-mg-high", "39999999", 1500, 3, "table-sudeste"},
		{"nordeste-ba", "40000000", 3500, 9, "table-nordeste"},
		{"nordeste-ma-high", "65999999", 3500, 9, "table-nordeste"},
		{"norte-pa", "66000000", 4500, 12, "table-norte"},
		{"norte-to", "77000000", 4500, 12, "table-norte"},
		{"centro-oeste-df", "70000000", 2800, 7, "table-centro-oeste"},
		{"centro-oeste-ms", "79000000", 2800, 7, "table-centro-oeste"},
		{"sul-pr", "80000000", 2200, 5, "table-sul"},
		{"sul-rs-high", "99999999", 2200, 5, "table-sul"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := p.Quote(context.Background(), QuoteRequest{DestCEP: c.cep})
			if err != nil {
				t.Fatalf("cep=%s unexpected error: %v", c.cep, err)
			}
			if q.CostCents != c.wantCost || q.EstimatedDays != c.wantDays || q.Method != c.wantMethod {
				t.Errorf("cep=%s got {cost=%d days=%d method=%q}, want {cost=%d days=%d method=%q}",
					c.cep, q.CostCents, q.EstimatedDays, q.Method, c.wantCost, c.wantDays, c.wantMethod)
			}
		})
	}
}

// First-match precedence: an override rule placed before a broad range wins.
func TestQuote_FirstMatchPrecedence(t *testing.T) {
	p := NewTableProvider("01001000", []Rule{
		{CEPPrefixFrom: "01310", CEPPrefixTo: "01310", CostCents: 500, EstimatedDays: 1, Method: "override"},
		{CEPPrefixFrom: "01", CEPPrefixTo: "39", CostCents: 1500, EstimatedDays: 3, Method: "table-sudeste"},
	})

	q, err := p.Quote(context.Background(), QuoteRequest{DestCEP: "01310100"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Method != "override" || q.CostCents != 500 {
		t.Errorf("got %+v, want override rule (cost 500)", q)
	}
}

// A rule without an explicit Method falls back to the generic "table" label.
func TestQuote_DefaultMethodLabel(t *testing.T) {
	p := NewTableProvider("01001000", []Rule{
		{CEPPrefixFrom: "01", CEPPrefixTo: "09", CostCents: 1000, EstimatedDays: 2},
	})

	q, err := p.Quote(context.Background(), QuoteRequest{DestCEP: "01000000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Method != "table" {
		t.Errorf("Method = %q, want fallback %q", q.Method, "table")
	}
}
