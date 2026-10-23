//go:build integration

package gateway_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// paid returns a booking confirmed by a Stripe payment and the ledger id
// of that payment.
func (e *env) paid(spot int, day string) (*store.Booking, string, *store.PaymentIntent) {
	e.t.Helper()
	b := e.held(spot, day)
	res := e.start(b)
	e.mustDeliver(e.paidEvent(e.sessionOf(res)))
	pi := e.intent(res.IntentID)
	if pi.PaymentID == nil {
		e.t.Fatal("payment not recorded")
	}
	return e.reload(b.ID), *pi.PaymentID, pi
}

// TestRefundViaProvider_CapsAndIdempotency (INV-24): partial refunds
// confirmed synchronously write the ledger row at once; a refund above
// the residual is refused before calling the provider; the idempotency
// key replays the same refund; the permission is checked.
func TestRefundViaProvider_CapsAndIdempotency(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b, pmID, _ := e.paid(0, "2027-07-10")
		total := b.TotalAmount
		ri, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{
			BookingID: b.ID, PaymentID: pmID, Amount: 1000, Reason: "Pioggia", IdempotencyKey: "rf-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if ri.Status != store.IntentSucceeded || ri.PaymentID == nil || ri.ExternalID[:3] != "re_" {
			t.Fatalf("refund intent: %+v", ri)
		}
		if paid := e.ledgerOf(b.ID); paid != total-1000 {
			t.Fatalf("paid %d", paid)
		}
		// Replay with the same key: same refund, nothing new at the provider.
		again, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: 1000, IdempotencyKey: "rf-1"})
		if err != nil || again.ID != ri.ID || len(e.srv.Requests()) == 0 {
			t.Fatalf("replay: %v %+v", err, again)
		}
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: 999, IdempotencyKey: "rf-1"}); !errors.Is(err, gateway.ErrIdempotencyMismatch) {
			t.Fatalf("key reuse: %v", err)
		}
		before := len(e.srv.Requests())
		var ve *configerr.ValidationError
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: total - 999}); !errors.As(err, &ve) || ve.Fields[0].Field != "amount" {
			t.Fatalf("above residual: %v", err)
		}
		if len(e.srv.Requests()) != before {
			t.Fatal("provider called for a refused refund")
		}
		// The rest refunds exactly.
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: total - 1000}); err != nil {
			t.Fatal(err)
		}
		if paid := e.ledgerOf(b.ID); paid != 0 {
			t.Fatalf("paid after full refund: %d", paid)
		}
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: 1}); !errors.As(err, &ve) {
			t.Fatalf("nothing left: %v", err)
		}
		// Permissions and validation.
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Viewer, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: 1}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer: %v", err)
		}
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Outsider, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: pmID, Amount: 1}); err == nil {
			t.Fatal("outsider refunded")
		}
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: b.ID, PaymentID: store.NewID(), Amount: 1}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("not an online payment: %v", err)
		}
		if _, err := e.gw.RefundBooking(e.ctx, e.f.Manager, e.f.Est.ID, gateway.RefundRequest{BookingID: store.NewID(), PaymentID: pmID, Amount: 1}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("other booking: %v", err)
		}
		if n := e.count("audit_log", "action = ?", "payment.refund_requested"); n != 2 {
			t.Fatalf("refund audits: %d", n)
		}
	})
}

// TestRefundViaProvider_PendingThenWebhook: a refund Stripe keeps pending
// reserves its amount (a second refund cannot exceed what is left) and is
// written to the ledger when charge.refunded reports it; a failed refund
// releases the amount.
func TestRefundViaProvider_PendingThenWebhook(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b, pmID, pay := e.paid(0, "2027-07-10")
		total := b.TotalAmount
		e.srv.RefundStatus = "pending"
		ri, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: total - 500})
		if err != nil || ri.Status != store.IntentPending {
			t.Fatalf("pending refund: %v %+v", err, ri)
		}
		if paid := e.ledgerOf(b.ID); paid != total {
			t.Fatalf("pending refund touched the ledger: %d", paid)
		}
		var ve *configerr.ValidationError
		if _, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: 501}); !errors.As(err, &ve) {
			t.Fatalf("over the reserved amount: %v", err)
		}
		// Stripe confirms: charge.refunded with the cumulative amount.
		e.srv.SetRefundStatus(ri.ExternalID, "succeeded")
		piID := *pay.ExternalRef
		e.mustDeliver(e.srv.ChargeRefundedEvent(e.evt(), piID, e.f.Clock.Now()))
		if got := e.intent(ri.ID); got.Status != store.IntentSucceeded || got.PaymentID == nil {
			t.Fatalf("refund after webhook: %+v", got)
		}
		if paid := e.ledgerOf(b.ID); paid != 500 {
			t.Fatalf("paid after webhook: %d", paid)
		}
		// A failing refund: refund.failed releases the amount.
		r2, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: 500})
		if err != nil || r2.Status != store.IntentPending {
			t.Fatalf("second refund: %v %+v", err, r2)
		}
		e.srv.SetRefundStatus(r2.ExternalID, "failed")
		e.mustDeliver(e.srv.RefundEvent(e.evt(), "refund.failed", r2.ExternalID, e.f.Clock.Now()))
		if got := e.intent(r2.ID); got.Status != store.IntentFailed {
			t.Fatalf("failed refund: %+v", got)
		}
		e.srv.RefundStatus = "succeeded"
		if _, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: 500}); err != nil {
			t.Fatalf("amount released by the failure: %v", err)
		}
		if paid := e.ledgerOf(b.ID); paid != 0 {
			t.Fatalf("paid at the end: %d", paid)
		}
	})
}

// TestRefundViaProvider_OutageRetriedByReconciliation: with Stripe down
// the refund intent stays created (returned as pending); the
// reconciliation sends it with the same idempotency key, exactly once.
func TestRefundViaProvider_OutageRetriedByReconciliation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b, pmID, pay := e.paid(0, "2027-07-10")
		e.srv.FailNext(1)
		ri, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: 700, IdempotencyKey: "cancel-1"})
		if err != nil || ri.Status != store.IntentCreated {
			t.Fatalf("refund during outage: %v %+v", err, ri)
		}
		e.f.Clock.Advance(2 * time.Minute)
		st, err := e.gw.Reconcile(e.ctx)
		if err != nil || st.Updated != 1 || st.Failed != 0 {
			t.Fatalf("reconcile: %v %+v", err, st)
		}
		if got := e.intent(ri.ID); got.Status != store.IntentSucceeded {
			t.Fatalf("refund after reconcile: %+v", got)
		}
		if n := len(e.srv.RefundsOf(*pay.ExternalRef)); n != 1 {
			t.Fatalf("provider refunds: %d", n)
		}
		if paid := e.ledgerOf(b.ID); paid != b.TotalAmount-700 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestReconcile_FixesMissedWebhook: a payment whose webhook never arrived
// is applied by the reconciliation after PendingAfter (booking confirmed
// once); the late real webhook changes nothing; the report shows the
// mismatch before and nothing after.
func TestReconcile_FixesMissedWebhook(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		cs := e.sessionOf(res)
		e.srv.Pay(cs) // the webhook is lost
		open := e.held(1, "2027-07-10")
		e.start(open) // still open at the provider

		// Too young: the job leaves it alone.
		if st, err := e.gw.Reconcile(e.ctx); err != nil || st.Updated != 0 {
			t.Fatalf("young intents: %v %+v", err, st)
		}
		e.f.Clock.Advance(11 * time.Minute)
		rep, err := e.gw.Report(e.ctx, e.f.Viewer, e.f.Est.ID, gateway.ReportQuery{IssuesOnly: true})
		if err != nil || rep.Issues != 2 || len(rep.Items) != 2 {
			t.Fatalf("report before: %v %+v", err, rep)
		}
		issues := map[string]string{}
		for _, it := range rep.Items {
			issues[it.Intent.ID] = it.Issue + "/" + it.ProviderStatus
		}
		if issues[res.IntentID] != gateway.IssueStatusMismatch+"/succeeded" {
			t.Fatalf("issues: %v", issues)
		}

		st, err := e.gw.Reconcile(e.ctx)
		if err != nil || st.Checked != 2 || st.Updated != 1 || st.Failed != 0 {
			t.Fatalf("reconcile: %v %+v", err, st)
		}
		if got := e.reload(b.ID); got.Status != "confirmed" || got.PaidAmount != b.TotalAmount {
			t.Fatalf("booking after reconcile: %+v", got)
		}
		if pi := e.intent(res.IntentID); pi.LastCheckedAt == nil || pi.Status != store.IntentSucceeded {
			t.Fatalf("intent after reconcile: %+v", pi)
		}
		// The real webhook arrives late: nothing changes.
		e.mustDeliver(e.srv.SessionEvent(e.evt(), "checkout.session.completed", cs, e.f.Clock.Now()))
		if n := e.count("payments", "booking_id = ?", b.ID); n != 1 || e.count("outbox_events", "type = ?", "booking.confirmed") != 1 {
			t.Fatalf("late webhook: %d ledger rows", n)
		}
		// A second run is idempotent.
		if st, err := e.gw.Reconcile(e.ctx); err != nil || st.Updated != 0 {
			t.Fatalf("second run: %v %+v", err, st)
		}
		// Manual run from the panel (payments.configure): the open session
		// expires at Stripe and is closed.
		e.srv.ExpireSession(e.intent(e.latest(open.ID)).ExternalID)
		if _, err := e.gw.ReconcileEstablishment(e.ctx, e.f.Cashier, e.f.Est.ID); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier run: %v", err)
		}
		st, err = e.gw.ReconcileEstablishment(e.ctx, e.f.Owner, e.f.Est.ID)
		if err != nil || st.Updated != 1 {
			t.Fatalf("manual run: %v %+v", err, st)
		}
		rep, err = e.gw.Report(e.ctx, e.f.Viewer, e.f.Est.ID, gateway.ReportQuery{})
		if err != nil || rep.Issues != 0 || rep.Checked != 2 {
			t.Fatalf("report after: %v %+v", err, rep)
		}
		if _, err := e.gw.Report(e.ctx, e.f.Outsider, e.f.Est.ID, gateway.ReportQuery{}); err == nil {
			t.Fatal("outsider read the report")
		}
	})
}

func (e *env) latest(bookingID string) string {
	e.t.Helper()
	pi, err := e.gw.LatestForBooking(e.ctx, e.db, e.es, bookingID)
	if err != nil {
		e.t.Fatal(err)
	}
	return pi.ID
}

// TestConcurrency_WebhookDuplicate (INV-23): 100 parallel deliveries of
// the same event produce one ledger row, one confirmation, one processed
// event; every other delivery is acknowledged as a duplicate.
func TestConcurrency_WebhookDuplicate(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		payload := e.paidEvent(e.sessionOf(res))
		const n = 100
		var wg sync.WaitGroup
		var fresh, dup atomic.Int32
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := e.deliver(payload)
				switch {
				case err != nil:
					t.Error(err)
				case out.Duplicate:
					dup.Add(1)
				default:
					fresh.Add(1)
				}
			}()
		}
		wg.Wait()
		if fresh.Load() != 1 || dup.Load() != n-1 {
			t.Fatalf("fresh %d duplicate %d", fresh.Load(), dup.Load())
		}
		if e.count("payments", "booking_id = ?", b.ID) != 1 || e.count("webhook_events", "1 = 1") != 1 ||
			e.count("outbox_events", "type = ?", "booking.confirmed") != 1 || e.count("outbox_events", "type = ?", "payment.succeeded") != 1 {
			t.Fatal("duplicate effects")
		}
		if e.reload(b.ID).Status != "confirmed" || e.ledgerOf(b.ID) != b.TotalAmount {
			t.Fatal("final state")
		}
	})
}

// TestConcurrency_RefundViaProvider (INV-24): 100 parallel refunds of 1%
// of the payment plus the in-flight reservation never refund more than
// was captured, at the provider or in the ledger.
func TestConcurrency_RefundViaProvider(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b, pmID, pay := e.paid(0, "2027-07-10")
		part := b.TotalAmount / 50 // 50 refunds fit, 100 are attempted
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.gw.RefundViaProvider(e.ctx, e.es, gateway.RefundRequest{PaymentID: pmID, Amount: part})
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, configerr.ErrValidation):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 50 || refused.Load() != 50 {
			t.Fatalf("ok %d refused %d", ok.Load(), refused.Load())
		}
		var atProvider money.Cents
		for _, r := range e.srv.RefundsOf(*pay.ExternalRef) {
			atProvider += money.Cents(r.Amount)
		}
		if atProvider != part*50 || e.ledgerOf(b.ID) != b.TotalAmount-part*50 {
			t.Fatalf("provider %d ledger paid %d", atProvider, e.ledgerOf(b.ID))
		}
	})
}

// TestRefund_CreditCancellationCountsRefundsInFlight (INV-24): a double
// payment leaves 2T paid with the automatic overpayment refund of T still
// pending at the provider. A credit cancellation from the manage area
// must give back at most T (paid minus the refund in flight), so that when
// the provider confirms its refund the ledger still settles it and the
// money leaving never exceeds the money captured.
func TestRefund_CreditCancellationCountsRefundsInFlight(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		total := b.TotalAmount
		r1 := e.start(b)
		cs1 := e.sessionOf(r1)
		e.srv.Pay(cs1)
		r2 := e.start(b)
		e.srv.RefundStatus = "pending"
		e.mustDeliver(e.srv.SessionEvent(e.evt(), "checkout.session.completed", cs1, e.f.Clock.Now()))
		e.mustDeliver(e.paidEvent(e.sessionOf(r2)))
		if paid := e.reload(b.ID).PaidAmount; paid != 2*total {
			t.Fatalf("paid after the double payment: %d", paid)
		}
		if n := e.count("payment_intents", "kind = 'refund' AND status IN ('created', 'pending')"); n != 1 {
			t.Fatalf("overpayment refunds in flight: %d", n)
		}
		// The customer cancels with a credit: computed on T, not 2T.
		res, err := e.bookings.CancelAsCustomer(e.ctx, audit.SystemActor, e.es, b.ID, booking.CancelInput{Settlement: booking.SettleCredit})
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}
		if res.Refund.PaidAmount != total || res.Refund.Amount > total {
			t.Fatalf("refund preview must exclude the refund in flight: %+v", res.Refund)
		}
		// Stripe then confirms the overpayment refund: still within the paid.
		var ri store.PaymentIntent
		if err := db.NewSelect().Model(&ri).Where("kind = 'refund' AND booking_id = ?", b.ID).Scan(e.ctx); err != nil {
			t.Fatal(err)
		}
		e.srv.SetRefundStatus(ri.ExternalID, "succeeded")
		pay := e.intent(r2.IntentID)
		piID := pay.ExternalID
		if pay.ExternalRef != nil {
			piID = *pay.ExternalRef
		}
		e.mustDeliver(e.srv.ChargeRefundedEvent(e.evt(), piID, e.f.Clock.Now()))
		if got := e.intent(ri.ID); got.Status != store.IntentSucceeded {
			t.Fatalf("overpayment refund after the webhook: %+v", got)
		}
		if paid := e.reload(b.ID).PaidAmount; paid < 0 || paid != 2*total-total-res.Refund.Amount {
			t.Fatalf("paid at the end: %d (credit %d)", paid, res.Refund.Amount)
		}
		if out := e.ledgerOf(b.ID); out < 0 {
			t.Fatalf("ledger net negative: %d", out)
		}
	})
}
