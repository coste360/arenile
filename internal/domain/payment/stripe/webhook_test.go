package stripe_test

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/pkg/ext"
)

const target = "/webhooks/payments/stripe/est-1"

// TestParseWebhook_Signature: only deliveries signed with the
// establishment's secret at a time within the tolerance are accepted;
// every failure is ext.ErrInvalidSignature.
func TestParseWebhook_Signature(t *testing.T) {
	payload := stripetest.Event("evt_1", "checkout.session.completed", t0, map[string]any{
		"id": "cs_1", "object": "checkout.session", "amount_total": 4250, "currency": "eur", "payment_status": "paid",
		"status": "complete", "payment_intent": "pi_1", "metadata": map[string]string{"establishment_id": "est-1", "booking_id": "bk-1"},
	})
	for _, tc := range []struct {
		name    string
		header  func() string
		now     time.Time
		creds   ext.Credentials
		wantErr bool
	}{
		{"valid", func() string { return stripetest.Sign(payload, t0, stripetest.WebhookSecret) }, t0, creds(), false},
		{"valid within tolerance", func() string { return stripetest.Sign(payload, t0, stripetest.WebhookSecret) }, t0.Add(4 * time.Minute), creds(), false},
		{"rotated secret (second v1)", func() string { return stripetest.Sign(payload, t0, "whsec_old", stripetest.WebhookSecret) }, t0, creds(), false},
		{"wrong secret", func() string { return stripetest.Sign(payload, t0, "whsec_other") }, t0, creds(), true},
		{"replayed after tolerance", func() string { return stripetest.Sign(payload, t0, stripetest.WebhookSecret) }, t0.Add(6 * time.Minute), creds(), true},
		{"timestamp in the future", func() string { return stripetest.Sign(payload, t0.Add(10*time.Minute), stripetest.WebhookSecret) }, t0, creds(), true},
		{"missing header", func() string { return "" }, t0, creds(), true},
		{"garbage header", func() string { return "t=abc,v1=zz" }, t0, creds(), true},
		{"tampered timestamp", func() string {
			h := stripetest.Sign(payload, t0, stripetest.WebhookSecret)
			return "t=" + "1815639000" + h[len("t=1815638400"):]
		}, t0, creds(), true},
		{"no webhook secret configured", func() string { return stripetest.Sign(payload, t0, stripetest.WebhookSecret) }, t0, ext.Credentials{stripe.CredSecretKey: stripetest.SecretKey}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, c := newDriver(t)
			c.Set(tc.now)
			r := httptest.NewRequest("POST", target, bytes.NewReader(payload))
			if h := tc.header(); h != "" {
				r.Header.Set("Stripe-Signature", h)
			}
			ev, err := d.ParseWebhook(context.Background(), r, tc.creds)
			if tc.wantErr {
				if !errors.Is(err, ext.ErrInvalidSignature) {
					t.Fatalf("want invalid signature, got %v %+v", err, ev)
				}
				return
			}
			if err != nil || ev.ProviderEventID != "evt_1" || ev.Type != ext.PaymentSucceeded {
				t.Fatalf("valid delivery: %v %+v", err, ev)
			}
		})
	}
	t.Run("tampered body", func(t *testing.T) {
		d, _, _ := newDriver(t)
		r := httptest.NewRequest("POST", target, bytes.NewReader(append([]byte{}, bytes.Replace(payload, []byte("4250"), []byte("1"), 1)...)))
		r.Header.Set("Stripe-Signature", stripetest.Sign(payload, t0, stripetest.WebhookSecret))
		if _, err := d.ParseWebhook(context.Background(), r, creds()); !errors.Is(err, ext.ErrInvalidSignature) {
			t.Fatalf("tampered body: %v", err)
		}
	})
}

// TestParseWebhook_Mapping: every handled Stripe event type maps to the
// normalised event; others are ignored.
func TestParseWebhook_Mapping(t *testing.T) {
	md := map[string]string{"establishment_id": "est-1", "booking_id": "bk-1", "intent_id": "in-1"}
	session := func(status, paymentStatus string) map[string]any {
		return map[string]any{
			"id": "cs_1", "object": "checkout.session", "amount_total": 4250, "currency": "eur",
			"payment_status": paymentStatus, "status": status, "payment_intent": "pi_1", "metadata": md,
		}
	}
	for _, tc := range []struct {
		typ     string
		object  map[string]any
		want    ext.PaymentEventType
		ref     string
		amount  int64
		outcome string
		reason  string
		ignored bool
	}{
		{typ: "checkout.session.completed", object: session("complete", "paid"), want: ext.PaymentSucceeded, ref: "cs_1", amount: 4250},
		{typ: "checkout.session.completed", object: session("complete", "unpaid"), want: ext.PaymentPending, ref: "cs_1", amount: 4250},
		{typ: "checkout.session.async_payment_succeeded", object: session("complete", "paid"), want: ext.PaymentSucceeded, ref: "cs_1", amount: 4250},
		{typ: "checkout.session.async_payment_failed", object: session("complete", "unpaid"), want: ext.PaymentFailed, ref: "cs_1", amount: 4250, reason: "async_payment_failed"},
		{typ: "checkout.session.expired", object: session("expired", "unpaid"), want: ext.PaymentFailed, ref: "cs_1", amount: 4250, outcome: stripe.OutcomeExpired, reason: "expired"},
		{typ: "payment_intent.succeeded", object: map[string]any{"id": "pi_1", "object": "payment_intent", "amount": 4250, "amount_received": 4250, "currency": "eur", "status": "succeeded", "metadata": md}, want: ext.PaymentSucceeded, ref: "pi_1", amount: 4250},
		{typ: "payment_intent.payment_failed", object: map[string]any{"id": "pi_1", "object": "payment_intent", "amount": 4250, "currency": "eur", "status": "requires_payment_method", "metadata": md, "last_payment_error": map[string]any{"type": "card_error", "code": "card_declined", "message": "declined for mario@example.org"}}, want: ext.PaymentFailed, ref: "pi_1", amount: 4250, outcome: stripe.OutcomeAttemptFailed, reason: "card_declined"},
		{typ: "charge.refunded", object: map[string]any{"id": "ch_1", "object": "charge", "amount": 4250, "amount_captured": 4250, "amount_refunded": 1500, "currency": "eur", "payment_intent": "pi_1", "metadata": map[string]string{}}, want: ext.PaymentRefunded, ref: "pi_1", amount: 1500, outcome: stripe.OutcomeRefundTotal},
		{typ: "refund.failed", object: map[string]any{"id": "re_1", "object": "refund", "amount": 1500, "currency": "eur", "payment_intent": "pi_1", "status": "failed", "failure_reason": "expired_or_canceled_card", "metadata": md}, want: ext.PaymentFailed, ref: "re_1", amount: 1500, outcome: stripe.OutcomeRefundFailed, reason: "expired_or_canceled_card"},
		{typ: "customer.created", object: map[string]any{"id": "cus_1", "object": "customer"}, ignored: true},
		{typ: "charge.refunded", object: map[string]any{"id": "ch_2", "object": "charge", "amount": 10}, ignored: true},
	} {
		t.Run(tc.typ+"/"+tc.ref+tc.outcome, func(t *testing.T) {
			d, _, _ := newDriver(t)
			payload := stripetest.Event("evt_x", tc.typ, t0, tc.object)
			r := stripetest.WebhookRequest(target, payload, t0, stripetest.WebhookSecret)
			ev, err := d.ParseWebhook(context.Background(), r, creds())
			if tc.ignored {
				if !errors.Is(err, ext.ErrIgnoredEvent) {
					t.Fatalf("want ignored, got %v %+v", err, ev)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ev.Type != tc.want || ev.ProviderRef != tc.ref || ev.Amount != tc.amount || ev.Metadata[stripe.MetaOutcome] != tc.outcome || ev.FailureReason != tc.reason {
				t.Fatalf("event: %+v", ev)
			}
			if ev.ProviderEventID != "evt_x" || !ev.OccurredAt.Equal(t0) || ev.Currency != "EUR" {
				t.Fatalf("envelope: %+v", ev)
			}
			if tc.typ != "charge.refunded" && (ev.EstablishmentID != "est-1" || ev.BookingID != "bk-1") {
				t.Fatalf("ids: %+v", ev)
			}
		})
	}
}
