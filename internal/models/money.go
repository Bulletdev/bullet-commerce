package models

// DefaultCurrency is the ISO-4217 code applied when a request omits currency.
// A fork serving a non-BRL market changes this single constant (and the DEFAULT
// in migration 000009) - no other code references a hard-coded currency.
const DefaultCurrency = "BRL"

// Money amounts are stored and transported as int64 minor units (cents).
// This keeps arithmetic exact and matches payment-provider APIs; never use float
// for money. Formatting to a decimal string is a presentation concern (frontend).
