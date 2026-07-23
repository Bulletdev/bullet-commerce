package propay

import (
	"context"
	"net/http"
	"strings"
	"time"

	"bullet-commerce/internal/payment"
)

// StartFlow creates the PIX charge (same wire call as CreatePixCharge) and returns
// the entry FlowStatus. PIX is always ActionDisplayPix on creation: the shopper has
// to be shown the QR/copy-paste and the client then polls FlowState until terminal.
func (c *Client) StartFlow(ctx context.Context, req payment.PixChargeRequest) (*payment.PixCharge, payment.FlowStatus, error) {
	charge, err := c.CreatePixCharge(ctx, req)
	if err != nil {
		return nil, payment.FlowStatus{}, err
	}
	fs := payment.FlowStatus{
		Status:     charge.Status,
		Action:     payment.ActionDisplayPix,
		ActionData: pixActionData(charge),
	}
	return charge, fs, nil
}

// FlowState re-reads the charge and maps the PSP status onto a FlowStatus. It reads
// the raw ProPay status (not the normalized one) so it can keep "approved" distinct
// from "paid": normalizeStatus collapses both to ChargePaid, but the flow layer must
// not tell a shopper "paid" on a mere authorization.
func (c *Client) FlowState(ctx context.Context, providerID string) (payment.FlowStatus, error) {
	raw, err := c.do(ctx, http.MethodGet, "/v1/service/charges/"+providerID, nil, "")
	if err != nil {
		return payment.FlowStatus{}, err
	}
	r, err := decodeChargeResponse(raw)
	if err != nil {
		return payment.FlowStatus{}, err
	}
	return flowStatusFor(r), nil
}

// cancelRequest carries the cancellation reason to ProPay.
//
// ASSUMPTION (ProPay-side pending): PRD-desacoplamento.md has no cancel spec for the
// service route. Modeled as DELETE /v1/service/charges/{id} with a {"reason": ...}
// body, following ProPay's convention of a filled reason on state transitions.
// Adjust verb/shape once the ProPay cancel route is defined.
type cancelRequest struct {
	Reason string `json:"reason"`
}

// CancelCharge aborts a pending charge. A non-2xx (incl. 404 -> ErrChargeNotFound)
// surfaces via do so the order layer can distinguish "already gone" from a fault.
func (c *Client) CancelCharge(ctx context.Context, providerID, reason string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/service/charges/"+providerID, cancelRequest{Reason: reason}, "")
	return err
}

// pixActionData is the ActionData a client needs to render a PIX charge: the hosted
// QR link, the EMV copy-and-paste payload, and when it expires. expires_at is
// omitted (rather than emitting a zero time) when the PSP gave no expiry.
func pixActionData(charge *payment.PixCharge) map[string]string {
	data := map[string]string{
		"qr_code":    charge.QRCodeImage, // hosted QR/payment link (qr_code_url)
		"copy_paste": charge.QRCodeText,  // EMV brCode
	}
	if !charge.ExpiresAt.IsZero() {
		data["expires_at"] = charge.ExpiresAt.Format(time.RFC3339)
	}
	return data
}

// flowStatusFor maps a raw ProPay status to a FlowStatus. Pending/active means the
// shopper still owes payment -> keep showing the PIX. "approved" is an authorization
// (funds held, not settled): no shopper action, but reported as pending so the order
// is not marked paid before capture. Everything terminal is ActionNone.
func flowStatusFor(r *chargeResponse) payment.FlowStatus {
	switch strings.ToUpper(strings.TrimSpace(r.Status)) {
	case "PENDING", "ACTIVE", "WAITING", "CREATED":
		return payment.FlowStatus{
			Status:     payment.ChargePending,
			Action:     payment.ActionDisplayPix,
			ActionData: pixActionData(chargeView(r)),
		}
	case "APPROVED", "AUTHORIZED":
		return payment.FlowStatus{Status: payment.ChargePending, Action: payment.ActionNone}
	default:
		return payment.FlowStatus{Status: normalizeStatus(r.Status), Action: payment.ActionNone}
	}
}

// chargeView is a thin adapter so pixActionData can reuse the wire response's QR
// fields without re-running the full parseCharge mapping.
func chargeView(r *chargeResponse) *payment.PixCharge {
	return &payment.PixCharge{
		QRCodeText:  r.QRCode,
		QRCodeImage: r.QRCodeURL,
		ExpiresAt:   parseTime(r.ExpiresAt),
	}
}
