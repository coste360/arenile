//go:build integration

package booking_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const parallel = 100

// TestConcurrency_CreateSameSpot: 100 desk creations of the same spot on the
// same day produce exactly one booking; the others fail with a typed
// conflict and leave no row behind (booking, lines, inline customer,
// allocations, audit, outbox: INV-01, INV-18).
func TestConcurrency_CreateSameSpot(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			ok        int
			conflicts int
			other     []error
		)
		start := make(chan struct{})
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil:
					ok++
				case errors.As(err, &ce) && errors.Is(err, availability.ErrSpotUnavailable):
					conflicts++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if ok != 1 || conflicts != parallel-1 {
			t.Fatalf("ok = %d, conflicts = %d", ok, conflicts)
		}
		counts := map[string]int{
			"bookings":    f.Count((*store.Booking)(nil), ""),
			"lines":       f.Count((*store.BookingLine)(nil), ""),
			"customers":   f.Count((*store.Customer)(nil), ""),
			"allocations": f.Count((*store.SpotAllocation)(nil), ""),
			"audit":       f.Count((*store.AuditEntry)(nil), "entity_type = 'booking'"),
			"outbox":      f.Count((*store.OutboxEvent)(nil), "type LIKE 'booking.%'"),
		}
		want := map[string]int{"bookings": 1, "lines": 1, "customers": 1, "allocations": 2, "audit": 1, "outbox": 2}
		for k, v := range want {
			if counts[k] != v {
				t.Errorf("%s = %d, want %d", k, counts[k], v)
			}
		}
	})
}

// TestConcurrency_CreateDistinctSpotsUniqueCodes: parallel creations on
// different days all succeed and get distinct sequential codes.
func TestConcurrency_CreateDistinctSpotsUniqueCodes(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		const n = 40
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				day := bookingtest.D("2027-07-01").AddDays(i % 20).String()
				_, err := f.Svc.Create(f.Ctx, f.Cashier, f.Est.ID, f.Input(f.Spots[i/20], day, day))
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("create: %v", err)
			}
		}
		var codes []string
		if err := db.NewSelect().Model((*store.Booking)(nil)).Column("code").Where("establishment_id = ?", f.Est.ID).
			OrderExpr("code ASC").Scan(f.Ctx, &codes); err != nil {
			t.Fatal(err)
		}
		if len(codes) != n || codes[0] != "LD-2027-000001" || codes[n-1] != "LD-2027-000040" {
			t.Fatalf("codes %v", codes)
		}
	})
}

// TestConcurrency_ModifyVsModify: 100 modifications sent with the same
// If-Match version: exactly one applies, the others get 412
// version_conflict, and the allocations match the winner.
func TestConcurrency_ModifyVsModify(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		det := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			ok        int
			conflicts int
			other     []error
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				ends := bookingtest.D("2027-07-10").AddDays(1 + i%5)
				v := int64(1)
				_, err := f.Svc.Update(f.Ctx, f.Cashier, f.Est.ID, det.Booking.ID, &v, booking.PatchInput{EndsOn: &ends})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, configerr.ErrVersionConflict):
					conflicts++
				default:
					other = append(other, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if ok != 1 || conflicts != parallel-1 {
			t.Fatalf("ok = %d, conflicts = %d", ok, conflicts)
		}
		got, err := f.Svc.Get(f.Ctx, f.Owner, f.Est.ID, det.Booking.ID)
		if err != nil {
			t.Fatal(err)
		}
		days := got.Booking.StartsOn.DaysUntil(got.Booking.EndsOn) + 1
		if got.Booking.Version != 2 || len(got.SpotAllocations) != 2*days || got.Booking.TotalAmount != bookingtestPrice(days) {
			t.Fatalf("winner state: version %d, %d allocations for %d days, total %d", got.Booking.Version, len(got.SpotAllocations), days, got.Booking.TotalAmount)
		}
	})
}

func bookingtestPrice(days int) money.Cents { return money.Cents(days * bookingtest.BasePerDay) }
