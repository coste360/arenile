package stripe_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/pkg/ext"
)

var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

func creds() ext.Credentials {
	return ext.Credentials{stripe.CredSecretKey: stripetest.SecretKey, stripe.CredWebhookSecret: stripetest.WebhookSecret}
}

func newDriver(t *testing.T) (*stripe.Driver, *stripetest.Server, *clock.Fake) {
	t.Helper()
	c := clock.NewFake(t0)
	srv := stripetest.New(t)
	srv.Now = c.Now
	zero := int64(0)
	return stripe.New(stripe.Options{BaseURL: srv.URL, Clock: c, MaxNetworkRetries: &zero}), srv, c
}

func paymentRequest() ext.CreatePaymentRequest {
	return ext.CreatePaymentRequest{
		EstablishmentID: "est-1", BookingID: "bk-1", BookingCode: "LD-0001", Amount: 4250, Currency: "EUR",
		Description: "Ombrellone A3, 15 luglio", CustomerEmail: "mario@example.org",
		ReturnURL: "https://lido.example/b/lido?checkout=1", CancelURL: "https://lido.example/b/lido",
		PreferredMode: ext.PaymentModeRedirect, IdempotencyKey: "bk-1:1",
		Metadata:    map[string]string{"intent_id": "in-1", "locale": "de", "expires_at": t0.Add(45 * time.Minute).Format(time.RFC3339)},
		Credentials: creds(),
	}
}

// TestCreatePayment_CheckoutSession: the session carries the amount in
// cents, currency, metadata on the session and its payment intent, locale,
// expiry aligned with the hold, the idempotency key and the pinned version;
// hints are not forwarded as metadata.
func TestCreatePayment_CheckoutSession(t *testing.T) {
	d, srv, _ := newDriver(t)
	res, err := d.CreatePayment(context.Background(), paymentRequest())
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ext.PaymentModeRedirect || !strings.HasPrefix(res.ProviderRef, "cs_") || res.RedirectURL == "" {
		t.Fatalf("result: %+v", res)
	}
	if res.ExpiresAt == nil || !res.ExpiresAt.Equal(t0.Add(45*time.Minute)) {
		t.Fatalf("expires: %v", res.ExpiresAt)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	r := reqs[0]
	f := r.Form
	checks := map[string]string{
		"mode": "payment", "line_items[0][quantity]": "1", "line_items[0][price_data][unit_amount]": "4250",
		"line_items[0][price_data][currency]": "eur", "line_items[0][price_data][product_data][name]": "Ombrellone A3, 15 luglio",
		"customer_email": "mario@example.org", "client_reference_id": "bk-1", "locale": "de",
		"success_url": "https://lido.example/b/lido?checkout=1", "cancel_url": "https://lido.example/b/lido",
		"metadata[booking_id]": "bk-1", "metadata[establishment_id]": "est-1", "metadata[booking_code]": "LD-0001",
		"metadata[intent_id]": "in-1", "payment_intent_data[metadata][booking_id]": "bk-1",
		"expires_at": "1815641100",
	}
	for k, want := range checks {
		if got := f.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if f.Get("metadata[locale]") != "" || f.Get("metadata[expires_at]") != "" {
		t.Errorf("hints forwarded as metadata: %v", f)
	}
	if r.IdempotencyKey != "bk-1:1" || r.StripeVersion != stripe.APIVersion || r.Authorization != "Bearer "+stripetest.SecretKey {
		t.Errorf("headers: %+v", r)
	}
}

// TestSessionExpiry_Clamped: a hold shorter than Stripe's minimum gets the
// minimum (31 min), a missing hint too, a far one the 24 h maximum.
func TestSessionExpiry_Clamped(t *testing.T) {
	for _, tc := range []struct {
		name string
		hint string
		want time.Time
	}{
		{"short hold", t0.Add(10 * time.Minute).Format(time.RFC3339), t0.Add(31 * time.Minute)},
		{"no hint", "", t0.Add(31 * time.Minute)},
		{"garbage", "tomorrow", t0.Add(31 * time.Minute)},
		{"aligned", t0.Add(2 * time.Hour).Format(time.RFC3339), t0.Add(2 * time.Hour)},
		{"too far", t0.Add(48 * time.Hour).Format(time.RFC3339), t0.Add(24 * time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _, _ := newDriver(t)
			req := paymentRequest()
			req.Metadata["expires_at"] = tc.hint
			res, err := d.CreatePayment(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if !res.ExpiresAt.Equal(tc.want) {
				t.Fatalf("expires %v, want %v", res.ExpiresAt, tc.want)
			}
		})
	}
}

// TestCreatePayment_Embedded: a PaymentIntent with a client secret.
func TestCreatePayment_Embedded(t *testing.T) {
	d, srv, _ := newDriver(t)
	req := paymentRequest()
	req.PreferredMode = ext.PaymentModeEmbedded
	res, err := d.CreatePayment(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != ext.PaymentModeEmbedded || !strings.HasPrefix(res.ProviderRef, "pi_") || res.ClientSecret == "" {
		t.Fatalf("result: %+v", res)
	}
	f := srv.Requests()[0].Form
	if f.Get("amount") != "4250" || f.Get("currency") != "eur" || f.Get("automatic_payment_methods[enabled]") != "true" || f.Get("receipt_email") != "mario@example.org" {
		t.Fatalf("form: %v", f)
	}
}

// TestCreatePayment_Errors: outages are temporary, refusals carry only the
// Stripe code, a refused key and a reused idempotency key are typed.
func TestCreatePayment_Errors(t *testing.T) {
	t.Run("outage", func(t *testing.T) {
		d, srv, _ := newDriver(t)
		srv.FailNext(1)
		_, err := d.CreatePayment(context.Background(), paymentRequest())
		var tmp interface{ Temporary() bool }
		if !errors.Is(err, stripe.ErrUnavailable) || !errors.As(err, &tmp) || !tmp.Temporary() {
			t.Fatalf("outage: %v", err)
		}
	})
	t.Run("network", func(t *testing.T) {
		c := clock.NewFake(t0)
		zero := int64(0)
		d := stripe.New(stripe.Options{BaseURL: "http://127.0.0.1:1", Clock: c, MaxNetworkRetries: &zero})
		if _, err := d.CreatePayment(context.Background(), paymentRequest()); !errors.Is(err, stripe.ErrUnavailable) {
			t.Fatalf("network: %v", err)
		}
	})
	t.Run("bad key", func(t *testing.T) {
		d, _, _ := newDriver(t)
		req := paymentRequest()
		req.Credentials = ext.Credentials{stripe.CredSecretKey: "sk_test_wrong"}
		if _, err := d.CreatePayment(context.Background(), req); !errors.Is(err, stripe.ErrAuthentication) {
			t.Fatalf("bad key: %v", err)
		}
	})
	t.Run("missing key", func(t *testing.T) {
		d, _, _ := newDriver(t)
		req := paymentRequest()
		req.Credentials = ext.Credentials{}
		if _, err := d.CreatePayment(context.Background(), req); !errors.Is(err, stripe.ErrMissingCredentials) {
			t.Fatalf("missing key: %v", err)
		}
	})
	t.Run("idempotency", func(t *testing.T) {
		d, _, c := newDriver(t)
		if _, err := d.CreatePayment(context.Background(), paymentRequest()); err != nil {
			t.Fatal(err)
		}
		// Same key, later hold: different parameters.
		req := paymentRequest()
		req.Metadata["expires_at"] = c.Now().Add(50 * time.Minute).Format(time.RFC3339)
		_, err := d.CreatePayment(context.Background(), req)
		var ic interface{ IdempotencyConflict() bool }
		if !errors.As(err, &ic) || !ic.IdempotencyConflict() {
			t.Fatalf("idempotency: %v", err)
		}
	})
	t.Run("replay", func(t *testing.T) {
		d, _, _ := newDriver(t)
		a, err := d.CreatePayment(context.Background(), paymentRequest())
		if err != nil {
			t.Fatal(err)
		}
		b, err := d.CreatePayment(context.Background(), paymentRequest())
		if err != nil || b.ProviderRef != a.ProviderRef {
			t.Fatalf("replay: %v %s %s", err, a.ProviderRef, b.ProviderRef)
		}
	})
	t.Run("refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"type":"card_error","code":"card_declined","message":"Your card mario@example.org was declined"}}`))
		}))
		defer srv.Close()
		zero := int64(0)
		d := stripe.New(stripe.Options{BaseURL: srv.URL, Clock: clock.NewFake(t0), MaxNetworkRetries: &zero})
		_, err := d.CreatePayment(context.Background(), paymentRequest())
		var ae *stripe.APIError
		if !errors.As(err, &ae) || ae.ProviderCode() != "card_declined" || strings.Contains(err.Error(), "mario") {
			t.Fatalf("refused: %v", err)
		}
	})
}

// TestRefund_ResolvesSessionAndMapsStatus: a refund of a Checkout Session
// targets its PaymentIntent, idempotently; statuses map to ext.
func TestRefund_ResolvesSessionAndMapsStatus(t *testing.T) {
	d, srv, _ := newDriver(t)
	res, err := d.CreatePayment(context.Background(), paymentRequest())
	if err != nil {
		t.Fatal(err)
	}
	srv.Pay(res.ProviderRef)
	req := ext.RefundRequest{EstablishmentID: "est-1", PaymentID: "pm-1", ProviderRef: res.ProviderRef, Amount: 1000, IdempotencyKey: "r-1", Credentials: creds()}
	out, err := d.Refund(context.Background(), req)
	if err != nil || out.Status != ext.RefundSucceeded || !strings.HasPrefix(out.ProviderRef, "re_") {
		t.Fatalf("refund: %v %+v", err, out)
	}
	again, err := d.Refund(context.Background(), req)
	if err != nil || again.ProviderRef != out.ProviderRef {
		t.Fatalf("idempotent refund: %v %+v", err, again)
	}
	pi := srv.Session(res.ProviderRef).PaymentIntent
	if n := len(srv.RefundsOf(pi)); n != 1 {
		t.Fatalf("refunds at provider: %d", n)
	}
	srv.RefundStatus = "pending"
	req.IdempotencyKey, req.Amount = "r-2", 500
	if out, err = d.Refund(context.Background(), req); err != nil || out.Status != ext.RefundPending {
		t.Fatalf("pending refund: %v %+v", err, out)
	}
	req.IdempotencyKey, req.Amount = "r-3", 5000
	var ae *stripe.APIError
	if _, err = d.Refund(context.Background(), req); !errors.As(err, &ae) || ae.Code != "amount_too_large" {
		t.Fatalf("too large: %v", err)
	}
}

// TestPaymentStatus_Lookups: sessions, payment intents and refunds report
// their outcome, nil while open.
func TestPaymentStatus_Lookups(t *testing.T) {
	d, srv, _ := newDriver(t)
	ctx := context.Background()
	res, err := d.CreatePayment(ctx, paymentRequest())
	if err != nil {
		t.Fatal(err)
	}
	if ev, err := d.PaymentStatus(ctx, res.ProviderRef, creds()); err != nil || ev != nil {
		t.Fatalf("open session: %v %+v", err, ev)
	}
	srv.Pay(res.ProviderRef)
	ev, err := d.PaymentStatus(ctx, res.ProviderRef, creds())
	if err != nil || ev.Type != ext.PaymentSucceeded || ev.Amount != 4250 || ev.BookingID != "bk-1" || ev.Metadata[stripe.MetaPaymentIntent] == "" {
		t.Fatalf("paid session: %v %+v", err, ev)
	}
	pi := ev.Metadata[stripe.MetaPaymentIntent]
	if ev, err := d.PaymentStatus(ctx, pi, creds()); err != nil || ev.Type != ext.PaymentSucceeded {
		t.Fatalf("payment intent: %v %+v", err, ev)
	}
	srv.RefundStatus = "pending"
	rf, err := d.Refund(ctx, ext.RefundRequest{ProviderRef: pi, Amount: 100, IdempotencyKey: "x", Credentials: creds()})
	if err != nil {
		t.Fatal(err)
	}
	if ev, err := d.PaymentStatus(ctx, rf.ProviderRef, creds()); err != nil || ev != nil {
		t.Fatalf("pending refund: %v %+v", err, ev)
	}
	srv.SetRefundStatus(rf.ProviderRef, "succeeded")
	if ev, err := d.PaymentStatus(ctx, rf.ProviderRef, creds()); err != nil || ev.Type != ext.PaymentRefunded || ev.Metadata[stripe.MetaOutcome] != stripe.OutcomeRefundSingle || ev.Amount != 100 {
		t.Fatalf("succeeded refund: %v %+v", err, ev)
	}
	res2, _ := d.CreatePayment(ctx, func() ext.CreatePaymentRequest { r := paymentRequest(); r.IdempotencyKey = "bk-1:2"; return r }())
	srv.ExpireSession(res2.ProviderRef)
	if ev, err := d.PaymentStatus(ctx, res2.ProviderRef, creds()); err != nil || ev.Type != ext.PaymentFailed || ev.Metadata[stripe.MetaOutcome] != stripe.OutcomeExpired {
		t.Fatalf("expired session: %v %+v", err, ev)
	}
}

// TestCancelPayment_ExpiresOpenSessionOnly: an open session is expired, a
// closed one is refused.
func TestCancelPayment_ExpiresOpenSessionOnly(t *testing.T) {
	d, srv, _ := newDriver(t)
	ctx := context.Background()
	res, _ := d.CreatePayment(ctx, paymentRequest())
	if err := d.CancelPayment(ctx, res.ProviderRef, creds()); err != nil || srv.Session(res.ProviderRef).Status != "expired" {
		t.Fatalf("cancel: %v %+v", err, srv.Session(res.ProviderRef))
	}
	// No longer open: refused, the caller keeps its record.
	var ae *stripe.APIError
	if err := d.CancelPayment(ctx, res.ProviderRef, creds()); !errors.As(err, &ae) {
		t.Fatalf("cancel twice: %v", err)
	}
}

// TestVerifyCredentials_Label.
func TestVerifyCredentials_Label(t *testing.T) {
	d, _, _ := newDriver(t)
	label, err := d.VerifyCredentials(context.Background(), creds())
	if err != nil || label != "Lido Test (test)" {
		t.Fatalf("label %q: %v", label, err)
	}
	if _, err := d.VerifyCredentials(context.Background(), ext.Credentials{stripe.CredSecretKey: "sk_live_nope"}); !errors.Is(err, stripe.ErrAuthentication) {
		t.Fatalf("bad key: %v", err)
	}
	for key, want := range map[string]string{"sk_live_x": "live", "rk_test_x": "test", "pk_test_x": ""} {
		if got := stripe.KeyMode(key); got != want {
			t.Errorf("KeyMode(%s) = %q", key, got)
		}
	}
}
