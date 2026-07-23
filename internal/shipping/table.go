package shipping

import (
	"context"
	"strings"
)

// Rule maps an inclusive CEP prefix range to a fixed cost and estimate.
//
// WHY prefix range: Brazilian CEPs are geographically ordered by their leading
// digits, so a [from,to] range over the normalized 8-digit number selects a
// region without a full CEP database. Prefixes may be shorter than 8 digits;
// they are padded (From with '0', To with '9') to bound the range - e.g.
// From "01" To "19" covers 01000000..19999999.
type Rule struct {
	CEPPrefixFrom string
	CEPPrefixTo   string
	CostCents     int64
	EstimatedDays int
	// Method optionally names this rule in the resulting Quote. When empty the
	// provider falls back to "table".
	Method string
}

// TableProvider is the MVP Provider: a static ordered list of region rules.
//
// WHY extensibility note: free-shipping-above-threshold is intentionally NOT
// implemented here (out of scope), but SubtotalCents already flows in via
// QuoteRequest so a rule/decorator can add it later without an interface change.
type TableProvider struct {
	// senderCEP is the origin; unused by the flat table today but kept so a
	// future distance-based table can compute origin->dest without a signature
	// change.
	senderCEP string
	rules     []Rule
}

// NewTableProvider builds a TableProvider from the origin CEP and its rules.
func NewTableProvider(senderCEP string, rules []Rule) *TableProvider {
	return &TableProvider{senderCEP: senderCEP, rules: rules}
}

// Quote implements Provider. It normalizes and validates the destination CEP,
// then returns the first matching rule. WHY first-match: rules are treated as an
// ordered priority list, letting a fork put specific overrides before broad
// regional ranges.
func (p *TableProvider) Quote(_ context.Context, req QuoteRequest) (Quote, error) {
	cep, ok := normalizeCEP(req.DestCEP)
	if !ok {
		return Quote{}, ErrInvalidCEP
	}

	for _, r := range p.rules {
		if cepInRange(cep, r.CEPPrefixFrom, r.CEPPrefixTo) {
			method := r.Method
			if method == "" {
				method = "table"
			}
			return Quote{
				CostCents:     r.CostCents,
				EstimatedDays: r.EstimatedDays,
				Method:        method,
			}, nil
		}
	}

	return Quote{}, ErrDestinationUnavailable
}

// normalizeCEP strips an optional single hyphen and validates 8 numeric digits.
// Returns the clean 8-digit string and whether it is valid.
func normalizeCEP(raw string) (string, bool) {
	clean := strings.ReplaceAll(strings.TrimSpace(raw), "-", "")
	if len(clean) != 8 {
		return "", false
	}
	for i := 0; i < len(clean); i++ {
		if clean[i] < '0' || clean[i] > '9' {
			return "", false
		}
	}
	return clean, true
}

// cepInRange reports whether an already-normalized 8-digit cep falls within the
// inclusive prefix range. Bounds are padded to 8 digits so shorter prefixes
// expand to their widest span (from -> low end, to -> high end).
func cepInRange(cep, from, to string) bool {
	lo := padPrefix(from, '0')
	hi := padPrefix(to, '9')
	if lo == "" || hi == "" {
		return false
	}
	// WHY string comparison: both operands are fixed 8-char zero-padded numeric
	// strings, so lexical order equals numeric order.
	return cep >= lo && cep <= hi
}

// padPrefix right-pads a numeric prefix to 8 chars with fill. Returns "" if the
// prefix is not purely numeric or longer than 8 digits (misconfigured rule).
func padPrefix(prefix string, fill byte) string {
	if len(prefix) > 8 {
		return ""
	}
	for i := 0; i < len(prefix); i++ {
		if prefix[i] < '0' || prefix[i] > '9' {
			return ""
		}
	}
	return prefix + strings.Repeat(string(fill), 8-len(prefix))
}

// DefaultBrazilRules returns plausible placeholder rules by macro-region.
//
// WHY placeholder: all costs/estimates are hardcoded MVP values, NOT real
// carrier tariffs. Sudeste (origin region) is cheapest/fastest; Norte and
// Nordeste are the most expensive/slowest, matching real-world distance from a
// Sudeste sender. Replace this whole function when wiring a real carrier.
//
// CEP first-2-digit ranges by region (IBGE/Correios geographic ordering):
//   - Sudeste:     01..39  (SP, RJ, ES, MG)
//   - Sul:         80..99  (PR, SC, RS)
//   - Centro-Oeste:70..76  (DF, GO, TO*, MT, MS)  (*TO shares 77)
//   - Nordeste:    40..65  (BA, SE, AL, PE, PB, RN, CE, PI, MA)
//   - Norte:       66..69 + 77..78 (PA, AM, AC, RR, AP, RO, TO)
func DefaultBrazilRules() []Rule {
	return []Rule{
		{CEPPrefixFrom: "01", CEPPrefixTo: "39", CostCents: 1500, EstimatedDays: 3, Method: "table-sudeste"},
		{CEPPrefixFrom: "40", CEPPrefixTo: "65", CostCents: 3500, EstimatedDays: 9, Method: "table-nordeste"},
		{CEPPrefixFrom: "66", CEPPrefixTo: "69", CostCents: 4500, EstimatedDays: 12, Method: "table-norte"},
		{CEPPrefixFrom: "70", CEPPrefixTo: "76", CostCents: 2800, EstimatedDays: 7, Method: "table-centro-oeste"},
		{CEPPrefixFrom: "77", CEPPrefixTo: "78", CostCents: 4500, EstimatedDays: 12, Method: "table-norte"},
		{CEPPrefixFrom: "79", CEPPrefixTo: "79", CostCents: 2800, EstimatedDays: 7, Method: "table-centro-oeste"},
		{CEPPrefixFrom: "80", CEPPrefixTo: "99", CostCents: 2200, EstimatedDays: 5, Method: "table-sul"},
	}
}
