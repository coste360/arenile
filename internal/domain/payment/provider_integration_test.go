//go:build integration

package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func (e *env) inTx(t *testing.T, fn func(ctx context.Context, tx bun.Tx) error) error {
	t.Helper()
	return store.RunInTx(e.ctx, e.db, fn)
}

// TestProviderPayment_ConfirmsAndReplays: a provider payment reaching the
// deposit confirms through the BookingConfirmer, writes the ledger row
// without a cash session, audit (actor system) and payment.succeeded; the
// same provider reference replays without writing.
func TestProviderPayment_ConfirmsAndReplays(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.openSession(t, 1000) // an open session must not receive online payments
		b := e.booking(t, "pending_payment", 10000, 3000, snapshot22and10)
		in := payment.ProviderPayment{BookingID: b.ID, Provider: "stripe", ProviderRef: "cs_test_1", Amount: 3000, IntentID: store.NewID(), EventID: "evt_1"}
		var res *payment.Result
		if err := e.inTx(t, func(ctx context.Context, tx bun.Tx) error {
			var err error
			res, err = e.svc.RecordProviderPayment(ctx, tx, e.esA, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !res.Confirmed || res.Replayed || res.Booking.Status != "confirmed" || len(e.conf.calls) != 1 {
			t.Fatalf("result: %+v", res)
		}
		pm := res.Payment
		if pm.Provider != "stripe" || pm.Method != store.PaymentMethodCard || pm.CashSessionID != nil || pm.Status != store.PaymentSucceeded || pm.ReceivedByUserID != nil {
			t.Fatalf("ledger row: %+v", pm)
		}
		if e.count(t, "audit_log", "entity_id = ? AND actor_type = ?", pm.ID, "system") != 1 || e.count(t, "outbox_events", "type = ?", "payment.succeeded") != 1 {
			t.Fatal("audit/outbox")
		}
		if e.count(t, "fiscal_documents", "payment_id = ?", pm.ID) != 1 {
			t.Fatal("fiscal document")
		}
		var again *payment.Result
		if err := e.inTx(t, func(ctx context.Context, tx bun.Tx) error {
			var err error
			again, err = e.svc.RecordProviderPayment(ctx, tx, e.esA, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !again.Replayed || again.Payment.ID != pm.ID || len(e.conf.calls) != 1 {
			t.Fatalf("replay: %+v", again)
		}
		if paid := e.assertLedger(t, b.ID); paid != 3000 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestProviderPayment_LateNoConfirm: NoConfirm records the money on an
// expired booking without confirming it.
func TestProviderPayment_LateNoConfirm(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "expired", 5000, 0, snapshot22and10)
		var res *payment.Result
		if err := e.inTx(t, func(ctx context.Context, tx bun.Tx) error {
			var err error
			res, err = e.svc.RecordProviderPayment(ctx, tx, e.esA, payment.ProviderPayment{BookingID: b.ID, Provider: "stripe", ProviderRef: "cs_late", Amount: 5000, NoConfirm: true})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if res.Confirmed || len(e.conf.calls) != 0 || e.reload(t, b.ID).Status != "expired" {
			t.Fatalf("late: %+v", res)
		}
		if paid := e.assertLedger(t, b.ID); paid != 5000 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestProviderRefund_CapsAndReplays (INV-24): provider refunds never exceed
// the refunded payment nor the booking net; the same reference replays.
func TestProviderRefund_CapsAndReplays(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "pending_payment", 10000, 0, snapshot22and10)
		var pmID string
		if err := e.inTx(t, func(ctx context.Context, tx bun.Tx) error {
			r, err := e.svc.RecordProviderPayment(ctx, tx, e.esA, payment.ProviderPayment{BookingID: b.ID, Provider: "stripe", ProviderRef: "cs_2", Amount: 10000})
			if r != nil {
				pmID = r.Payment.ID
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		refund := func(ref string, amount int64, paymentID string) (*store.Payment, error) {
			var out *store.Payment
			err := e.inTx(t, func(ctx context.Context, tx bun.Tx) error {
				var err error
				out, err = e.svc.RecordProviderRefund(ctx, tx, e.esA, payment.ProviderRefund{
					BookingID: b.ID, PaymentID: paymentID, Provider: "stripe", ProviderRef: ref, Amount: money.Cents(amount), Reason: "Pioggia",
				})
				return err
			})
			return out, err
		}
		r1, err := refund("re_1", 4000, pmID)
		if err != nil || r1.RefundOfPaymentID == nil || *r1.RefundOfPaymentID != pmID || r1.Direction != store.PaymentDirectionRefund {
			t.Fatalf("refund: %v %+v", err, r1)
		}
		if r, err := refund("re_1", 4000, pmID); err != nil || r.ID != r1.ID {
			t.Fatalf("replay: %v", err)
		}
		if _, err := refund("re_2", 6001, pmID); !errors.Is(err, payment.ErrRefundExceedsPaid) {
			t.Fatalf("above the payment: %v", err)
		}
		if _, err := refund("re_3", 6000, pmID); err != nil {
			t.Fatal(err)
		}
		if paid := e.assertLedger(t, b.ID); paid != 0 {
			t.Fatalf("paid %d", paid)
		}
		if _, err := refund("re_4", 1, store.NewID()); !errors.Is(err, payment.ErrPaymentNotFound) {
			t.Fatalf("unknown payment: %v", err)
		}
		if e.count(t, "outbox_events", "type = ?", "payment.refunded") != 2 {
			t.Fatal("refund events")
		}
	})
}
