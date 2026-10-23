package stripe

import (
	"context"
	"fmt"
	"strings"

	stripego "github.com/stripe/stripe-go/v87"

	"github.com/coste360/arenile/pkg/ext"
)

// Refund implements ext.PaymentProvider: POST /v1/refunds on the
// PaymentIntent of the payment (ProviderRef may be the Checkout Session:
// its PaymentIntent is looked up first) with the amount in cents and the
// request idempotency key, so a retried refund is executed once.
func (d *Driver) Refund(ctx context.Context, req ext.RefundRequest) (*ext.RefundResult, error) {
	c, err := d.client(req.Credentials)
	if err != nil {
		return nil, err
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("stripe: refund amount must be positive")
	}
	pi, err := d.paymentIntentOf(ctx, c, req.ProviderRef)
	if err != nil {
		return nil, err
	}
	p := &stripego.RefundCreateParams{
		PaymentIntent: stripego.String(pi),
		Amount:        stripego.Int64(req.Amount),
		Reason:        stripego.String(string(stripego.RefundReasonRequestedByCustomer)),
		Metadata:      map[string]string{"establishment_id": req.EstablishmentID, "payment_id": req.PaymentID},
	}
	if req.IdempotencyKey != "" {
		p.SetIdempotencyKey(req.IdempotencyKey)
	}
	rf, err := c.V1Refunds.Create(ctx, p)
	if err != nil {
		return nil, mapErr("create refund", err)
	}
	return &ext.RefundResult{ProviderRef: rf.ID, Status: refundStatus(rf.Status)}, nil
}

// paymentIntentOf resolves the PaymentIntent id of a provider reference.
func (d *Driver) paymentIntentOf(ctx context.Context, c *stripego.Client, ref string) (string, error) {
	if !strings.HasPrefix(ref, "cs_") {
		return ref, nil
	}
	cs, err := c.V1CheckoutSessions.Retrieve(ctx, ref, nil)
	if err != nil {
		return "", mapErr("retrieve checkout session", err)
	}
	if cs.PaymentIntent == nil || cs.PaymentIntent.ID == "" {
		return "", fmt.Errorf("stripe: refund: %w", &APIError{Status: 400, Type: "invalid_request_error", Code: "no_payment_intent"})
	}
	return cs.PaymentIntent.ID, nil
}

func refundStatus(s stripego.RefundStatus) ext.RefundStatus {
	switch s {
	case stripego.RefundStatusSucceeded:
		return ext.RefundSucceeded
	case stripego.RefundStatusFailed, stripego.RefundStatusCanceled:
		return ext.RefundFailed
	default: // pending, requires_action
		return ext.RefundPending
	}
}

// PaymentStatus implements gateway.PaymentStatusChecker: it retrieves the
// Checkout Session (cs_), PaymentIntent (pi_) or Refund (re_) and returns
// the event a webhook would have carried, or nil while it is still open.
// ProviderEventID is empty (the gateway uses a synthetic id).
func (d *Driver) PaymentStatus(ctx context.Context, providerRef string, creds ext.Credentials) (*ext.PaymentEvent, error) {
	c, err := d.client(creds)
	if err != nil {
		return nil, err
	}
	out := &ext.PaymentEvent{OccurredAt: d.o.Clock.Now().UTC(), Metadata: map[string]string{}}
	switch {
	case strings.HasPrefix(providerRef, "cs_"):
		cs, err := c.V1CheckoutSessions.Retrieve(ctx, providerRef, nil)
		if err != nil {
			return nil, mapErr("retrieve checkout session", err)
		}
		fromSession(out, cs)
		switch {
		case cs.Status == stripego.CheckoutSessionStatusComplete && sessionPaid(cs):
			out.Type = ext.PaymentSucceeded
		case cs.Status == stripego.CheckoutSessionStatusExpired:
			out.Type, out.FailureReason = ext.PaymentFailed, OutcomeExpired
			out.Metadata[MetaOutcome] = OutcomeExpired
		default:
			return nil, nil
		}
	case strings.HasPrefix(providerRef, "pi_"):
		pi, err := c.V1PaymentIntents.Retrieve(ctx, providerRef, nil)
		if err != nil {
			return nil, mapErr("retrieve payment intent", err)
		}
		fromPaymentIntent(out, pi)
		switch pi.Status {
		case stripego.PaymentIntentStatusSucceeded:
			out.Type = ext.PaymentSucceeded
		case stripego.PaymentIntentStatusCanceled:
			out.Type, out.FailureReason = ext.PaymentFailed, "canceled"
		default:
			return nil, nil
		}
	case strings.HasPrefix(providerRef, "re_"):
		rf, err := c.V1Refunds.Retrieve(ctx, providerRef, nil)
		if err != nil {
			return nil, mapErr("retrieve refund", err)
		}
		fromRefund(out, rf)
		switch refundStatus(rf.Status) {
		case ext.RefundSucceeded:
			out.Type = ext.PaymentRefunded
			out.Metadata[MetaOutcome] = OutcomeRefundSingle
		case ext.RefundFailed:
			out.Type = ext.PaymentFailed
			out.Metadata[MetaOutcome] = OutcomeRefundFailed
			out.FailureReason = "refund_failed"
			if rf.FailureReason != "" {
				out.FailureReason = string(rf.FailureReason)
			}
		default:
			return nil, nil
		}
	default:
		return nil, fmt.Errorf("stripe: status: unknown reference kind")
	}
	return out, nil
}

// VerifyCredentials implements gateway.CredentialsVerifier: GET
// /v1/account with the secret key; the label is the dashboard display name
// (or business name, or account id) and the key mode ("test"/"live").
func (d *Driver) VerifyCredentials(ctx context.Context, creds ext.Credentials) (string, error) {
	c, err := d.client(creds)
	if err != nil {
		return "", err
	}
	acct, err := c.V1Accounts.Retrieve(ctx, nil)
	if err != nil {
		return "", mapErr("retrieve account", err)
	}
	label := acct.ID
	switch {
	case acct.Settings != nil && acct.Settings.Dashboard != nil && acct.Settings.Dashboard.DisplayName != "":
		label = acct.Settings.Dashboard.DisplayName
	case acct.BusinessProfile != nil && acct.BusinessProfile.Name != "":
		label = acct.BusinessProfile.Name
	}
	if m := KeyMode(creds[CredSecretKey]); m != "" {
		label += " (" + m + ")"
	}
	return label, nil
}

// KeyMode returns "live" or "test" from the prefix of a secret or
// restricted key ("" when unrecognisable).
func KeyMode(key string) string {
	switch {
	case strings.HasPrefix(key, "sk_live_"), strings.HasPrefix(key, "rk_live_"):
		return "live"
	case strings.HasPrefix(key, "sk_test_"), strings.HasPrefix(key, "rk_test_"):
		return "test"
	default:
		return ""
	}
}
