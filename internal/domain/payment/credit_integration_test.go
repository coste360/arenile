//go:build integration

package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// creditEnv is env with the M5B credit ledger as the Credits port (balance
// table, conditional update, expiry) instead of the store double.
func creditEnv(t testing.TB, db *store.DB) (*env, *credit.Service) {
	t.Helper()
	e := newEnv(t, db)
	aw, ob := audit.NewWriter(e.clock), event.NewOutbox(e.clock)
	cs := credit.NewService(credit.Deps{DB: db, Clock: e.clock, Audit: aw, Outbox: ob})
	e.svc = payment.NewService(payment.Deps{
		DB: db, Clock: e.clock, Audit: aw, Outbox: ob, Bookings: ledger{}, Cash: e.cash, Fiscal: e.fiscal,
		Credits: cs, Providers: registry{e.clock}, Confirmer: e.conf,
	})
	cs.SetPayments(e.svc)
	return e, cs
}

func grant(t testing.TB, e *env, cs *credit.Service, amount money.Cents) {
	t.Helper()
	if err := store.RunInTx(e.ctx, e.db, func(ctx context.Context, tx bun.Tx) error {
		return cs.GrantCredit(ctx, tx, e.esA, &store.CustomerCredit{CustomerID: e.custA.ID, Amount: amount, Source: store.CreditSourceGoodwill})
	}); err != nil {
		t.Fatal(err)
	}
}

func balanceOf(t testing.TB, e *env) money.Cents {
	t.Helper()
	bal, err := store.GetCustomerCreditBalance(e.ctx, e.db, e.esA, e.custA.ID)
	if err != nil {
		t.Fatal(err)
	}
	return bal
}

func TestPayWithCredit_ManageLinkPartial(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e, cs := creditEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)
		grant(t, e, cs, 3000)
		pm, err := cs.ApplyFromLink(e.ctx, e.estA.ID, b.ID, 0, "k-1")
		if err != nil {
			t.Fatal(err)
		}
		if pm.Amount != 3000 || pm.Method != store.PaymentMethodCredit || pm.CashSessionID != nil {
			t.Fatalf("payment %+v", pm)
		}
		if got := e.assertLedger(t, b.ID); got != 3000 {
			t.Fatalf("paid %d", got)
		}
		if bal := balanceOf(t, e); bal != 0 {
			t.Fatalf("balance %d", bal)
		}
		// Replay with the same key returns the same payment.
		again, err := cs.ApplyFromLink(e.ctx, e.estA.ID, b.ID, 0, "k-1")
		if err != nil || again.ID != pm.ID {
			t.Fatalf("replay %v %v", again, err)
		}
		// Nothing left to spend.
		if _, err := cs.ApplyFromLink(e.ctx, e.estA.ID, b.ID, 0, ""); !errors.Is(err, payment.ErrNoCreditAvailable) {
			t.Fatalf("no credit: %v", err)
		}
		// A held booking is not payable from the manage link.
		h := e.booking(t, "held", 5000, 0, snapshot22and10)
		grant(t, e, cs, 1000)
		if _, err := cs.ApplyFromLink(e.ctx, e.estA.ID, h.ID, 0, ""); !errors.Is(err, payment.ErrBookingNotPayable) {
			t.Fatalf("held: %v", err)
		}
	})
}

func TestApplyCreditToBooking_HeldConfirmsAtDeposit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e, cs := creditEnv(t, db)
		b := e.booking(t, "held", 10000, 3000, snapshot22and10)
		grant(t, e, cs, 5000)
		applied, err := cs.ApplyCreditToBooking(e.ctx, e.esA, b.ID, 4000, "")
		if err != nil || applied != 4000 {
			t.Fatalf("applied %d %v", applied, err)
		}
		if got := e.reload(t, b.ID); got.Status != "confirmed" || got.PaidAmount != 4000 {
			t.Fatalf("booking %s paid %d", got.Status, got.PaidAmount)
		}
		if bal := balanceOf(t, e); bal != 1000 {
			t.Fatalf("balance %d", bal)
		}
		// No credit for another customer: zero applied, no error.
		other := e.customer(t, e.estA.ID)
		b2 := e.booking(t, "held", 1000, 0, snapshot22and10)
		if _, err := db.NewUpdate().TableExpr("bookings").Set("customer_id = ?", other.ID).Where("id = ?", b2.ID).Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		if applied, err := cs.ApplyCreditToBooking(e.ctx, e.esA, b2.ID, 0, ""); err != nil || applied != 0 {
			t.Fatalf("no credit: %d %v", applied, err)
		}
	})
}

// TestRefund_CreditPaidPartGoesBackAsCredit: a money refund of a booking
// paid partly with customer credit returns that part as credit (spec 13.4).
func TestRefund_CreditPaidPartGoesBackAsCredit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e, cs := creditEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)
		grant(t, e, cs, 6000)
		if _, err := cs.ApplyFromLink(e.ctx, e.estA.ID, b.ID, 6000, ""); err != nil {
			t.Fatal(err)
		}
		e.openSession(t, 0)
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 4000, Method: store.PaymentMethodCash}); err != nil {
			t.Fatal(err)
		}
		// Refund 7000 as money: 4000 cash max, the other 3000 become credit.
		res, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{
			BookingID: b.ID, Amount: 7000, As: payment.RefundAsRefund, Method: store.PaymentMethodCash, Reason: "rimborso",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.Payment.Method != store.PaymentMethodCash || res.Payment.Amount != 4000 {
			t.Fatalf("money part %+v", res.Payment)
		}
		if got := e.assertLedger(t, b.ID); got != 3000 {
			t.Fatalf("paid after refund %d", got)
		}
		if bal := balanceOf(t, e); bal != 3000 {
			t.Fatalf("credit back %d", bal)
		}
		if n := e.count(t, "payments", "booking_id = ? AND direction = ? AND method = ?", b.ID, store.PaymentDirectionRefund, store.PaymentMethodCredit); n != 1 {
			t.Fatalf("credit refunds %d", n)
		}
		// A refund aimed at the credit payment itself comes back as credit
		// of source cancellation (policy expiry), never as "manual".
		var orig store.Payment
		if err := db.NewSelect().Model(&orig).Where("pm.booking_id = ?", b.ID).Where("pm.direction = ?", store.PaymentDirectionIn).
			Where("pm.method = ?", store.PaymentMethodCredit).Scan(e.ctx); err != nil {
			t.Fatal(err)
		}
		res, err = e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{
			BookingID: b.ID, Amount: 1000, As: payment.RefundAsRefund, Method: store.PaymentMethodCash, PaymentID: orig.ID, Reason: "storno",
		})
		if err != nil || res.Payment.Method != store.PaymentMethodCredit || res.Payment.Amount != 1000 {
			t.Fatalf("refund of the credit payment: %v %+v", err, res)
		}
		if n := e.count(t, "customer_credits", "customer_id = ? AND source = ? AND amount = 1000", e.custA.ID, store.CreditSourceCancellation); n != 1 {
			t.Fatalf("cancellation credit of the payment refund %d", n)
		}
		if bal := balanceOf(t, e); bal != 4000 {
			t.Fatalf("credit after the payment refund %d", bal)
		}
	})
}
