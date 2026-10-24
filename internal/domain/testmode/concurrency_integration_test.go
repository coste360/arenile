//go:build integration

package testmode_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/testmode"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestConcurrency_PurgeVsCreate (INV-76): 100 parallel operations while
// test mode is on, half creations of test bookings and half purges. A real
// booking created before the switch is never touched; every surviving
// booking is a test booking created after the last purge, with its
// allocations consistent (a purge deletes whole bookings, never a line or
// an allocation alone); a final purge leaves no test row behind.
func TestConcurrency_PurgeVsCreate(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
		if err != nil {
			t.Fatal(err)
		}
		real := f.MustCreate(f.Input(f.Spots[0], "2027-07-01", "2027-07-01"))
		row, err := store.EnsureTestModeSettings(f.Ctx, db, es, f.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		row.Enabled = true
		if ok, err := store.UpdateTestModeSettingsVersioned(f.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("enable test mode: %v %v", ok, err)
		}
		svc := testmode.NewService(testmode.Deps{
			DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock), Inventory: f.Inv,
		})
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			other []error
		)
		start := make(chan struct{})
		for i := range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				var err error
				if i%2 == 0 {
					day := f.Clock.Now().AddDate(0, 0, 10+i/2).Format("2006-01-02")
					_, err = f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, f.Input(f.Spots[1], day, day))
				} else {
					_, err = svc.Purge(f.Ctx, f.Owner, f.Est.ID, false)
				}
				if err != nil {
					mu.Lock()
					other = append(other, err)
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if b, err := store.GetBooking(f.Ctx, db, es, real.Booking.ID); err != nil || b.Test || b.Status != real.Booking.Status {
			t.Fatalf("real booking touched: %v %+v", err, b)
		}
		if n := f.Count((*store.Booking)(nil), "test = ?", false); n != 1 {
			t.Fatalf("real bookings %d, want 1", n)
		}
		// Every surviving test booking is whole: its lines and allocations exist.
		var survivors []store.Booking
		if err := db.NewSelect().Model(&survivors).Where("establishment_id = ? AND test = ?", f.Est.ID, true).Scan(f.Ctx); err != nil {
			t.Fatal(err)
		}
		for _, b := range survivors {
			if f.Count((*store.BookingLine)(nil), "booking_id = ?", b.ID) != 1 || f.Count((*store.SpotAllocation)(nil), "booking_id = ?", b.ID) != 2 {
				t.Fatalf("test booking %s left partial by a concurrent purge", b.ID)
			}
		}
		res, err := svc.Purge(f.Ctx, f.Owner, f.Est.ID, false)
		if err != nil || res.Bookings != int64(len(survivors)) {
			t.Fatalf("final purge: %v %+v (survivors %d)", err, res, len(survivors))
		}
		if n := f.Count((*store.Booking)(nil), "test = ?", true); n != 0 {
			t.Fatalf("test bookings after the final purge: %d", n)
		}
		if n := f.Count((*store.SpotAllocation)(nil), "booking_id <> ?", real.Booking.ID); n != 0 {
			t.Fatalf("allocations of purged bookings left: %d", n)
		}
		if n := f.Count((*store.AuditEntry)(nil), "action = ?", testmode.ActionPurged); n < 1 {
			t.Fatal("purge audit missing")
		}
	})
}

// TestPurge_RefusedWhenTestCreditSpentByRealBooking (INV-76): a credit
// granted by a test booking and already spent on a real booking blocks the
// purge (dry run and real run, 409) until the credits are corrected;
// deleting the grant would leave real money paid with credit that never
// existed.
func TestPurge_RefusedWhenTestCreditSpentByRealBooking(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		es, err := store.SystemEstablishmentScope(f.Ctx, db, f.Est.ID)
		if err != nil {
			t.Fatal(err)
		}
		real := f.MustCreate(f.Input(f.Spots[0], "2027-07-01", "2027-07-01"))
		row, err := store.EnsureTestModeSettings(f.Ctx, db, es, f.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		row.Enabled = true
		if ok, err := store.UpdateTestModeSettingsVersioned(f.Ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("enable test mode: %v %v", ok, err)
		}
		test := f.MustCreate(f.Input(f.Spots[1], "2027-07-02", "2027-07-02"))
		now := f.Clock.Now()
		cust := real.Booking.CustomerID
		grant := &store.CustomerCredit{
			ID: store.NewID(), EstablishmentID: f.Est.ID, CustomerID: cust, Amount: 1000, Source: store.CreditSourceCancellation,
			BookingID: &test.Booking.ID, Note: "prova", CreatedAt: now,
		}
		spent := &store.CustomerCredit{
			ID: store.NewID(), EstablishmentID: f.Est.ID, CustomerID: cust, Amount: -1000, Source: store.CreditSourceRedemption,
			BookingID: &real.Booking.ID, Note: "saldo", CreatedAt: now,
		}
		f.Insert(grant)
		f.Insert(spent)
		f.Insert(&store.CustomerCreditBalance{EstablishmentID: f.Est.ID, CustomerID: cust, Balance: 0, UpdatedAt: now})

		svc := testmode.NewService(testmode.Deps{
			DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock), Inventory: f.Inv,
		})
		if _, err := svc.Purge(f.Ctx, f.Owner, f.Est.ID, true); !errors.Is(err, testmode.ErrTestCreditRedeemed) {
			t.Fatalf("dry run: %v", err)
		}
		if _, err := svc.Purge(f.Ctx, f.Owner, f.Est.ID, false); !errors.Is(err, testmode.ErrTestCreditRedeemed) {
			t.Fatalf("purge: %v", err)
		}
		if n := f.Count((*store.Booking)(nil), "test = ?", true); n != 1 {
			t.Fatalf("test booking deleted by a refused purge: %d left", n)
		}
		// The manager corrects the ledger (a goodwill grant covers the spent
		// credit): the purge runs and the real booking keeps its credit.
		f.Insert(&store.CustomerCredit{
			ID: store.NewID(), EstablishmentID: f.Est.ID, CustomerID: cust, Amount: 1000, Source: store.CreditSourceGoodwill,
			Note: "regolarizzazione", CreatedAt: now,
		})
		if _, err := db.NewUpdate().Model((*store.CustomerCreditBalance)(nil)).Set("balance = ?", 1000).
			Where("establishment_id = ? AND customer_id = ?", f.Est.ID, cust).Exec(f.Ctx); err != nil {
			t.Fatal(err)
		}
		res, err := svc.Purge(f.Ctx, f.Owner, f.Est.ID, false)
		if err != nil || res.Bookings != 1 {
			t.Fatalf("purge after the correction: %v %+v", err, res)
		}
		var bal store.CustomerCreditBalance
		if err := db.NewSelect().Model(&bal).Where("establishment_id = ? AND customer_id = ?", f.Est.ID, cust).Scan(f.Ctx); err != nil || bal.Balance != 0 {
			t.Fatalf("balance after purge: %v %+v", err, bal)
		}
		if n := f.Count((*store.CustomerCredit)(nil), "source = ?", store.CreditSourceAdjustment); n != 0 {
			t.Fatalf("automatic adjustment rows written: %d", n)
		}
	})
}
