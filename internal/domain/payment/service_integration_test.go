//go:build integration

package payment_test

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestPaymentService_DeskFlow: cash needs an open session, the balance is
// never exceeded, the VAT split follows the price snapshot, paid_amount
// follows the ledger (INV-22), every write has audit + outbox + fiscal
// document with a daily sequence number.
func TestPaymentService_DeskFlow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)

		// Cash without an open session: 409 cash_session_closed.
		_, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 4000, Method: store.PaymentMethodCash})
		if !errors.Is(err, cash.ErrSessionClosed) {
			t.Fatalf("cash without session: %v", err)
		}
		// Card works without a session.
		card, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 3000, Method: store.PaymentMethodCard, Note: "POS 1"})
		if err != nil {
			t.Fatal(err)
		}
		if card.Payment.CashSessionID != nil || card.Payment.Provider != payment.ProviderManual || card.Payment.ProviderRef == nil {
			t.Fatalf("card payment: %+v", card.Payment)
		}
		if card.Payment.BusinessDate != day0 || card.Booking.PaidAmount != 3000 {
			t.Fatalf("card: date %s paid %d", card.Payment.BusinessDate, card.Booking.PaidAmount)
		}
		// VAT split proportional to the snapshot (61% at 22, 39% at 10).
		vb := card.Payment.VATBreakdown
		if len(vb) != 2 || vb[0].RateBP != 1000 || vb[0].Gross != 1170 || vb[1].RateBP != 2200 || vb[1].Gross != 1830 {
			t.Fatalf("vat split: %+v", vb)
		}
		for _, sh := range vb {
			if sh.Taxable+sh.Tax != sh.Gross {
				t.Fatalf("INV-16: %+v", sh)
			}
		}

		cs := e.openSession(t, 5000)
		pay, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 4000, Method: store.PaymentMethodCash})
		if err != nil {
			t.Fatal(err)
		}
		if pay.Payment.CashSessionID == nil || *pay.Payment.CashSessionID != cs.ID {
			t.Fatalf("cash payment not attached to the session: %+v", pay.Payment)
		}
		// Over the balance (3000 left): validation, nothing written.
		_, err = e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 3001, Method: store.PaymentMethodCash})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "amount" {
			t.Fatalf("overpayment: %v", err)
		}
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 3000, Method: store.PaymentMethodBankTransfer}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 1, Method: store.PaymentMethodCash}); !errors.As(err, &ve) {
			t.Fatalf("paid booking: %v", err)
		}
		if paid := e.assertLedger(t, b.ID); paid != 10000 {
			t.Fatalf("paid = %d", paid)
		}

		// Refunds: cashier lacks refunds.issue; above paid is refused (INV-24).
		if _, err := e.svc.RefundBooking(e.ctx, e.cashier, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 100, Method: store.PaymentMethodCash}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier refund: %v", err)
		}
		if _, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 10001, Method: store.PaymentMethodCash}); !errors.As(err, &ve) {
			t.Fatalf("refund above paid: %v", err)
		}
		// Refund of a specific payment is bounded by that payment.
		if _, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 3001, Method: store.PaymentMethodCard, PaymentID: card.Payment.ID}); !errors.As(err, &ve) {
			t.Fatalf("refund above the payment: %v", err)
		}
		rf, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 1000, Method: store.PaymentMethodCard, PaymentID: card.Payment.ID, Reason: "maltempo"})
		if err != nil {
			t.Fatal(err)
		}
		if rf.Payment.Direction != store.PaymentDirectionRefund || rf.Booking.PaidAmount != 9000 || *rf.Payment.RefundOfPaymentID != card.Payment.ID {
			t.Fatalf("refund: %+v paid %d", rf.Payment, rf.Booking.PaidAmount)
		}
		// Refund as credit: method credit, customer credit granted.
		if _, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 500, As: payment.RefundAsCredit}); err != nil {
			t.Fatal(err)
		}
		if bal, _ := (credits{}).CreditBalance(e.ctx, e.db, e.esA, e.custA.ID, day0); bal != 500 {
			t.Fatalf("credit balance %d", bal)
		}
		if paid := e.assertLedger(t, b.ID); paid != 8500 {
			t.Fatalf("paid after refunds = %d", paid)
		}

		// 3 payments + 2 refunds: audit, outbox and fiscal documents.
		if n := e.count(t, "audit_log", "entity_type = 'payment' AND establishment_id = ?", e.estA.ID); n != 5 {
			t.Fatalf("audit rows %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.PaymentSucceeded); n != 3 {
			t.Fatalf("payment.succeeded events %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.PaymentRefunded); n != 2 {
			t.Fatalf("payment.refunded events %d", n)
		}
		docs, err := e.fiscal.ListDocuments(e.ctx, e.owner, e.estA.ID, store.FiscalDocumentFilter{}, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(docs.Items) != 5 || *docs.Items[0].DocumentNumber != "20270715-0005" || *docs.Items[4].DocumentNumber != "20270715-0001" {
			t.Fatalf("fiscal documents: %d %+v", len(docs.Items), docs.Items)
		}
		for _, d := range docs.Items {
			if d.Status != store.FiscalIssued || d.Driver != "none" {
				t.Fatalf("document %+v", d)
			}
		}

		// Corrispettivi reconcile with the ledger.
		rep, err := e.fiscal.Corrispettivi(e.ctx, e.owner, e.estA.ID, fiscal.CorrispettiviQuery{From: day0, To: day0})
		if err != nil {
			t.Fatal(err)
		}
		if rep.GrandTotal != 8500 {
			t.Fatalf("corrispettivi grand total %d", rep.GrandTotal)
		}

		// Reads: viewer may list, not record; another tenant sees nothing.
		list, err := e.svc.BookingPayments(e.ctx, e.viewer, e.estA.ID, b.ID)
		if err != nil || len(list) != 5 {
			t.Fatalf("booking payments: %d %v", len(list), err)
		}
		page, err := e.svc.List(e.ctx, e.viewer, e.estA.ID, store.PaymentFilter{Direction: store.PaymentDirectionRefund}, "", 50)
		if err != nil || len(page.Items) != 2 || page.Items[0].BookingCode == nil || *page.Items[0].BookingCode != b.Code {
			t.Fatalf("list refunds: %+v %v", page.Items, err)
		}
		if _, err := e.svc.RecordPayment(e.ctx, e.viewer, "", e.estA.ID, b.ID, payment.Input{Amount: 1, Method: store.PaymentMethodCard}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer payment: %v", err)
		}
		if _, err := e.svc.BookingPayments(e.ctx, e.ownerB, e.estA.ID, b.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		if _, err := e.svc.BookingPayments(e.ctx, e.owner, e.estA.ID, store.NewID()); !errors.Is(err, payment.ErrBookingNotFound) {
			t.Fatalf("unknown booking: %v", err)
		}
	})
}

// TestPaymentService_ConfirmsOnDeposit: the manual driver confirms a held
// booking once the deposit of its policy is paid (INV-19), never before.
func TestPaymentService_ConfirmsOnDeposit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "held", 10000, 3000, snapshot22and10)
		r, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 2000, Method: store.PaymentMethodCard})
		if err != nil || r.Confirmed || len(e.conf.calls) != 0 {
			t.Fatalf("below deposit: %v %+v", err, r)
		}
		r, err = e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 1000, Method: store.PaymentMethodCard})
		if err != nil || !r.Confirmed || len(e.conf.calls) != 1 || e.conf.calls[0] != b.ID {
			t.Fatalf("deposit reached: %v %+v %v", err, r, e.conf.calls)
		}
		if got := e.reload(t, b.ID).Status; got != "confirmed" {
			t.Fatalf("status %s", got)
		}
		// Confirmed bookings are not confirmed again.
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 7000, Method: store.PaymentMethodCard}); err != nil || len(e.conf.calls) != 1 {
			t.Fatalf("balance: %v %v", err, e.conf.calls)
		}

		// Without deposit, the total is the threshold.
		b2 := e.booking(t, "pending_payment", 5000, 0, snapshot22and10)
		r, err = e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b2.ID, payment.Input{Amount: 5000, Method: store.PaymentMethodCard})
		if err != nil || !r.Confirmed {
			t.Fatalf("full payment: %v %+v", err, r)
		}
		// Cancelled bookings take no payments.
		b3 := e.booking(t, "cancelled", 5000, 0, snapshot22and10)
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b3.ID, payment.Input{Amount: 100, Method: store.PaymentMethodCard}); !errors.Is(err, payment.ErrBookingNotPayable) {
			t.Fatalf("cancelled: %v", err)
		}
	})
}

// TestPaymentService_CreditsAndCounterSales: overpayment becomes customer
// credit only when asked; paying with credit needs enough balance; counter
// sales use the given (or default) VAT rate.
func TestPaymentService_CreditsAndCounterSales(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.openSession(t, 0)
		b := e.booking(t, "confirmed", 6000, 0, "{}")
		r, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 8000, Method: store.PaymentMethodCash, OverpaymentAsCredit: true})
		if err != nil {
			t.Fatal(err)
		}
		if r.Excess == nil || r.Excess.Amount != 2000 || r.Excess.Method != store.PaymentMethodCredit || r.Booking.PaidAmount != 6000 {
			t.Fatalf("overpayment: %+v %+v", r.Excess, r.Booking)
		}
		// No snapshot: everything at the default 22%.
		if vb := r.Payment.VATBreakdown; len(vb) != 1 || vb[0].RateBP != 2200 || vb[0].Taxable != 6557 || vb[0].Tax != 1443 {
			t.Fatalf("default vat: %+v", vb)
		}
		e.assertLedger(t, b.ID)

		b2 := e.booking(t, "confirmed", 5000, 0, "{}")
		var ve *configerr.ValidationError
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b2.ID, payment.Input{Amount: 2001, Method: store.PaymentMethodCredit}); !errors.As(err, &ve) {
			t.Fatalf("insufficient credit: %v", err)
		}
		cr, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b2.ID, payment.Input{Amount: 2000, Method: store.PaymentMethodCredit})
		if err != nil || cr.Payment.CashSessionID != nil {
			t.Fatalf("credit payment: %v %+v", err, cr)
		}
		if bal, _ := (credits{}).CreditBalance(e.ctx, e.db, e.esA, e.custA.ID, day0); bal != 0 {
			t.Fatalf("credit left %d", bal)
		}

		rate := int64(1000)
		cs, err := e.svc.RecordCounterSale(e.ctx, e.cashier, "", e.estA.ID, payment.CounterSaleInput{Amount: 1100, Method: store.PaymentMethodCash, Description: "Lettino extra", VATRateBP: &rate})
		if err != nil {
			t.Fatal(err)
		}
		if cs.Payment.BookingID != nil || cs.Payment.CashSessionID == nil || cs.Payment.VATBreakdown[0].Tax != 100 {
			t.Fatalf("counter sale: %+v", cs.Payment)
		}
		if _, err := e.svc.RecordCounterSale(e.ctx, e.cashier, "", e.estA.ID, payment.CounterSaleInput{Amount: 100, Method: store.PaymentMethodCard}); !errors.As(err, &ve) {
			t.Fatalf("counter sale without description: %v", err)
		}
		if _, err := e.svc.RecordCounterSale(e.ctx, e.cashier, "", e.estA.ID, payment.CounterSaleInput{Amount: 100, Method: store.PaymentMethodCard, Description: "x", CustomerID: store.NewID()}); !errors.Is(err, payment.ErrCustomerNotFound) {
			t.Fatalf("unknown customer: %v", err)
		}
	})
}

// TestPaymentService_Idempotency: the same key replays the payment, a key
// reused for another payment is a conflict.
func TestPaymentService_Idempotency(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)
		in := payment.Input{Amount: 1500, Method: store.PaymentMethodCard, IdempotencyKey: "offline-0001"}
		first, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, in)
		if err != nil || first.Replayed {
			t.Fatalf("first: %v %+v", err, first)
		}
		again, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, in)
		if err != nil || !again.Replayed || again.Payment.ID != first.Payment.ID {
			t.Fatalf("replay: %v %+v", err, again)
		}
		in.Amount = 1600
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, in); !errors.Is(err, payment.ErrIdempotencyMismatch) {
			t.Fatalf("mismatch: %v", err)
		}
		if paid := e.assertLedger(t, b.ID); paid != 1500 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestConcurrency_PaymentIdempotency: 100 parallel submissions of the same
// offline payment (same Idempotency-Key) record it exactly once.
func TestConcurrency_PaymentIdempotency(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)
		const workers = 100
		var (
			wg       sync.WaitGroup
			fresh    atomic.Int32
			replayed atomic.Int32
			ids      sync.Map
		)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID,
					payment.Input{Amount: 2500, Method: store.PaymentMethodCard, IdempotencyKey: "queue-42"})
				if err != nil {
					t.Error(err)
					return
				}
				ids.Store(r.Payment.ID, true)
				if r.Replayed {
					replayed.Add(1)
				} else {
					fresh.Add(1)
				}
			}()
		}
		wg.Wait()
		n := 0
		ids.Range(func(any, any) bool { n++; return true })
		if fresh.Load() != 1 || replayed.Load() != workers-1 || n != 1 {
			t.Fatalf("fresh %d replayed %d distinct ids %d", fresh.Load(), replayed.Load(), n)
		}
		if paid := e.assertLedger(t, b.ID); paid != 2500 {
			t.Fatalf("paid %d", paid)
		}
		if c := e.count(t, "payments", "booking_id = ?", b.ID); c != 1 {
			t.Fatalf("payments rows %d", c)
		}
	})
}

// TestConcurrency_PaymentsOnSameBooking: 100 parallel payments of 1.00 on a
// booking with 50.00 due: exactly 50 succeed, the balance is never exceeded
// and paid_amount matches the ledger.
func TestConcurrency_PaymentsOnSameBooking(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 5000, 0, snapshot22and10)
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 100, Method: store.PaymentMethodCard})
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
		if paid := e.assertLedger(t, b.ID); paid != 5000 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestPayments_PaidAmountMatchesLedger (INV-22, INV-24) and the
// reconciliation property of the corrispettivi export: after any sequence
// of payments and refunds, paid_amount equals the ledger, refunds never
// exceed payments, and the export grand total (and the sum of its rows and
// of its per-rate totals) equals payments - refunds exactly.
func TestPayments_PaidAmountMatchesLedger(t *testing.T) {
	db := storetest.SQLite(t)
	e := newEnv(t, db)
	e.openSession(t, 0)
	methods := []string{store.PaymentMethodCash, store.PaymentMethodCard, store.PaymentMethodBankTransfer, store.PaymentMethodOther}
	rapid.Check(t, func(rt *rapid.T) {
		total := money.Cents(rapid.Int64Range(1, 500_000).Draw(rt, "total"))
		b := e.booking(t, "confirmed", total, 0, snapshot22and10)
		ops := rapid.IntRange(1, 12).Draw(rt, "ops")
		var expectNet money.Cents
		for i := 0; i < ops; i++ {
			method := rapid.SampledFrom(methods).Draw(rt, "method")
			amount := money.Cents(rapid.Int64Range(1, 300_000).Draw(rt, "amount"))
			if rapid.Bool().Draw(rt, "refund") {
				_, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: amount, Method: method})
				switch {
				case err == nil:
					expectNet -= amount
				case errors.Is(err, configerr.ErrValidation):
					if amount <= expectNet {
						rt.Fatalf("refund %d refused with %d paid: %v", amount, expectNet, err)
					}
				default:
					rt.Fatal(err)
				}
				continue
			}
			_, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: amount, Method: method})
			switch {
			case err == nil:
				expectNet += amount
			case errors.Is(err, configerr.ErrValidation):
				if amount <= total-expectNet {
					rt.Fatalf("payment %d refused with %d due: %v", amount, total-expectNet, err)
				}
			default:
				rt.Fatal(err)
			}
		}
		if got := e.assertLedger(t, b.ID); got != expectNet {
			rt.Fatalf("paid %d, want %d", got, expectNet)
		}
		if expectNet > total {
			rt.Fatalf("paid %d above total %d", expectNet, total)
		}
	})
	// Reconciliation over everything recorded by the property run.
	all, err := store.ListSucceededPayments(e.ctx, db, e.esA, day0, day0)
	if err != nil {
		t.Fatal(err)
	}
	var ledger money.Cents
	for i := range all {
		ledger += all[i].Signed()
	}
	for _, g := range []fiscal.GroupBy{fiscal.GroupByDay, fiscal.GroupByMonth} {
		rep, err := e.fiscal.Corrispettivi(e.ctx, e.owner, e.estA.ID, fiscal.CorrispettiviQuery{From: day0, To: day0, GroupBy: g})
		if err != nil {
			t.Fatal(err)
		}
		var rows, rates money.Cents
		for _, r := range rep.Rows {
			rows += r.Gross
			if r.Taxable+r.Tax != r.Gross {
				t.Fatalf("INV-16 row %+v", r)
			}
		}
		for _, r := range rep.Totals {
			rates += r.Gross
		}
		if rep.GrandTotal != ledger || rows != ledger || rates != ledger {
			t.Fatalf("%s: grand %d rows %d rates %d ledger %d", g, rep.GrandTotal, rows, rates, ledger)
		}
	}
}

// TestRefund_NeverExceedsPaid (INV-24): parallel refunds of a paid booking
// never refund more than was paid.
func TestRefund_NeverExceedsPaid(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 3000, 0, snapshot22and10)
		if _, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 3000, Method: store.PaymentMethodCard}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var ok atomic.Int32
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.svc.RefundBooking(e.ctx, e.owner, "", e.estA.ID, payment.RefundRequest{BookingID: b.ID, Amount: 100, Method: store.PaymentMethodCard})
				if err == nil {
					ok.Add(1)
				} else if !errors.Is(err, configerr.ErrValidation) {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 30 {
			t.Fatalf("refunds accepted %d, want 30", ok.Load())
		}
		if paid := e.assertLedger(t, b.ID); paid != 0 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestPaymentService_AtomicRollback: when a later step fails (here the
// fiscal driver is misconfigured), nothing of the payment remains.
func TestPaymentService_AtomicRollback(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		b := e.booking(t, "confirmed", 10000, 0, snapshot22and10)
		e.estA.Settings = store.JSON(`{"fiscal":{"driver":"rt-missing"}}`)
		if _, err := db.NewUpdate().Model(e.estA).Column("settings").WherePK().Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.RecordPayment(e.ctx, e.cashier, "", e.estA.ID, b.ID, payment.Input{Amount: 1000, Method: store.PaymentMethodCard})
		if !errors.Is(err, fiscal.ErrUnknownDriver) {
			t.Fatalf("err %v", err)
		}
		if n := e.count(t, "payments", "booking_id = ?", b.ID); n != 0 {
			t.Fatalf("payments %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.PaymentSucceeded); n != 0 {
			t.Fatalf("events %d", n)
		}
		if paid := e.assertLedger(t, b.ID); paid != 0 {
			t.Fatalf("paid %d", paid)
		}
	})
}

// TestConcurrency_CreditRedemption_NeverNegative (INV-47 / INV-70): 100
// counter sales paid with the same customer credit of 50.00 run in parallel
// on the real credit ledger (credit.Service: balance row lock, conditional
// balance update); exactly one spends it, the balance never goes below zero,
// the stored balance equals the ledger sum and the redemption rows match
// the payments.
func TestConcurrency_CreditRedemption_NeverNegative(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e, cs := creditEnv(t, db)
		grant(t, e, cs, 5000)
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				for attempt := 0; attempt < 50; attempt++ {
					_, err = e.svc.RecordCounterSale(e.ctx, e.cashier, "", e.estA.ID, payment.CounterSaleInput{
						Amount: 5000, Method: store.PaymentMethodCredit, Description: "Lettino", CustomerID: e.custA.ID,
					})
					if err == nil || !retryable(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, configerr.ErrValidation), errors.Is(err, credit.ErrInsufficient):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 1 || refused.Load() != 99 {
			t.Fatalf("ok %d refused %d", ok.Load(), refused.Load())
		}
		assertCreditLedger(t, e, 0)
		if n := e.count(t, "customer_credits", "customer_id = ? AND source = ?", e.custA.ID, store.CreditSourceRedemption); n != 1 {
			t.Fatalf("redemption rows %d", n)
		}
		if n := e.count(t, "payments", "method = ? AND status = ?", store.PaymentMethodCredit, store.PaymentSucceeded); n != 1 {
			t.Fatalf("credit payments %d", n)
		}
	})
}


// TestConcurrency_PayWithCredit_RealLedger (INV-47 / INV-70 / INV-71): 100
// parallel credit payments from the magic link (credit.ApplyFromLink, 10.00
// each on ten bookings) against a balance of 50.00: exactly five succeed,
// the bookings receive exactly 50.00 in total and the stored balance equals
// the ledger sum (0).
func TestConcurrency_PayWithCredit_RealLedger(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e, cs := creditEnv(t, db)
		grant(t, e, cs, 5000)
		var bookings []*store.Booking
		for range 10 {
			bookings = append(bookings, e.booking(t, "confirmed", 1000, 0, snapshot22and10))
		}
		var wg sync.WaitGroup
		var ok, refused atomic.Int32
		for i := range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				for attempt := 0; attempt < 50; attempt++ {
					_, err = cs.ApplyFromLink(e.ctx, e.estA.ID, bookings[i%10].ID, 1000, "")
					if err == nil || !retryable(err) {
						break
					}
					time.Sleep(time.Duration(attempt+1) * time.Millisecond)
				}
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, credit.ErrInsufficient), errors.Is(err, configerr.ErrValidation):
					refused.Add(1)
				default:
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if ok.Load() != 5 || refused.Load() != 95 {
			t.Fatalf("ok %d refused %d", ok.Load(), refused.Load())
		}
		assertCreditLedger(t, e, 0)
		var paid money.Cents
		for _, b := range bookings {
			paid += e.assertLedger(t, b.ID)
		}
		if paid != 5000 {
			t.Fatalf("paid with credit %d", paid)
		}
		if n := e.count(t, "customer_credits", "customer_id = ? AND source = ?", e.custA.ID, store.CreditSourceRedemption); n != 5 {
			t.Fatalf("redemption rows %d", n)
		}
	})
}

// assertCreditLedger checks balance = SUM(ledger) = want for custA.
func assertCreditLedger(t testing.TB, e *env, want money.Cents) {
	t.Helper()
	var sum int64
	if err := e.db.NewSelect().TableExpr("customer_credits").ColumnExpr("COALESCE(SUM(amount), 0)").
		Where("customer_id = ?", e.custA.ID).Scan(e.ctx, &sum); err != nil {
		t.Fatal(err)
	}
	if bal := balanceOf(t, e); int64(bal) != sum || bal != want {
		t.Fatalf("balance %d, ledger %d, want %d", bal, sum, want)
	}
}

// retryable reports a transient lock error of the file-backed SQLite
// database (single writer) under heavy contention.
func retryable(err error) bool {
	for _, s := range []string{"database is locked", "SQLITE_BUSY", "deadlock detected", "could not serialize"} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
