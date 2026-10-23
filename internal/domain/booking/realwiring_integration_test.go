//go:build integration

package booking_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// wired is the booking service on the real inventory, customer, payment,
// cash and fiscal services, wired as internal/app/m3.go does.
type wired struct {
	svc  *booking.Service
	pay  *payment.Service
	cash *cash.Service
}

func newReal(f *bookingtest.Fixture) *wired {
	db, c := f.DB, f.Clock
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	reg := app.NewRegistry(c, ext.Extensions{})
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cs := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg})
	pay := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cs, Fiscal: fs, Credits: cust, Providers: reg,
	})
	svc := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing, Customers: cust, Payments: pay,
		Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: cust}),
	})
	pay.SetConfirmer(svc)
	return &wired{svc: svc, pay: pay, cash: cs}
}

// TestBookingRealServices_DeskDay runs a desk flow on the real services:
// held booking confirmed by the manual payment (ConfirmFromPayment), a
// confirmed booking paid at the till, modified, cancelled with a cash
// refund; paid_amount always equals the ledger (INV-22) and allocations
// follow the status (INV-20).
func TestBookingRealServices_DeskDay(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		ctx, est := f.Ctx, f.Est.ID
		if _, err := r.cash.Open(ctx, f.Cashier, "", est, cash.OpenInput{OpeningFloat: 10000}); err != nil {
			t.Fatal(err)
		}
		ledger := func(id string) money.Cents {
			es, _ := store.SystemEstablishmentScope(ctx, db, est)
			pms, err := r.pay.ListByBooking(ctx, db, es, id)
			if err != nil {
				t.Fatal(err)
			}
			var sum money.Cents
			for i := range pms {
				if pms[i].Status == "succeeded" {
					sum += pms[i].Signed()
				}
			}
			return sum
		}

		// A quote on the real inventory (no booking id yet) prices a free
		// spot without conflicts.
		q, err := r.svc.Quote(ctx, f.Cashier, est, f.Input(f.Spots[0], "2027-07-10", "2027-07-11"))
		if err != nil || len(q.Conflicts) != 0 || q.Result.Breakdown.Total <= 0 {
			t.Fatalf("quote on a free spot: %v %+v", err, q)
		}

		// Held -> paid in full with the manual driver -> confirmed.
		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.Status = booking.StatusHeld
		held, err := r.svc.Create(ctx, f.Cashier, est, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.pay.RecordPayment(ctx, f.Cashier, "", est, held.Booking.ID, payment.Input{Amount: held.Booking.TotalAmount, Method: store.PaymentMethodCard}); err != nil {
			t.Fatal(err)
		}
		got, err := r.svc.Get(ctx, f.Cashier, est, held.Booking.ID)
		if err != nil || got.Booking.Status != string(booking.StatusConfirmed) || got.Booking.HoldExpiresAt != nil || len(got.Payments) != 1 {
			t.Fatalf("confirmed by payment: %v %+v", err, got.Booking)
		}
		for _, a := range got.SpotAllocations {
			if a.Kind != store.AllocationBooking {
				t.Fatalf("allocation %+v", a)
			}
		}

		// The same quote now reports the taken half-days.
		q, err = r.svc.Quote(ctx, f.Cashier, est, f.Input(f.Spots[0], "2027-07-10", "2027-07-11"))
		if err != nil || len(q.Conflicts) != 4 {
			t.Fatalf("quote on a taken spot: %v %+v", err, q)
		}

		// Confirmed at the desk, paid in cash, extended by a day, cancelled.
		det, err := r.svc.Create(ctx, f.Cashier, est, f.Input(f.Spots[1], "2027-07-10", "2027-07-11"))
		if err != nil {
			t.Fatal(err)
		}
		id := det.Booking.ID
		if _, err := r.pay.RecordPayment(ctx, f.Cashier, "", est, id, payment.Input{Amount: 4000, Method: store.PaymentMethodCash}); err != nil {
			t.Fatal(err)
		}
		ends := d("2027-07-12")
		cur, _ := r.svc.Get(ctx, f.Cashier, est, id)
		res, err := r.svc.Update(ctx, f.Cashier, est, id, &cur.Booking.Version, booking.PatchInput{EndsOn: &ends})
		if err != nil || res.NewTotal != 6000 || res.Detail.Booking.BalanceDue() != 2000 || len(res.Detail.SpotAllocations) != 6 {
			t.Fatalf("modify: %v %+v", err, res)
		}
		cr, err := r.svc.Cancel(ctx, f.Manager, est, id, booking.CancelInput{RefundMethod: store.PaymentMethodCash, Reason: "rinuncia"})
		if err != nil || cr.Recorded == nil || cr.Refund.Amount != 4000 {
			t.Fatalf("cancel: %v %+v", err, cr)
		}
		got, _ = r.svc.Get(ctx, f.Manager, est, id)
		if got.Booking.PaidAmount != 0 || ledger(id) != got.Booking.PaidAmount || len(got.SpotAllocations) != 0 {
			t.Fatalf("after cancel: paid %d ledger %d allocations %d", got.Booking.PaidAmount, ledger(id), len(got.SpotAllocations))
		}
		if b := held.Booking.ID; ledger(b) != held.Booking.TotalAmount {
			t.Fatalf("ledger of the held booking %d", ledger(b))
		}
		// The released spot can be sold again.
		if _, err := r.svc.Create(ctx, f.Cashier, est, f.Input(f.Spots[1], "2027-07-10", "2027-07-12")); err != nil {
			t.Fatalf("resell released spot: %v", err)
		}
	})
}

// TestConcurrency_CreateSameSpot_RealInventory is TestConcurrency_CreateSameSpot
// on the real inventory service (unique spot allocation key, savepoints).
func TestConcurrency_CreateSameSpot_RealInventory(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			ok, conflicts   int
			firstUnexpected error
		)
		start := make(chan struct{})
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, availability.ErrSpotUnavailable):
					conflicts++
				case firstUnexpected == nil:
					firstUnexpected = err
				}
			}()
		}
		close(start)
		wg.Wait()
		if firstUnexpected != nil {
			t.Fatalf("unexpected error: %v", firstUnexpected)
		}
		if ok != 1 || conflicts != parallel-1 {
			t.Fatalf("ok = %d, conflicts = %d", ok, conflicts)
		}
		if n := f.Count((*store.Booking)(nil), ""); n != 1 {
			t.Fatalf("bookings = %d", n)
		}
		if n := f.Count((*store.SpotAllocation)(nil), ""); n != 2 {
			t.Fatalf("allocations = %d", n)
		}
		if n := f.Count((*store.Customer)(nil), ""); n != 1 {
			t.Fatalf("customers = %d (orphans of refused creations)", n)
		}
	})
}
