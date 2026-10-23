//go:build integration

package gateway_test

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// TestPaymentWebhook_ConfirmsBooking (INV-19): a verified
// checkout.session.completed records the ledger row, sets paid_amount,
// confirms the booking through the booking service (hold allocations
// become booking allocations) and writes audit + outbox, all in the
// transaction that records the event.
func TestPaymentWebhook_ConfirmsBooking(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		if res.Mode != ext.PaymentModeRedirect || res.RedirectURL == "" || res.Amount != b.TotalAmount {
			t.Fatalf("start: %+v", res)
		}
		pi := e.intent(res.IntentID)
		if pi.Status != store.IntentPending || pi.Attempt != 1 || *pi.IdempotencyKey != b.ID+":1" || pi.Mode != store.ProviderModeTest {
			t.Fatalf("intent: %+v", pi)
		}
		cs := e.srv.Session(pi.ExternalID)
		if cs.Metadata["intent_id"] != pi.ID || cs.Metadata["booking_id"] != b.ID || cs.Metadata["establishment_id"] != e.f.Est.ID || cs.Locale != "it" {
			t.Fatalf("session metadata: %+v", cs)
		}
		// The session is open at least 30 minutes (Stripe) although the hold is 15.
		if got := time.Unix(cs.ExpiresAt, 0).Sub(e.f.Clock.Now()); got < 30*time.Minute {
			t.Fatalf("session lifetime %v", got)
		}
		if n := e.count("outbox_events", "type = ?", "payment.pending"); n != 1 {
			t.Fatalf("payment.pending events: %d", n)
		}
		// Polling the status never confirms (only the webhook does).
		if _, err := e.gw.LatestForBooking(e.ctx, db, e.es, b.ID); err != nil || e.reload(b.ID).Status != "held" {
			t.Fatalf("polling changed the booking: %v", err)
		}

		out := e.mustDeliver(e.paidEvent(pi.ExternalID))
		if out.Duplicate || out.Ignored {
			t.Fatalf("outcome: %+v", out)
		}
		got := e.reload(b.ID)
		if got.Status != "confirmed" || got.HoldExpiresAt != nil || got.PaidAmount != b.TotalAmount {
			t.Fatalf("booking: %+v", got)
		}
		if e.ledgerOf(b.ID) != b.TotalAmount {
			t.Fatal("ledger")
		}
		if k := e.allocKinds(b.ID); k[store.AllocationHold] != 0 || k[store.AllocationBooking] == 0 {
			t.Fatalf("allocations: %v", k)
		}
		pi = e.intent(res.IntentID)
		if pi.Status != store.IntentSucceeded || pi.PaymentID == nil || pi.ExternalRef == nil || len(pi.RawEventIDs) != 1 {
			t.Fatalf("intent after: %+v", pi)
		}
		pm, err := store.GetPayment(e.ctx, db, e.es, *pi.PaymentID)
		if err != nil || pm.Provider != stripe.Code || *pm.ProviderRef != pi.ExternalID || pm.Amount != b.TotalAmount || pm.CashSessionID != nil {
			t.Fatalf("ledger row: %v %+v", err, pm)
		}
		for _, typ := range []string{"booking.confirmed", "payment.succeeded"} {
			if n := e.count("outbox_events", "type = ?", typ); n != 1 {
				t.Fatalf("%s events: %d", typ, n)
			}
		}
		if n := e.count("audit_log", "action = ? AND entity_id = ?", "payment.recorded", pm.ID); n != 1 {
			t.Fatalf("payment audit: %d", n)
		}
		if n := e.count("audit_log", "entity_id = ? AND action LIKE ?", b.ID, "%confirm%"); n != 1 {
			t.Fatalf("booking confirmation audit: %d", n)
		}
		if n := e.count("webhook_events", "provider = ? AND status = ?", stripe.Code, store.WebhookEventProcessed); n != 1 {
			t.Fatalf("webhook events: %d", n)
		}
	})
}

// TestPaymentWebhook_AtomicRollback (INV-18): a failure after the ledger
// and the confirmation leaves nothing behind (no event row, booking still
// held, no ledger row, no audit, no outbox), so the provider's retry
// applies the event from scratch.
func TestPaymentWebhook_AtomicRollback(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		payload := e.paidEvent(e.sessionOf(res))
		outbox := e.count("outbox_events", "1 = 1")
		audits := e.count("audit_log", "1 = 1")
		e.ledger.failAfterPayment.Store(true)
		if _, err := e.deliver(payload); !errors.Is(err, errInjected) {
			t.Fatalf("want injected failure, got %v", err)
		}
		if got := e.reload(b.ID); got.Status != "held" || got.PaidAmount != 0 {
			t.Fatalf("booking changed: %+v", got)
		}
		if e.count("payments", "booking_id = ?", b.ID) != 0 || e.count("webhook_events", "1 = 1") != 0 ||
			e.count("outbox_events", "1 = 1") != outbox || e.count("audit_log", "1 = 1") != audits {
			t.Fatal("partial writes after rollback")
		}
		if e.intent(res.IntentID).Status != store.IntentPending {
			t.Fatal("intent changed")
		}
		// The retry of the same event succeeds.
		e.ledger.failAfterPayment.Store(false)
		if out := e.mustDeliver(payload); out.Duplicate || e.reload(b.ID).Status != "confirmed" {
			t.Fatalf("retry: %+v", out)
		}
	})
}

// TestPaymentWebhook_Deduplicated (INV-23): the same event delivered again
// is acknowledged as a duplicate with no effect; a different event about
// the same payment (payment_intent.succeeded) adds no ledger row.
func TestPaymentWebhook_Deduplicated(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		cs := e.sessionOf(res)
		payload := e.paidEvent(cs)
		e.mustDeliver(payload)
		for i := 0; i < 3; i++ {
			if out := e.mustDeliver(payload); !out.Duplicate {
				t.Fatalf("delivery %d not a duplicate", i+2)
			}
		}
		piEvt := e.srv.IntentEvent(e.evt(), "payment_intent.succeeded", e.srv.Session(cs).PaymentIntent, e.f.Clock.Now())
		if out := e.mustDeliver(piEvt); out.Duplicate || out.Ignored {
			t.Fatalf("payment_intent.succeeded: %+v", out)
		}
		if n := e.count("payments", "booking_id = ?", b.ID); n != 1 {
			t.Fatalf("ledger rows: %d", n)
		}
		if n := e.count("outbox_events", "type = ?", "booking.confirmed"); n != 1 {
			t.Fatalf("confirmations: %d", n)
		}
		if e.ledgerOf(b.ID) != b.TotalAmount || len(e.intent(res.IntentID).RawEventIDs) != 2 {
			t.Fatal("state after duplicates")
		}
	})
}

// TestPaymentWebhook_SignatureAndTarget: bad signatures, replays and
// unknown targets are refused before any write.
func TestPaymentWebhook_SignatureAndTarget(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		payload := e.paidEvent(e.sessionOf(res))
		now := e.f.Clock.Now()
		url := "/webhooks/payments/stripe/" + e.f.Est.ID

		bad := stripetest.WebhookRequest(url, payload, now, "whsec_wrong")
		if _, err := e.gw.HandleWebhook(e.ctx, stripe.Code, e.f.Est.ID, bad); !errors.Is(err, gateway.ErrSignatureInvalid) || !errors.Is(err, ext.ErrInvalidSignature) {
			t.Fatalf("wrong secret: %v", err)
		}
		// A delivery captured 6 minutes ago and replayed now.
		old := stripetest.WebhookRequest(url, payload, now.Add(-6*time.Minute), stripetest.WebhookSecret)
		if _, err := e.gw.HandleWebhook(e.ctx, stripe.Code, e.f.Est.ID, old); !errors.Is(err, gateway.ErrSignatureInvalid) {
			t.Fatalf("replay: %v", err)
		}
		unsigned := httptest.NewRequest("POST", url, bytes.NewReader(payload))
		if _, err := e.gw.HandleWebhook(e.ctx, stripe.Code, e.f.Est.ID, unsigned); !errors.Is(err, gateway.ErrSignatureInvalid) {
			t.Fatalf("unsigned: %v", err)
		}
		for name, tc := range map[string][2]string{
			"unknown establishment": {stripe.Code, store.NewID()},
			"not a uuid":            {stripe.Code, "../../etc"},
			"unknown provider":      {"paypal", e.f.Est.ID},
		} {
			r := stripetest.WebhookRequest(url, payload, now, stripetest.WebhookSecret)
			if _, err := e.gw.HandleWebhook(e.ctx, tc[0], tc[1], r); !errors.Is(err, gateway.ErrWebhookTarget) || !errors.Is(err, configerr.ErrNotFound) {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if e.count("webhook_events", "1 = 1") != 0 || e.reload(b.ID).Status != "held" {
			t.Fatal("refused deliveries wrote something")
		}
		// An authentic event of an unhandled type is acknowledged and ignored.
		other := stripetest.Event(e.evt(), "customer.created", now, map[string]any{"id": "cus_1", "object": "customer"})
		if out := e.mustDeliver(other); !out.Ignored {
			t.Fatalf("unhandled type: %+v", out)
		}
	})
}

// TestPaymentWebhook_OutOfOrder: late "expired" never undoes a success; a
// refund of a charge whose payment intent is not known yet is refused with
// 503 (the provider retries) and settled after the success; a success
// after "expired" is still applied (the money arrived).
func TestPaymentWebhook_OutOfOrder(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		now := func() time.Time { return e.f.Clock.Now() }

		// completed, then a stale expired.
		b1 := e.held(0, "2027-07-10")
		r1 := e.start(b1)
		cs1 := e.sessionOf(r1)
		completed := e.paidEvent(cs1)
		expired := stripetest.Event(e.evt(), "checkout.session.expired", now(), map[string]any{
			"id": cs1, "object": "checkout.session", "amount_total": b1.TotalAmount, "currency": "eur", "status": "expired",
			"payment_status": "unpaid", "metadata": map[string]string{"establishment_id": e.f.Est.ID, "booking_id": b1.ID},
		})
		e.mustDeliver(completed)
		e.mustDeliver(expired)
		if pi := e.intent(r1.IntentID); pi.Status != store.IntentSucceeded || e.reload(b1.ID).Status != "confirmed" {
			t.Fatalf("expired after success: %+v", pi)
		}

		// charge.refunded (partial, from the dashboard) before completed.
		b2 := e.held(1, "2027-07-10")
		r2 := e.start(b2)
		cs2 := e.sessionOf(r2)
		e.srv.Pay(cs2)
		piID := e.srv.Session(cs2).PaymentIntent
		e.srv.RefundDashboard(piID, 500)
		refunded := e.srv.ChargeRefundedEvent(e.evt(), piID, now())
		completed2 := e.srv.SessionEvent(e.evt(), "checkout.session.completed", cs2, now())
		// The charge names a payment intent not stored yet: 503, the
		// provider retries, nothing is recorded.
		if _, err := e.deliver(refunded); !errors.Is(err, gateway.ErrProviderUnavailable) {
			t.Fatalf("refund before success: %v", err)
		}
		if e.count("webhook_events", "1 = 1") != 2 || e.count("payments", "booking_id = ?", b2.ID) != 0 {
			t.Fatal("refund before success recorded something")
		}
		e.mustDeliver(completed2)
		e.mustDeliver(refunded) // the provider's retry
		e.mustDeliver(refunded) // duplicate
		if n := e.count("payments", "booking_id = ? AND direction = ?", b2.ID, store.PaymentDirectionRefund); n != 1 || e.ledgerOf(b2.ID) != b2.TotalAmount-500 {
			t.Fatalf("after completed: %d refund rows", n)
		}

		// expired first, then the success (paid at the last second).
		b3 := e.held(2, "2027-07-10")
		r3 := e.start(b3)
		cs3 := e.sessionOf(r3)
		exp3 := stripetest.Event(e.evt(), "checkout.session.expired", now(), map[string]any{
			"id": cs3, "object": "checkout.session", "amount_total": b3.TotalAmount, "currency": "eur", "status": "expired",
			"payment_status": "unpaid", "metadata": map[string]string{"establishment_id": e.f.Est.ID, "booking_id": b3.ID},
		})
		e.mustDeliver(exp3)
		if pi := e.intent(r3.IntentID); pi.Status != store.IntentExpired || e.count("outbox_events", "type = ?", "payment.failed") != 1 {
			t.Fatalf("expired: %+v", pi)
		}
		e.mustDeliver(e.paidEvent(cs3))
		if pi := e.intent(r3.IntentID); pi.Status != store.IntentSucceeded || e.reload(b3.ID).Status != "confirmed" {
			t.Fatalf("success after expired: %+v", pi)
		}
	})
}

// TestPaymentWebhook_FailedAttempt: a declined card keeps the session
// payable (intent pending, reason saved, booking untouched); an async
// failure closes the intent with payment.failed.
func TestPaymentWebhook_FailedAttempt(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		cs := e.sessionOf(res)
		piID := e.srv.Session(cs).PaymentIntent
		failed := stripetest.Event(e.evt(), "payment_intent.payment_failed", e.f.Clock.Now(), map[string]any{
			"id": piID, "object": "payment_intent", "amount": b.TotalAmount, "currency": "eur", "status": "requires_payment_method",
			"metadata":           map[string]string{"establishment_id": e.f.Est.ID, "booking_id": b.ID, "intent_id": res.IntentID},
			"last_payment_error": map[string]any{"type": "card_error", "code": "card_declined"},
		})
		e.mustDeliver(failed)
		pi := e.intent(res.IntentID)
		if pi.Status != store.IntentPending || pi.FailureReason == nil || *pi.FailureReason != "card_declined" || e.reload(b.ID).Status != "held" {
			t.Fatalf("attempt failed: %+v", pi)
		}
		async := stripetest.Event(e.evt(), "checkout.session.async_payment_failed", e.f.Clock.Now(), map[string]any{
			"id": cs, "object": "checkout.session", "amount_total": b.TotalAmount, "currency": "eur", "status": "complete",
			"payment_status": "unpaid", "metadata": map[string]string{"establishment_id": e.f.Est.ID, "booking_id": b.ID},
		})
		e.mustDeliver(async)
		if pi := e.intent(res.IntentID); pi.Status != store.IntentFailed || e.count("outbox_events", "type = ?", "payment.failed") != 1 {
			t.Fatalf("async failure: %+v", pi)
		}
		if e.reload(b.ID).Status != "held" || e.count("payments", "booking_id = ?", b.ID) != 0 {
			t.Fatal("a failure must not touch the booking or the ledger")
		}
	})
}

// lateConfirmer is a test LateConfirmer: ok confirms the booking (as the
// booking service would after re-allocating), otherwise it writes then
// fails, to prove the savepoint discards its writes.
type lateConfirmer struct{ ok bool }

func (l lateConfirmer) ConfirmLatePayment(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, bookingID string, _ *store.Payment) error {
	if _, err := tx.NewUpdate().TableExpr("bookings").Set("notes_internal = 'touched'").
		Where("id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx); err != nil {
		return err
	}
	if !l.ok {
		return errors.New("spot taken")
	}
	_, err := tx.NewUpdate().TableExpr("bookings").Set("status = 'confirmed'").
		Where("id = ?", bookingID).Where("establishment_id = ?", es.ID()).Exec(ctx)
	return err
}

// TestPaymentWebhook_LatePayment (ADR 0014): a payment that succeeds after
// the hold expired is recorded; if the booking cannot be confirmed again
// (no LateConfirmer, or it fails) the payment is refunded automatically
// with payment.refund_scheduled; otherwise the booking is confirmed.
func TestPaymentWebhook_LatePayment(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lc      gateway.LateConfirmer
		confirm bool
	}{
		{"no late confirmer: refunded", nil, false},
		{"resources taken: refunded", lateConfirmer{ok: false}, false},
		{"resources free: confirmed", lateConfirmer{ok: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storetest.Each(t, func(t *testing.T, db *store.DB) {
				e := newEnv(t, db)
				if tc.lc != nil {
					e.gw.SetLateConfirmer(tc.lc)
				}
				b := e.held(0, "2027-07-10")
				res := e.start(b)
				cs := e.sessionOf(res)
				e.f.Clock.Advance(16 * time.Minute)
				if n, err := e.bookings.ExpireHolds(e.ctx); err != nil || n != 1 || e.reload(b.ID).Status != "expired" {
					t.Fatalf("expire holds: %d %v", n, err)
				}
				e.mustDeliver(e.paidEvent(cs))
				got := e.reload(b.ID)
				refunds := e.srv.RefundsOf(e.srv.Session(cs).PaymentIntent)
				if tc.confirm {
					if got.Status != "confirmed" || got.PaidAmount != b.TotalAmount || len(refunds) != 0 {
						t.Fatalf("late confirm: %+v refunds %d", got, len(refunds))
					}
					return
				}
				if got.Status != "expired" || got.NotesInternal == "touched" {
					t.Fatalf("booking: %+v", got)
				}
				if len(refunds) != 1 || refunds[0].Amount != int64(b.TotalAmount) {
					t.Fatalf("provider refunds: %+v", refunds)
				}
				if paid := e.ledgerOf(b.ID); paid != 0 {
					t.Fatalf("paid after automatic refund: %d", paid)
				}
				if e.count("outbox_events", "type = ?", gateway.EventRefundScheduled) != 1 ||
					e.count("payment_intents", "kind = ? AND status = ?", store.IntentKindRefund, store.IntentSucceeded) != 1 {
					t.Fatal("refund scheduling")
				}
				// A redelivery schedules nothing more.
				e.mustDeliver(e.srv.SessionEvent(e.evt(), "checkout.session.async_payment_succeeded", cs, e.f.Clock.Now()))
				if len(e.srv.RefundsOf(e.srv.Session(cs).PaymentIntent)) != 1 || e.count("payment_intents", "kind = ?", store.IntentKindRefund) != 1 {
					t.Fatal("second refund scheduled")
				}
			})
		})
	}
}

// TestPaymentWebhook_DoublePaymentRefundsExcess: two attempts paid (the
// first session could not be expired in time) -> the second payment is
// above the total and refunded automatically.
func TestPaymentWebhook_DoublePaymentRefundsExcess(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		r1 := e.start(b)
		cs1 := e.sessionOf(r1)
		e.srv.Pay(cs1) // paid, webhook not delivered yet
		r2 := e.start(b)
		if a := e.intent(r2.IntentID).Attempt; a != 2 {
			t.Fatalf("attempt %d", a)
		}
		// cs1 was complete: it could not be expired, the intent stays pending.
		if e.intent(r1.IntentID).Status != store.IntentPending {
			t.Fatal("paid session marked cancelled")
		}
		e.mustDeliver(e.srv.SessionEvent(e.evt(), "checkout.session.completed", cs1, e.f.Clock.Now()))
		e.mustDeliver(e.paidEvent(e.sessionOf(r2)))
		if paid := e.ledgerOf(b.ID); paid != b.TotalAmount {
			t.Fatalf("paid after excess refund: %d", paid)
		}
		if e.reload(b.ID).Status != "confirmed" || len(e.srv.RefundsOf(e.srv.Session(e.sessionOf(r2)).PaymentIntent)) != 1 {
			t.Fatal("excess not refunded")
		}
	})
}

// TestPaymentWebhook_CrossTenant (INV-26): an event of establishment A
// delivered to the URL of B never touches A's data.
func TestPaymentWebhook_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		payload := e.paidEvent(e.sessionOf(res))
		out, err := e.deliverTo(e.f.Other.ID, payload)
		if err != nil || !out.Ignored {
			t.Fatalf("cross tenant: %v %+v", err, out)
		}
		if e.reload(b.ID).Status != "held" || e.count("payments", "1 = 1") != 0 {
			t.Fatal("cross-tenant delivery changed A")
		}
		// One Stripe account, two establishments: the same event then
		// reaches A's own endpoint and must be applied (not a duplicate).
		out, err = e.deliver(payload)
		if err != nil || out.Duplicate || out.Ignored {
			t.Fatalf("redelivery to the owner: %v %+v", err, out)
		}
		if e.reload(b.ID).Status != "confirmed" {
			t.Fatal("booking not confirmed by the delivery to its own establishment")
		}
		if e.count("webhook_events", "establishment_id = ?", e.f.Other.ID) != 0 {
			t.Fatal("event of another establishment must not be recorded")
		}
	})
}

// TestStart_AttemptsAndProvider: a new attempt expires the previous open
// session; a retry after the caller's rollback reuses the same provider
// session; without an online provider Start refuses.
func TestStart_AttemptsAndProvider(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		r1 := e.start(b)
		r2 := e.start(b)
		if e.intent(r1.IntentID).Status != store.IntentCancelled || e.srv.Session(e.sessionOf(r1)).Status != "expired" {
			t.Fatal("previous attempt not closed")
		}
		if pi := e.intent(r2.IntentID); pi.Attempt != 2 || *pi.IdempotencyKey != b.ID+":2" {
			t.Fatalf("attempt 2: %+v", pi)
		}
		// Rolled back after the provider call: the retry gets the same session.
		var first string
		_ = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			res, err := e.gw.Start(ctx, tx, e.es, gateway.StartRequest{BookingID: b.ID, Amount: b.TotalAmount, Purpose: gateway.PurposeFull})
			if err != nil {
				return err
			}
			pi, _ := store.GetPaymentIntent(ctx, tx, e.es, res.IntentID)
			first = pi.ExternalID
			return errors.New("rollback")
		})
		r3 := e.start(b)
		if got := e.sessionOf(r3); got == "" || (got != first && e.intent(r3.IntentID).Attempt != 3) {
			t.Fatalf("retry: %s vs %s", got, first)
		}
		// Embedded mode returns the client secret and the publishable key.
		e.settings.embedded = true
		b4 := e.held(1, "2027-07-11")
		r4 := e.start(b4)
		if r4.Mode != ext.PaymentModeEmbedded || r4.ClientSecret == "" || r4.PublishableKey != stripetest.PublishableKey {
			t.Fatalf("embedded: %+v", r4)
		}
		e.settings.manual.Store(true)
		err := store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			_, err := e.gw.Start(ctx, tx, e.es, gateway.StartRequest{BookingID: b.ID, Amount: b.TotalAmount})
			return err
		})
		if !errors.Is(err, gateway.ErrProviderNotConfigured) {
			t.Fatalf("manual provider: %v", err)
		}
		e.settings.manual.Store(false)
		b5 := e.held(2, "2027-07-12")
		e.srv.FailNext(1)
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			_, err := e.gw.Start(ctx, tx, e.es, gateway.StartRequest{BookingID: b5.ID, Amount: 1000})
			return err
		})
		if !errors.Is(err, gateway.ErrProviderUnavailable) {
			t.Fatalf("outage: %v", err)
		}
	})
}
