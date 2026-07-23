package money

// fraction maps an ISO-4217 currency code to its number of minor-unit digits
// (the "exponent"). WHY a table instead of assuming 2: currencies do not share a
// scale - JPY has no minor unit (0), most fiat use 2, and Bahraini dinar uses 3.
// Getting this wrong corrupts every formatted amount and every rounding split,
// so precision is data-driven per currency rather than hard-coded to cents.
var fraction = map[string]int{
	"BRL": 2,
	"USD": 2,
	"JPY": 0,
	"BHD": 3,
}

// defaultFraction is used for any currency absent from the table. WHY 2: the vast
// majority of world currencies use two decimal places, so it is the safest fallback
// and keeps an unknown code from silently formatting as whole units.
const defaultFraction = 2

// precision returns the number of minor-unit digits for cur, falling back to
// defaultFraction for unknown codes.
func precision(cur string) int {
	if f, ok := fraction[cur]; ok {
		return f
	}
	return defaultFraction
}

// pow10 returns 10^n as int64. WHY a tiny local helper: n is a currency exponent
// (0..3 in practice) so a loop is exact and avoids pulling float math from the
// stdlib, which would reintroduce the rounding errors int64 cents exist to prevent.
func pow10(n int) int64 {
	r := int64(1)
	for i := 0; i < n; i++ {
		r *= 10
	}
	return r
}
