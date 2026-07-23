// Package propay adapts the ProPay/OpenPix PSP to the payment.Provider contract.
//
// Acceptance criteria (covered by client_test.go):
//   - Given GoToSecret configured, When CreatePixCharge is called, Then the request
//     to ProPay carries a Bearer JWT with aud=["propay"] and exp <= now+5min.
//   - Given a webhook body and a correct HMAC signature with ToGoSecret, When
//     VerifyWebhook, Then it returns a WebhookEvent with the normalized status.
//   - Given an incorrect HMAC signature, When VerifyWebhook, Then it returns
//     payment.ErrInvalidSignature and NO event.
//   - Given ProPay returning 5xx/timeout, When CreatePixCharge, Then it returns an
//     error (does not panic).
//
// Transport model: the bullet-commerce core calls ProPay machine-to-machine, so
// outbound requests are authenticated with a short-lived service JWT signed with
// GoToSecret ("go->propay" direction). Inbound webhooks are authenticated with an
// HMAC-SHA256 over the exact raw body using ToGoSecret ("propay->go" direction).
// The two secrets are intentionally distinct: leaking one direction must not let
// an attacker forge the other.
package propay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bullet-commerce/internal/payment"

	"github.com/golang-jwt/jwt/v5"
)

const providerName = payment.Name("propay")

// serviceName is the machine-to-machine identity bullet-commerce presents to ProPay
// (service/user_id/iss/sub on the outbound service JWT). Single source so all four
// claim fields stay in lockstep.
const serviceName = "bullet-commerce"

// serviceTokenTTL bounds how long an outbound service JWT is valid. Kept short so
// a captured token has a small replay window; the acceptance criteria pin exp<=5min.
const serviceTokenTTL = 5 * time.Minute

// Config carries the ProPay endpoint and the two directional HMAC/JWT secrets.
type Config struct {
	URL        string        // base URL, e.g. https://propay.internal
	GoToSecret string        // signs outbound service JWTs (bullet-commerce -> propay)
	ToGoSecret string        // verifies inbound webhook HMAC (propay -> bullet-commerce)
	Timeout    time.Duration // per-request HTTP timeout
}

// Client is a stateless ProPay adapter. Safe for concurrent use.
// Compile-time proof that Client satisfies the provider contract - any change to
// payment.Provider breaks this build immediately, not at runtime registration.
var _ payment.Provider = (*Client)(nil)

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) Name() payment.Name { return providerName }

// serviceClaims is the outbound service JWT. aud is an array because the audience
// contract is multi-valued ("propay") and RegisteredClaims already models that.
//
// The shape mirrors ProPay's ServiceClaims contract (PRD-desacoplamento.md):
//
//	{ "service": "bullet-commerce", "user_id": "bullet-commerce", "role": "service",
//	  "aud": ["propay"], "iss": "bullet-commerce", "exp": "<now + 5min>" }
//
// user_id is REQUIRED: ProPay's Middleware::Auth#valid? reads user_id from the
// decoded token and rejects (401) any token without it - even a role=service one.
// It is set to the literal "bullet-commerce" because the service route is machine-to-
// machine and has no per-user identity. role=service is what the future
// /v1/service/charges route gates on via auth.service?.
type serviceClaims struct {
	Service string `json:"service"`
	UserID  string `json:"user_id"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

func (c *Client) signServiceToken() (string, error) {
	now := time.Now()
	claims := serviceClaims{
		Service: serviceName,
		UserID:  serviceName,
		Role:    "service",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    serviceName,
			Subject:   serviceName,
			Audience:  jwt.ClaimStrings{"propay"},
			ExpiresAt: jwt.NewNumericDate(now.Add(serviceTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(c.cfg.GoToSecret))
}

// --- wire types ---
//
// Field names below match ProPay's real request/response contract, extracted from
// the Ruby source:
//   - request  -> app/validators/charge_validator.rb (dry-validation params) and
//                 app/handlers/charges_handler.rb (Integer(body['amount_cents']),
//                 body['reference_type'], expires_in_seconds).
//   - response -> app/handlers/charges_handler.rb#serialize, always wrapped in a
//                 {"data": {...}} envelope with txid/status/amount_cents/qr_code/
//                 qr_code_url/expires_at/paid_at.
//
// Money is integer cents end-to-end (ProPay's amount_cents == the core's int64
// cents), so NO decimal conversion happens at this boundary. ProPay is BRL-only
// and returns no currency field.

type createChargeRequest struct {
	// ASSUMPTION (ProPay-side pending): ChargeValidator::VALID_REFERENCE_TYPES and
	// Charge::STATUSES do not yet include "order" (they cover subscription,
	// tournament_registration, wallet_deposit). PRD-desacoplamento.md lists adding
	// "order" to both as a required ProPay migration for e-commerce charges.
	ReferenceType string `json:"reference_type"`
	// ASSUMPTION (ProPay-side pending): charge_validator.rb declares reference_id as
	// maybe(:integer) and the current column is Bignum. The bullet-commerce order id is a
	// UUID string; ProPay needs the reference_id-as-text migration (PRD) to accept it.
	ReferenceID      string `json:"reference_id"`
	AmountCents      int64  `json:"amount_cents"`       // integer cents, >0
	Description      string `json:"description"`        // required, <=140 chars
	ExpiresInSeconds int64  `json:"expires_in_seconds"` // 300..86400
	// ASSUMPTION (ProPay-side pending): the future /v1/service/charges route is
	// Customer-less (no find_customer!), so ProPay ignores an inline customer today.
	// Kept for when the service route accepts payer data; fields mirror ProPay's
	// Customer model (full_name/cpf/email in customers_handler.rb).
	Customer *customerPayload `json:"customer,omitempty"`
}

type customerPayload struct {
	FullName string `json:"full_name,omitempty"`
	CPF      string `json:"cpf,omitempty"`
	Email    string `json:"email,omitempty"`
}

// chargeResponse maps the inner object of ProPay's {"data": {...}} charge envelope
// (charges_handler.rb#serialize). qr_code is the EMV copy-and-paste (brCode);
// qr_code_url is the hosted payment link (OpenPix paymentLinkUrl). ProPay exposes
// no base64 image and no created_at/currency at this layer.
type chargeResponse struct {
	TxID        string `json:"txid"`
	Status      string `json:"status"`
	AmountCents int64  `json:"amount_cents"`
	QRCode      string `json:"qr_code"`
	QRCodeURL   string `json:"qr_code_url"`
	ExpiresAt   string `json:"expires_at"`
	PaidAt      string `json:"paid_at"`
}

// CreatePixCharge posts to /v1/service/charges. That route is a ProPay-side
// blocker (PRD): it must be added as a service-authenticated, Customer-less route.
// The field names, cents-amount, Idempotency-Key requirement and status/response
// shape below already match ProPay's existing conventions.
func (c *Client) CreatePixCharge(ctx context.Context, req payment.PixChargeRequest) (*payment.PixCharge, error) {
	// ChargeValidator requires a filled description (<=140 chars); default from the
	// order id so an empty Description never trips 422 validation_failed.
	description := req.Description
	if description == "" {
		description = "Pedido " + req.ReferenceID
	}

	body := createChargeRequest{
		ReferenceType:    "order",
		ReferenceID:      req.ReferenceID,
		AmountCents:      int64(req.Amount),
		Description:      description,
		ExpiresInSeconds: int64(req.ExpiresIn.Seconds()),
	}
	if req.Customer != nil {
		body.Customer = &customerPayload{
			FullName: req.Customer.Name,
			CPF:      req.Customer.TaxID,
			Email:    req.Customer.Email,
		}
	}

	// ProPay's ChargesHandler#require_idempotency_key! rejects (422) any create
	// without an Idempotency-Key header. The order id is the natural key.
	raw, err := c.do(ctx, http.MethodPost, "/v1/service/charges", body, req.ReferenceID)
	if err != nil {
		return nil, err
	}
	return c.parseCharge(raw)
}

func (c *Client) GetCharge(ctx context.Context, providerID string) (*payment.PixCharge, error) {
	raw, err := c.do(ctx, http.MethodGet, "/v1/service/charges/"+providerID, nil, "")
	if err != nil {
		return nil, err
	}
	return c.parseCharge(raw)
}

// do issues an authenticated request and returns the raw response body. It fails
// on any non-2xx so callers never parse an error page as a charge. A non-empty
// idempotencyKey is sent as the Idempotency-Key header (required by ProPay on
// charge creation).
func (c *Client) do(ctx context.Context, method, path string, payloadBody any, idempotencyKey string) (json.RawMessage, error) {
	var buf io.Reader
	if payloadBody != nil {
		b, err := json.Marshal(payloadBody)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewReader(b)
	}

	token, err := c.signServiceToken()
	if err != nil {
		return nil, fmt.Errorf("propay: sign service token: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	if payloadBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", idempotencyKey)
	}

	// Roadmap: circuit breaker portado do riot-gateway
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("propay: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("propay: read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, payment.ErrChargeNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("propay: %s %s: unexpected status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return json.RawMessage(respBody), nil
}

// decodeChargeResponse unwraps ProPay's {"data": {...}} charge envelope, tolerating
// a bare object so a direct provider payload still parses. Shared by parseCharge
// and FlowState - the latter needs the untouched status string to tell "approved"
// from "paid", which normalizeStatus deliberately collapses.
func decodeChargeResponse(raw json.RawMessage) (*chargeResponse, error) {
	var env struct {
		Data *chargeResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("propay: decode charge: %w", err)
	}
	r := env.Data
	if r == nil || r.TxID == "" {
		var bare chargeResponse
		if err := json.Unmarshal(raw, &bare); err != nil {
			return nil, fmt.Errorf("propay: decode charge: %w", err)
		}
		if bare.TxID != "" {
			r = &bare
		}
	}
	if r == nil {
		return nil, fmt.Errorf("propay: decode charge: empty payload")
	}
	return r, nil
}

func (c *Client) parseCharge(raw json.RawMessage) (*payment.PixCharge, error) {
	r, err := decodeChargeResponse(raw)
	if err != nil {
		return nil, err
	}

	charge := &payment.PixCharge{
		Provider:   providerName,
		ProviderID: r.TxID,
		TxID:       r.TxID,
		Status:     normalizeStatus(r.Status),
		Amount:     payment.Money(r.AmountCents),
		// ProPay is BRL-only and returns no currency field.
		Currency:   payment.Currency("BRL"),
		QRCodeText: r.QRCode, // EMV copy-and-paste (brCode)
		// ProPay returns a hosted payment link, not a base64 image; the core's
		// PixCharge has no dedicated link field, so surface it as QRCodeImage.
		QRCodeImage: r.QRCodeURL,
		ExpiresAt:   parseTime(r.ExpiresAt),
		Raw:         raw,
	}
	return charge, nil
}

// webhookPayload maps the ProPay -> bullet-commerce webhook body.
//
// ASSUMPTION (ProPay-side pending): this outbound webhook does not exist yet.
// PixWebhookJob#process_reference has no `when 'order'` branch, so ProPay never
// PATCHes bullet-commerce on payment (PRD lists it as a required addition). The shape
// below follows ProPay's own conventions: a top-level event/event_id plus the
// charge under the {"data": {...}} envelope with the same field names ProPay
// serializes elsewhere (txid/status/amount_cents/reference_*/paid_at). Adjust the
// tags once the ProPay `order` webhook is implemented.
type webhookPayload struct {
	Event   string `json:"event"`
	EventID string `json:"event_id"`
	Data    struct {
		TxID          string `json:"txid"`
		Status        string `json:"status"`
		AmountCents   int64  `json:"amount_cents"`
		ReferenceType string `json:"reference_type"`
		ReferenceID   string `json:"reference_id"`
		PaidAt        string `json:"paid_at"`
	} `json:"data"`
}

func (c *Client) VerifyWebhook(ctx context.Context, raw payment.RawWebhook) (*payment.WebhookEvent, error) {
	if !c.validSignature(raw) {
		return nil, payment.ErrInvalidSignature
	}

	var p webhookPayload
	if err := json.Unmarshal(raw.Body, &p); err != nil {
		return nil, fmt.Errorf("propay: decode webhook: %w", err)
	}

	evt := &payment.WebhookEvent{
		Provider:    providerName,
		EventID:     p.EventID,
		Type:        normalizeEventType(p.Event),
		ProviderID:  p.Data.TxID,
		TxID:        p.Data.TxID,
		ReferenceID: p.Data.ReferenceID,
		Status:      normalizeStatus(p.Data.Status),
		Amount:      payment.Money(p.Data.AmountCents),
		Currency:    payment.Currency("BRL"), // ProPay is BRL-only.
		Raw:         json.RawMessage(raw.Body),
	}
	if t := parseTime(p.Data.PaidAt); !t.IsZero() {
		evt.PaidAt = &t
	}
	return evt, nil
}

// validSignature compares the provided HMAC over the exact raw body in constant
// time. Header format is "X-Propay-Signature: sha256=<hexlowercase>", HMAC-SHA256
// over the raw body keyed with PROPAY_TO_GO_SECRET - the contract fixed by
// PRD-desacoplamento.md for the ProPay -> bullet-commerce leg.
//
// Note: ProPay's *inbound* OpenPix leg (webhooks_handler.rb / OpenpixProvider)
// instead uses header "x-webhook-signature" with a bare hex digest (no "sha256="
// prefix). That is a different hop; the PATCH to bullet-commerce follows the PRD
// header above. The "sha256=" prefix is stripped defensively so a bare hex digest
// still verifies if ProPay ships the OpenPix-style format.
func (c *Client) validSignature(raw payment.RawWebhook) bool {
	sig := headerValue(raw.Headers, "X-Propay-Signature")
	if sig == "" {
		return false
	}
	sig = strings.TrimPrefix(sig, "sha256=")
	provided, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.cfg.ToGoSecret))
	mac.Write(raw.Body)
	return hmac.Equal(provided, mac.Sum(nil))
}

// headerValue does a case-insensitive lookup since RawWebhook.Headers is a plain
// map populated by whatever HTTP layer fronts the handler.
func headerValue(headers map[string]string, key string) string {
	if v, ok := headers[key]; ok {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// normalizeStatus maps ProPay's Charge::STATUSES (pending, active, paid, expired,
// cancelled, refunded) onto the core's ChargeStatus. The extra aliases keep the
// mapping resilient to provider variations.
func normalizeStatus(s string) payment.ChargeStatus {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PENDING", "ACTIVE", "WAITING", "CREATED":
		return payment.ChargePending
	case "PAID", "COMPLETED", "CONFIRMED":
		return payment.ChargePaid
	case "EXPIRED":
		return payment.ChargeExpired
	case "CANCELED", "CANCELLED":
		return payment.ChargeCanceled
	case "REFUNDED":
		return payment.ChargeRefunded
	case "FAILED", "ERROR":
		return payment.ChargeFailed
	default:
		return payment.ChargePending
	}
}

// normalizeEventType is substring-based so it handles both the assumed dot-style
// ("charge.paid") and ProPay's OpenPix-style event strings ("OPENPIX:CHARGE_COMPLETED"),
// whose payment_event? check keys on COMPLETED/PAID.
func normalizeEventType(e string) payment.EventType {
	x := strings.ToLower(strings.TrimSpace(e))
	switch {
	case strings.Contains(x, "refund"):
		return payment.EventChargeRefunded
	case strings.Contains(x, "expired"):
		return payment.EventChargeExpired
	case strings.Contains(x, "paid"), strings.Contains(x, "completed"), strings.Contains(x, "confirmed"):
		return payment.EventChargePaid
	default:
		return payment.EventUnknown
	}
}

// parseTime accepts RFC3339 and returns the zero time on empty/invalid input so
// callers can treat "no timestamp" and "bad timestamp" the same (best-effort).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
