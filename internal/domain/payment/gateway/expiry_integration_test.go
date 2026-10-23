//go:build integration

package gateway_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestPaymentWebhook_AfterHoldElapsedBeforeSweeper (INV-05, INV-51): a
// provider success delivered after hold_expires_at but before the hold
// sweeper ran is applied at once: the booking is expired in the same
// transaction (audit, outbox, resources released) and the payment follows
// the late-payment path (no LateConfirmer here: refunded automatically).
// The delivery is acknowledged, not retried, and the sweeper finds
// nothing left to do.
func TestPaymentWebhook_AfterHoldElapsedBeforeSweeper(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		res := e.start(b)
		cs := e.sessionOf(res)
		e.f.Clock.Advance(16 * time.Minute)
		if e.reload(b.ID).Status != "held" {
			t.Fatal("sweeper must not have run")
		}
		out := e.mustDeliver(e.paidEvent(cs))
		if out.Duplicate || out.Ignored {
			t.Fatalf("outcome %+v", out)
		}
		got := e.reload(b.ID)
		if got.Status != "expired" || got.HoldExpiresAt != nil {
			t.Fatalf("booking after the late delivery: %s", got.Status)
		}
		if n := e.allocKinds(b.ID); len(n) != 0 {
			t.Fatalf("allocations left: %v", n)
		}
		if refunds := e.srv.RefundsOf(e.srv.Session(cs).PaymentIntent); len(refunds) != 1 || refunds[0].Amount != int64(b.TotalAmount) {
			t.Fatalf("provider refunds: %+v", refunds)
		}
		if paid := e.ledgerOf(b.ID); paid != 0 || got.PaidAmount != 0 {
			t.Fatalf("money left on the expired booking: ledger %d paid %d", paid, got.PaidAmount)
		}
		if e.count("outbox_events", "type = ?", "booking.expired") != 1 || e.count("audit_log", "entity_id = ? AND action = ?", b.ID, "booking.expired") != 1 {
			t.Fatal("expiry must be audited once")
		}
		if n, err := e.bookings.ExpireHolds(e.ctx); err != nil || n != 0 {
			t.Fatalf("sweeper after the fact: %d %v", n, err)
		}
	})
}

// TestPaymentWebhook_BelowThresholdThenExpiry (INV-22, INV-51): the provider
// is paid less than the confirmation threshold (the total changed after
// the payment started). The booking stays unconfirmed with money on it;
// when its hold expires the money is refunded: the sweeper schedules the
// refund in the expiry transaction and the reconciliation sends it.
func TestPaymentWebhook_BelowThresholdThenExpiry(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.held(0, "2027-07-10")
		stale := b.TotalAmount - 500
		var res *gateway.StartResult
		err := store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			var err error
			res, err = e.gw.Start(ctx, tx, e.es, gateway.StartRequest{BookingID: b.ID, Amount: stale, Purpose: gateway.PurposeFull})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		cs := e.sessionOf(res)
		e.mustDeliver(e.paidEvent(cs))
		got := e.reload(b.ID)
		if got.Status != "held" || got.PaidAmount != stale {
			t.Fatalf("below the threshold: %s paid %d", got.Status, got.PaidAmount)
		}
		e.f.Clock.Advance(16 * time.Minute)
		if n, err := e.bookings.ExpireHolds(e.ctx); err != nil || n != 1 {
			t.Fatalf("expire holds: %d %v", n, err)
		}
		if e.count("payment_intents", "kind = 'refund' AND booking_id = ? AND status = 'created'", b.ID) != 1 ||
			e.count("outbox_events", "type = ?", gateway.EventRefundScheduled) != 1 {
			t.Fatal("refund of the expired booking not scheduled in the expiry transaction")
		}
		// Twice is harmless (idempotent per payment).
		err = store.RunInTx(e.ctx, db, func(ctx context.Context, tx bun.Tx) error {
			return e.gw.RefundExpiredBooking(ctx, tx, e.es, b.ID)
		})
		if err != nil || e.count("payment_intents", "kind = 'refund' AND booking_id = ?", b.ID) != 1 {
			t.Fatalf("second schedule: %v", err)
		}
		e.f.Clock.Advance(2 * time.Minute)
		if _, err := e.gw.Reconcile(e.ctx); err != nil {
			t.Fatal(err)
		}
		if refunds := e.srv.RefundsOf(e.srv.Session(cs).PaymentIntent); len(refunds) != 1 || refunds[0].Amount != int64(stale) {
			t.Fatalf("provider refunds: %+v", refunds)
		}
		if got := e.reload(b.ID); got.Status != "expired" || got.PaidAmount != 0 || e.ledgerOf(b.ID) != 0 {
			t.Fatalf("money left on the expired booking: %+v", got.PaidAmount)
		}
	})
}

// TestConcurrency_WebhookVsHoldExpiry (INV-05, INV-20, INV-51): 100
// bookings receive their payment webhook while their hold elapses and the
// sweeper expires them, in parallel (the clock jumps past the holds in the
// middle of the deliveries). Each ends either confirmed with its
// allocations and the money in the ledger, or expired with no allocation
// and the money refunded; never confirmed without resources, never
// expired with money on it.
func TestConcurrency_WebhookVsHoldExpiry(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		const n = 100
		type item struct {
			b  *store.Booking
			cs string
		}
		items := make([]item, 0, n)
		for i := 0; i < n; i++ {
			day := fmt.Sprintf("2027-06-%02d", 1+i%28)
			b := e.held(i/28, day)
			res := e.start(b)
			cs := e.sessionOf(res)
			e.srv.Pay(cs)
			items = append(items, item{b: b, cs: cs})
		}
		// One minute before the holds elapse; the jump in the middle of the
		// race stays within the webhook signature tolerance (5 minutes).
		e.f.Clock.Advance(14 * time.Minute)
		payloads := make([][]byte, len(items))
		for i, it := range items {
			payloads[i] = e.srv.SessionEvent(e.evt(), "checkout.session.completed", it.cs, e.f.Clock.Now())
		}
		var wg sync.WaitGroup
		var failures atomic.Int32
		var delivered atomic.Int32
		for i := range items {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.deliver(payloads[i]); err != nil {
					failures.Add(1)
					t.Logf("webhook: %v", err)
				}
				delivered.Add(1)
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Let some deliveries win, then the holds elapse and the
			// sweeper races the rest.
			for delivered.Load() < n/4 {
				time.Sleep(time.Millisecond)
			}
			e.f.Clock.Advance(2 * time.Minute)
			for range 4 {
				if _, err := e.bookings.ExpireHolds(e.ctx); err != nil {
					t.Error(err)
				}
			}
		}()
		wg.Wait()
		if failures.Load() != 0 {
			t.Fatalf("%d webhook deliveries failed", failures.Load())
		}
		var confirmed, expired int
		for _, it := range items {
			got := e.reload(it.b.ID)
			kinds := e.allocKinds(it.b.ID)
			switch got.Status {
			case "confirmed":
				confirmed++
				if kinds["booking"] == 0 || got.PaidAmount != it.b.TotalAmount || e.ledgerOf(it.b.ID) != it.b.TotalAmount {
					t.Fatalf("confirmed without resources or money: %v paid %d", kinds, got.PaidAmount)
				}
			case "expired":
				expired++
				if len(kinds) != 0 || got.PaidAmount != 0 || e.ledgerOf(it.b.ID) != 0 {
					t.Fatalf("expired with resources or money: %v paid %d ledger %d", kinds, got.PaidAmount, e.ledgerOf(it.b.ID))
				}
				if refunds := e.srv.RefundsOf(e.srv.Session(it.cs).PaymentIntent); len(refunds) != 1 {
					t.Fatalf("expired booking refunds at the provider: %d", len(refunds))
				}
			default:
				t.Fatalf("booking %s in %s", it.b.Code, got.Status)
			}
		}
		if n, err := e.bookings.ExpireHolds(e.ctx); err != nil || n != 0 {
			t.Fatalf("sweeper after the race: %d %v", n, err)
		}
		if confirmed == 0 || expired == 0 {
			t.Fatalf("race not exercised: confirmed %d expired %d", confirmed, expired)
		}
		t.Logf("confirmed %d expired %d", confirmed, expired)
	})
}
