// Package payment defines the provider-agnostic contract the bullet-commerce core
// uses to charge and reconcile payments. Concrete PSPs (OpenPix/ProPay, Efí,
// Itaú PIX Automático, static PIX) live in subpackages and are selected by fork
// config through the Registry - the core never imports a specific provider.
//
// A Provider is a stateless I/O adapter: it talks to the PSP and knows nothing
// about the database. Persistence and state transitions stay in the order layer.
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Money is an amount in minor units (cents). Always paired with a Currency.
type Money int64

// Currency is an ISO-4217 code ("BRL", "USD").
type Currency string

// Name identifies a provider in fork config: "openpix", "efi", "itau_pix", "pix_static".
type Name string

// ChargeStatus normalizes payment state across PSPs.
type ChargeStatus string

const (
	ChargePending  ChargeStatus = "pending"
	ChargePaid     ChargeStatus = "paid"
	ChargeExpired  ChargeStatus = "expired"
	ChargeCanceled ChargeStatus = "canceled"
	ChargeRefunded ChargeStatus = "refunded"
	ChargeFailed   ChargeStatus = "failed"
)

// Sentinel errors the core handles without knowing the PSP.
var (
	ErrInvalidSignature = errors.New("payment: invalid webhook signature")
	ErrUnsupported      = errors.New("payment: capability not supported by provider")
	ErrChargeNotFound   = errors.New("payment: charge not found")
	ErrNotConfigured    = errors.New("payment: provider not configured")
)

// CustomerRef carries payer data some PSPs require for PIX.
type CustomerRef struct {
	Name  string
	TaxID string // CPF/CNPJ, digits only
	Email string
}

// PixChargeRequest is the input to create a PIX charge.
type PixChargeRequest struct {
	// ReferenceID is the caller's natural key (order id). Providers pass it to the
	// PSP as reference/idempotency key when supported; the core upserts by it.
	ReferenceID string
	Amount      Money
	Currency    Currency
	ExpiresIn   time.Duration // QR TTL; provider maps to the PSP's accepted form
	Customer    *CustomerRef
	Description string
	Metadata    map[string]string
}

// PixCharge is a normalized PIX charge across PSPs.
type PixCharge struct {
	Provider    Name
	ProviderID  string // charge id at the PSP
	TxID        string // PIX txid (EMV)
	Status      ChargeStatus
	Amount      Money
	Currency    Currency
	QRCodeText  string // copy-and-paste (BR Code / EMV payload)
	QRCodeImage string // data-URI/base64, "" when the PSP does not provide one
	ExpiresAt   time.Time
	CreatedAt   time.Time
	Raw         json.RawMessage // PSP-specific payload
}

// RawWebhook is the framework-agnostic inbound webhook. Body must stay raw:
// signatures are verified over exact bytes before any parse.
type RawWebhook struct {
	Headers map[string]string
	Body    []byte
	Query   map[string]string
}

// EventType normalizes webhook event kinds. Unknown means "not handled" - the
// HTTP handler answers 200 and no-ops.
type EventType string

const (
	EventChargePaid     EventType = "charge.paid"
	EventChargeExpired  EventType = "charge.expired"
	EventChargeRefunded EventType = "charge.refunded"
	EventUnknown        EventType = "unknown"
)

// WebhookEvent is the verified, normalized result of a webhook.
type WebhookEvent struct {
	Provider    Name
	EventID     string // PSP event id when present - use for dedupe
	Type        EventType
	ProviderID  string
	TxID        string
	ReferenceID string
	Status      ChargeStatus
	Amount      Money
	Currency    Currency
	PaidAt      *time.Time
	Raw         json.RawMessage
}

// FlowAction is the next step the shopper-facing client must take to advance a
// charge. It decouples the core's checkout state machine from any single PSP's
// payment method: the same FlowStatus drives a PIX QR, a redirect (hosted
// checkout) or an embedded iframe without the caller branching on provider.
type FlowAction string

const (
	ActionDisplayPix FlowAction = "display_pix" // render QR + copy-and-paste, then poll
	ActionRedirect   FlowAction = "redirect"    // send the browser to ActionData["url"]
	ActionShowIframe FlowAction = "show_iframe" // embed ActionData["url"] in an iframe
	ActionNone       FlowAction = "none"        // terminal or nothing for the shopper to do
)

// FlowStatus is a snapshot of a charge as a state machine: the normalized Status
// plus the Action the client should surface now and the data that action needs
// (QR payload, redirect URL, expiry). ActionData is provider-shaped string map so
// the transport can pass it through without a typed coupling per method.
type FlowStatus struct {
	Status     ChargeStatus
	Action     FlowAction
	ActionData map[string]string
}

// Provider is the base contract every PSP adapter implements.
type Provider interface {
	Name() Name

	// CreatePixCharge creates the charge and returns qr/copy-paste/txid/expires_at.
	CreatePixCharge(ctx context.Context, req PixChargeRequest) (*PixCharge, error)

	// GetCharge fetches current state from the PSP (reconciliation / webhook fallback).
	GetCharge(ctx context.Context, providerID string) (*PixCharge, error)

	// StartFlow creates a charge and returns both the charge and the first
	// FlowStatus the client should act on - for PIX that is ActionDisplayPix with
	// the QR/copy-paste/expiry in ActionData. It is the state-machine entry point.
	StartFlow(ctx context.Context, req PixChargeRequest) (*PixCharge, FlowStatus, error)

	// FlowState re-reads the charge and reports the current FlowStatus so the client
	// can advance (or finish) without knowing the PSP's status vocabulary.
	FlowState(ctx context.Context, providerID string) (FlowStatus, error)

	// CancelCharge aborts a not-yet-paid charge, recording reason. Returns
	// ErrChargeNotFound when the PSP does not know the id.
	CancelCharge(ctx context.Context, providerID, reason string) error

	// VerifyWebhook validates the raw body's signature and returns the normalized
	// event. Returns ErrInvalidSignature on signature failure (handler -> 400);
	// Type == EventUnknown for events not handled (handler -> 200, no-op).
	VerifyWebhook(ctx context.Context, raw RawWebhook) (*WebhookEvent, error)
}

// Refunder is an optional capability. A PIX-only provider need not implement it;
// the core reaches it via type assertion.
type Refunder interface {
	// Refund with amount == 0 means full refund.
	Refund(ctx context.Context, providerID string, amount Money) (*Refund, error)
}

type Refund struct {
	Provider   Name
	ProviderID string
	ChargeID   string
	Amount     Money
	Status     ChargeStatus
	Raw        json.RawMessage
}

// CardCharger is an optional capability for card payments (Phase 3).
type CardCharger interface {
	CreateCardCharge(ctx context.Context, req CardChargeRequest) (*CardCharge, error)
}

type CardChargeRequest struct {
	ReferenceID     string
	Amount          Money
	Currency        Currency
	PaymentMethodID string // card token; never a raw PAN in the core
	Capture         bool   // false = auth only
	Customer        *CustomerRef
	Metadata        map[string]string
}

type CardCharge struct {
	Provider   Name
	ProviderID string
	Status     ChargeStatus
	Amount     Money
	Currency   Currency
	Raw        json.RawMessage
}

// Registry selects a provider by config name. It replaces Rails-style string
// constantize with an explicit map populated at startup.
type Registry struct {
	providers map[Name]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[Name]Provider)}
}

func (r *Registry) Register(p Provider) { r.providers[p.Name()] = p }

func (r *Registry) Get(name Name) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, ErrNotConfigured
	}
	return p, nil
}
