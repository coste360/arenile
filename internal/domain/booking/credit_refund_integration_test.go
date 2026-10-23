//go:build integration

package booking_test

import (
	"context"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// TestBookingCancel_CreditPaidPartReturnsAsCredit (M5B, spec 13.4): a
// booking paid partly with customer credit and cancelled with a money
// refund gives the credit-paid part back as credit (source cancellation)
// and only the rest as money; the credit balance table follows the ledger.
func TestBookingCancel_CreditPaidPartReturnsAsCredit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		c := f.Clock
		aw, ob := audit.NewWriter(c), event.NewOutbox(c)
		reg := app.NewRegistry(c, ext.Extensions{})
		cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		ledger := credit.NewService(credit.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		cust.UseCreditLedger(ledger)
		pays := payment.NewService(payment.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
			Cash:   cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
			Fiscal: fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg}), Credits: cust, Providers: reg,
		})
		bk := booking.NewService(booking.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: f.Inv, Pricer: f.Pricing, Customers: cust, Payments: pays})
		pays.SetConfirmer(bk)

		d, err := bk.Create(f.Ctx, f.Owner, f.Est.ID, f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
		if err != nil {
			t.Fatal(err)
		}
		b := d.Booking
		es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RunInTx(f.Ctx, db, func(ctx context.Context, tx bun.Tx) error {
			return ledger.GrantCredit(ctx, tx, es, &store.CustomerCredit{CustomerID: b.CustomerID, Amount: 1500, Source: store.CreditSourceGoodwill})
		}); err != nil {
			t.Fatal(err)
		}
		for _, p := range []payment.Input{{Amount: 1500, Method: store.PaymentMethodCredit}, {Amount: b.TotalAmount - 1500, Method: store.PaymentMethodCard}} {
			if _, err := pays.RecordPayment(f.Ctx, f.Owner, "", f.Est.ID, b.ID, p); err != nil {
				t.Fatalf("pay %s: %v", p.Method, err)
			}
		}
		full := b.TotalAmount
		res, err := bk.Cancel(f.Ctx, f.Owner, f.Est.ID, b.ID, booking.CancelInput{
			Reason: "test", Settlement: booking.SettleRefund, RefundAmount: &full, RefundMethod: store.PaymentMethodCard,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied || res.Recorded == nil || res.Recorded.Method != store.PaymentMethodCard || res.Recorded.Amount != full-1500 {
			t.Fatalf("money refund %+v", res.Recorded)
		}
		bal, err := store.GetCustomerCreditBalance(f.Ctx, db, es, b.CustomerID)
		if err != nil || bal != 1500 {
			t.Fatalf("credit back %d %v", bal, err)
		}
		var back store.CustomerCredit
		if err := db.NewSelect().Model(&back).Where("cc.customer_id = ?", b.CustomerID).Where("cc.amount > 0").
			Where("cc.source = ?", store.CreditSourceCancellation).Scan(f.Ctx); err != nil {
			t.Fatalf("cancellation credit: %v", err)
		}
		if back.Amount != money.Cents(1500) {
			t.Fatalf("cancellation credit %d", back.Amount)
		}
		totals, err := store.BookingPaymentTotals(f.Ctx, db, es, b.ID)
		if err != nil || totals.Net() != 0 {
			t.Fatalf("booking net %d %v", totals.Net(), err)
		}
	})
}

// TestBooking_ExpiredHeldCreditReturnsCredit (INV-71): credit spent on a
// booking that is still held (a renewal or waitlist acceptance below the
// deposit) goes back as credit in the same transaction that expires the
// hold, so an unanswered or abandoned booking never keeps the customer's
// credit.
func TestBooking_ExpiredHeldCreditReturnsCredit(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx, f := e.Ctx, e.F
		s := e.Create(e.Seat(f.Spots[0], "2027-07-10"))
		e.Details(s, "credito@example.org")
		b := e.Booking(s.BookingID)
		if b.Status != "held" || b.TotalAmount <= 500 {
			t.Fatalf("held booking: %s %d", b.Status, b.TotalAmount)
		}
		if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			return e.Credits.GrantCredit(ctx, tx, e.ES, &store.CustomerCredit{CustomerID: b.CustomerID, Amount: 500, Source: store.CreditSourceGoodwill})
		}); err != nil {
			t.Fatal(err)
		}
		pm, err := e.Payments.PayHeldWithCredit(ctx, e.ES, b.ID, 0, "")
		if err != nil || pm == nil || pm.Amount != 500 {
			t.Fatalf("credit on the hold: %+v %v", pm, err)
		}
		if got := e.Booking(b.ID); got.Status != "held" || got.PaidAmount != 500 {
			t.Fatalf("after credit: %s paid %d", got.Status, got.PaidAmount)
		}
		if bal, err := store.GetCustomerCreditBalance(ctx, db, e.ES, b.CustomerID); err != nil || bal != 0 {
			t.Fatalf("balance after redemption %d %v", bal, err)
		}
		f.Clock.Set(f.Clock.Now().Add(time.Hour))
		if n, err := e.Book.ExpireHolds(ctx); err != nil || n != 1 {
			t.Fatalf("expire holds: %d %v", n, err)
		}
		if got := e.Booking(b.ID); got.Status != "expired" || got.PaidAmount != 0 {
			t.Fatalf("expired booking: %s paid %d", got.Status, got.PaidAmount)
		}
		if bal, err := store.GetCustomerCreditBalance(ctx, db, e.ES, b.CustomerID); err != nil || bal != 500 {
			t.Fatalf("credit back %d %v", bal, err)
		}
		if n := e.Count((*store.CustomerCredit)(nil), "customer_id = ? AND source = ? AND amount = 500", b.CustomerID, store.CreditSourceCancellation); n != 1 {
			t.Fatalf("cancellation credit rows %d", n)
		}
		if n := e.Count((*store.Payment)(nil), "booking_id = ? AND direction = ? AND method = ?", b.ID, store.PaymentDirectionRefund, store.PaymentMethodCredit); n != 1 {
			t.Fatalf("credit refunds %d", n)
		}
		// Idempotent: a second sweep changes nothing.
		if n, err := e.Book.ExpireHolds(ctx); err != nil || n != 0 {
			t.Fatalf("second sweep: %d %v", n, err)
		}
	})
}
