package models

// PaymentSelection is the set of charges chosen to settle a single order. WHY a wrapper
// (not a bare []Charge): an order total may be split across a main payment plus gift card
// and loyalty charges, and callers need one value that knows how to sum itself.
type PaymentSelection struct {
	Charges []Charge
}

// TotalCents sums every charge in the selection (e.g. main + giftcard + loyalty).
func (s PaymentSelection) TotalCents() int64 {
	var total int64
	for _, c := range s.Charges {
		total += c.AmountCents
	}
	return total
}
