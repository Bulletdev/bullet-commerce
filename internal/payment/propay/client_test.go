package propay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bullet-commerce/internal/payment"

	"github.com/golang-jwt/jwt/v5"
)

const (
	goToSecret = "go-to-propay-secret"
	toGoSecret = "propay-to-go-secret"
)

func newTestClient(url string) *Client {
	return New(Config{
		URL:        url,
		GoToSecret: goToSecret,
		ToGoSecret: toGoSecret,
		Timeout:    2 * time.Second,
	})
}

// sampleChargeJSON mirrors ProPay's charges_handler.rb#serialize output: the
// {"data": {...}} envelope, status "active" (Charge default), integer amount_cents,
// qr_code (EMV brCode) and qr_code_url (hosted payment link).
func sampleChargeJSON() string {
	return `{
		"data": {
			"txid": "TX-123",
			"status": "active",
			"amount_cents": 1500,
			"qr_code": "00020126BR.GOV.BCB.PIX",
			"qr_code_url": "https://api.openpix.com.br/pay/TX-123",
			"expires_at": "2026-07-20T12:05:00Z",
			"paid_at": null
		}
	}`
}

func TestName(t *testing.T) {
	if got := newTestClient("http://x").Name(); got != payment.Name("propay") {
		t.Fatalf("Name() = %q, want propay", got)
	}
}

// AC: CreatePixCharge carries a Bearer JWT with aud=["propay"] and exp<=now+5min.
func TestCreatePixCharge_JWTClaimsAndRequest(t *testing.T) {
	var gotAuth string
	var gotIdempotency string
	var gotBody createChargeRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/service/charges" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(sampleChargeJSON()))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	before := time.Now()
	charge, err := c.CreatePixCharge(context.Background(), payment.PixChargeRequest{
		ReferenceID: "order-1",
		Amount:      1500,
		Currency:    "BRL",
		ExpiresIn:   5 * time.Minute,
		Description: "Test order",
		Customer:    &payment.CustomerRef{Name: "Ada", TaxID: "12345678900", Email: "ada@x.com"},
	})
	if err != nil {
		t.Fatalf("CreatePixCharge: %v", err)
	}

	// Request body assertions: ProPay field names (amount_cents, expires_in_seconds,
	// reference_type/reference_id) and the required Idempotency-Key header.
	if gotBody.ReferenceType != "order" || gotBody.ReferenceID != "order-1" ||
		gotBody.AmountCents != 1500 || gotBody.ExpiresInSeconds != 300 {
		t.Errorf("unexpected request body: %+v", gotBody)
	}
	if gotBody.Description != "Test order" {
		t.Errorf("description not forwarded: %q", gotBody.Description)
	}
	if gotIdempotency != "order-1" {
		t.Errorf("Idempotency-Key = %q, want order-1", gotIdempotency)
	}
	if gotBody.Customer == nil || gotBody.Customer.CPF != "12345678900" {
		t.Errorf("customer not forwarded: %+v", gotBody.Customer)
	}

	// JWT assertions.
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("missing Bearer prefix: %q", gotAuth)
	}
	tokenStr := strings.TrimPrefix(gotAuth, "Bearer ")
	claims := &serviceClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(goToSecret), nil
	})
	if err != nil || !tok.Valid {
		t.Fatalf("JWT invalid: %v", err)
	}
	if !containsAud(claims.Audience, "propay") {
		t.Errorf("aud = %v, want contains propay", claims.Audience)
	}
	if claims.Issuer != "bullet-commerce" || claims.Subject != "bullet-commerce" || claims.Role != "service" {
		t.Errorf("unexpected claims iss/sub/role: %+v", claims)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("exp missing")
	}
	maxExp := before.Add(serviceTokenTTL).Add(time.Second)
	if claims.ExpiresAt.After(maxExp) {
		t.Errorf("exp %v exceeds now+5min %v", claims.ExpiresAt.Time, maxExp)
	}

	// Response mapping.
	if charge.ProviderID != "TX-123" || charge.TxID != "TX-123" {
		t.Errorf("txid mapping wrong: %+v", charge)
	}
	if charge.Status != payment.ChargePending {
		t.Errorf("status = %q, want pending", charge.Status)
	}
	if charge.Amount != 1500 || charge.Currency != "BRL" {
		t.Errorf("amount/currency wrong: %d %s", charge.Amount, charge.Currency)
	}
	if charge.QRCodeText == "" || charge.QRCodeImage == "" {
		t.Errorf("qr fields empty: %+v", charge)
	}
	if len(charge.Raw) == 0 {
		t.Error("Raw not populated")
	}
}

func TestGetCharge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/service/charges/TX-123" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer on GetCharge")
		}
		w.Write([]byte(sampleChargeJSON()))
	}))
	defer srv.Close()

	charge, err := newTestClient(srv.URL).GetCharge(context.Background(), "TX-123")
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	if charge.TxID != "TX-123" || charge.Status != payment.ChargePending {
		t.Errorf("unexpected charge: %+v", charge)
	}
}

func TestGetCharge_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).GetCharge(context.Background(), "missing")
	if !errors.Is(err, payment.ErrChargeNotFound) {
		t.Fatalf("err = %v, want ErrChargeNotFound", err)
	}
}

// AC: ProPay 5xx => CreatePixCharge returns error (no panic).
func TestCreatePixCharge_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).CreatePixCharge(context.Background(), payment.PixChargeRequest{ReferenceID: "o1", Amount: 100})
	if err == nil {
		t.Fatal("expected error on 5xx")
	}
}

// AC: timeout => CreatePixCharge returns error (no panic).
func TestCreatePixCharge_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(sampleChargeJSON()))
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL, GoToSecret: goToSecret, ToGoSecret: toGoSecret, Timeout: 20 * time.Millisecond})
	_, err := c.CreatePixCharge(context.Background(), payment.PixChargeRequest{ReferenceID: "o1", Amount: 100})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// AC: correct HMAC => WebhookEvent with normalized status.
func TestVerifyWebhook_ValidSignature(t *testing.T) {
	body := []byte(`{
		"event": "charge.paid",
		"event_id": "evt-1",
		"data": {
			"txid": "TX-123",
			"reference_type": "order",
			"reference_id": "order-1",
			"status": "paid",
			"amount_cents": 1500,
			"paid_at": "2026-07-20T12:03:00Z"
		}
	}`)

	c := newTestClient("http://x")
	evt, err := c.VerifyWebhook(context.Background(), payment.RawWebhook{
		Headers: map[string]string{"X-Propay-Signature": signBody(toGoSecret, body)},
		Body:    body,
	})
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if evt.Type != payment.EventChargePaid {
		t.Errorf("type = %q, want charge.paid", evt.Type)
	}
	if evt.Status != payment.ChargePaid {
		t.Errorf("status = %q, want paid", evt.Status)
	}
	if evt.TxID != "TX-123" || evt.ReferenceID != "order-1" || evt.EventID != "evt-1" {
		t.Errorf("field mapping wrong: %+v", evt)
	}
	if evt.Amount != 1500 || evt.Currency != "BRL" {
		t.Errorf("amount/currency wrong: %+v", evt)
	}
	if evt.PaidAt == nil {
		t.Error("PaidAt should be set")
	}
}

// AC: incorrect HMAC => ErrInvalidSignature and NO event.
func TestVerifyWebhook_InvalidSignature(t *testing.T) {
	body := []byte(`{"event":"charge.paid","status":"PAID"}`)
	c := newTestClient("http://x")

	cases := map[string]string{
		"wrong secret":   signBody("attacker-secret", body),
		"tampered body":  signBody(toGoSecret, []byte(`{"other":true}`)),
		"missing header": "",
		"malformed hex":  "sha256=zzzz",
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{}
			if sig != "" {
				headers["X-Propay-Signature"] = sig
			}
			evt, err := c.VerifyWebhook(context.Background(), payment.RawWebhook{Headers: headers, Body: body})
			if !errors.Is(err, payment.ErrInvalidSignature) {
				t.Fatalf("err = %v, want ErrInvalidSignature", err)
			}
			if evt != nil {
				t.Errorf("event should be nil, got %+v", evt)
			}
		})
	}
}

func TestVerifyWebhook_UnknownEventType(t *testing.T) {
	body := []byte(`{"event":"charge.something_new","data":{"status":"active"}}`)
	c := newTestClient("http://x")
	evt, err := c.VerifyWebhook(context.Background(), payment.RawWebhook{
		Headers: map[string]string{"X-Propay-Signature": signBody(toGoSecret, body)},
		Body:    body,
	})
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if evt.Type != payment.EventUnknown {
		t.Errorf("type = %q, want unknown", evt.Type)
	}
}

func TestVerifyWebhook_CaseInsensitiveHeader(t *testing.T) {
	body := []byte(`{"event":"charge.expired","data":{"status":"expired"}}`)
	c := newTestClient("http://x")
	evt, err := c.VerifyWebhook(context.Background(), payment.RawWebhook{
		Headers: map[string]string{"x-propay-signature": signBody(toGoSecret, body)},
		Body:    body,
	})
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if evt.Type != payment.EventChargeExpired || evt.Status != payment.ChargeExpired {
		t.Errorf("unexpected event: %+v", evt)
	}
}

// AC: StartFlow creates the charge and returns ActionDisplayPix with the QR data.
func TestStartFlow_ReturnsDisplayPix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/service/charges" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(sampleChargeJSON()))
	}))
	defer srv.Close()

	charge, fs, err := newTestClient(srv.URL).StartFlow(context.Background(), payment.PixChargeRequest{
		ReferenceID: "order-1",
		Amount:      1500,
		ExpiresIn:   5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("StartFlow: %v", err)
	}
	if charge == nil || charge.TxID != "TX-123" {
		t.Fatalf("charge not returned: %+v", charge)
	}
	if fs.Action != payment.ActionDisplayPix {
		t.Errorf("action = %q, want display_pix", fs.Action)
	}
	if fs.Status != payment.ChargePending {
		t.Errorf("status = %q, want pending", fs.Status)
	}
	if fs.ActionData["copy_paste"] == "" || fs.ActionData["qr_code"] == "" {
		t.Errorf("ActionData missing qr fields: %+v", fs.ActionData)
	}
	if fs.ActionData["expires_at"] == "" {
		t.Errorf("ActionData missing expires_at: %+v", fs.ActionData)
	}
}

// FlowState must keep "approved" (authorization) distinct from "paid" (settlement):
// approved reports pending with no shopper action, paid reports paid.
func TestFlowState_ApprovedVsPaid(t *testing.T) {
	cases := []struct {
		raw        string
		wantStatus payment.ChargeStatus
		wantAction payment.FlowAction
	}{
		{"approved", payment.ChargePending, payment.ActionNone},
		{"paid", payment.ChargePaid, payment.ActionNone},
		{"active", payment.ChargePending, payment.ActionDisplayPix},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			body := `{"data":{"txid":"TX-123","status":"` + tc.raw + `","amount_cents":1500,` +
				`"qr_code":"00020126","qr_code_url":"https://x/pay","expires_at":"2026-07-20T12:05:00Z"}}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/service/charges/TX-123" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Write([]byte(body))
			}))
			defer srv.Close()

			fs, err := newTestClient(srv.URL).FlowState(context.Background(), "TX-123")
			if err != nil {
				t.Fatalf("FlowState: %v", err)
			}
			if fs.Status != tc.wantStatus || fs.Action != tc.wantAction {
				t.Errorf("got {%q,%q}, want {%q,%q}", fs.Status, fs.Action, tc.wantStatus, tc.wantAction)
			}
		})
	}
}

// AC: CancelCharge issues a DELETE with the reason and succeeds on 2xx.
func TestCancelCharge(t *testing.T) {
	var gotBody cancelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/service/charges/TX-123" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing bearer on CancelCharge")
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(srv.URL).CancelCharge(context.Background(), "TX-123", "customer_gave_up"); err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}
	if gotBody.Reason != "customer_gave_up" {
		t.Errorf("reason = %q, want customer_gave_up", gotBody.Reason)
	}
}

func TestCancelCharge_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).CancelCharge(context.Background(), "missing", "x")
	if !errors.Is(err, payment.ErrChargeNotFound) {
		t.Fatalf("err = %v, want ErrChargeNotFound", err)
	}
}

func containsAud(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}
