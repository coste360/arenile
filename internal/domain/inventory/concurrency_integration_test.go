//go:build integration

package inventory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// attempts is the number of parallel attempts on the same resource
// (AGENTS.md: at least 100).
const attempts = 120

// race runs fn attempts times in parallel and counts successes and
// conflicts; any other error fails the test.
func race(t *testing.T, fn func(i int) error, isConflict func(error) bool) (ok, refused int64) {
	t.Helper()
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := fn(i)
			switch {
			case err == nil:
				atomic.AddInt64(&ok, 1)
			case isConflict(err):
				atomic.AddInt64(&refused, 1)
			default:
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected error: %v", err)
	}
	return ok, refused
}

func isAvailabilityConflict(err error) bool {
	var ce *availability.ConflictError
	return errors.As(err, &ce)
}

// INV-01: exactly one of many parallel bookings of the same spot/day wins.
func TestConcurrency_SpotAllocation(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		spec := bookingSpec{from: "2027-07-02", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}
		ok, refused := race(t, func(int) error { _, err := f.book(t, spec, nil); return err }, isAvailabilityConflict)
		if ok != 1 || refused != attempts-1 {
			t.Fatalf("ok=%d refused=%d", ok, refused)
		}
		if n := f.count(t, "spot_allocations", "spot_id = ?", f.spots["A1"].ID); n != 4 {
			t.Fatalf("rows = %d", n)
		}
		if n := f.count(t, "bookings", "1 = 1"); n != 1 {
			t.Fatalf("bookings = %d (losers must roll back)", n)
		}
		f.checkInvariants(t)
	})
}

// INV-01 through panel holds (the name listed in docs/invariants.md).
func TestConcurrency_SpotHold_NoDoubleAllocation(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		in := inventory.HoldInput{StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsFull, Items: []store.HoldItem{{SpotID: f.spots["A2"].ID}}}
		ok, refused := race(t, func(int) error { _, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, in); return err }, isAvailabilityConflict)
		if ok != 1 || refused != attempts-1 {
			t.Fatalf("ok=%d refused=%d", ok, refused)
		}
		if n := f.count(t, "holds", "1 = 1"); n != 1 {
			t.Fatalf("holds = %d", n)
		}
		f.checkInvariants(t)
	})
}

// INV-02: a pool of capacity N accepts exactly N units out of many
// parallel attempts.
func TestConcurrency_PoolUsage(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		spec := bookingSpec{from: "2027-07-04", to: "2027-07-04", lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}
		ok, refused := race(t, func(int) error { _, err := f.book(t, spec, nil); return err }, isAvailabilityConflict)
		if ok != int64(f.qtyPool.Capacity) || refused != attempts-ok {
			t.Fatalf("ok=%d refused=%d", ok, refused)
		}
		if u, _, _ := f.usage(t, f.qtyPool.ID, "2027-07-04", calendar.PM); u != f.qtyPool.Capacity {
			t.Fatalf("used = %d", u)
		}
		f.checkInvariants(t)
	})
}

// INV-02 with mixed holds and bookings of different sizes on a component
// pool: the total never exceeds the capacity.
func TestConcurrency_PoolConsume_NeverExceedsCapacity(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		var units int64
		ok, _ := race(t, func(i int) error {
			qty := 1 + i%3
			var err error
			if i%2 == 0 {
				_, err = f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
					StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"), Slots: calendar.SlotsAM,
					Items: []store.HoldItem{{PoolID: f.sunbeds.ID, Quantity: qty}},
				})
			} else {
				_, err = f.book(t, bookingSpec{from: "2027-07-05", to: "2027-07-05", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}, map[string]int{"sunbed": qty})
			}
			if err == nil {
				atomic.AddInt64(&units, int64(qty))
			}
			return err
		}, isAvailabilityConflict)
		u, h, _ := f.usage(t, f.sunbeds.ID, "2027-07-05", calendar.AM)
		if ok == 0 || int64(u+h) != units || u+h > f.sunbeds.Capacity {
			t.Fatalf("ok=%d units=%d used=%d held=%d", ok, units, u, h)
		}
		f.checkInvariants(t)
	})
}

// INV-09: the online quota holds under contention (count 2 of capacity 4
// on the quantity zone; 40% of 5 spots on the seat zone).
func TestConcurrency_OnlineQuota(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		spec := bookingSpec{channel: store.ChannelOnline, from: "2027-07-06", to: "2027-07-06", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}
		ok, _ := race(t, func(int) error { _, err := f.book(t, spec, nil); return err }, isAvailabilityConflict)
		if ok != 2 {
			t.Fatalf("online quantity ok = %d, want 2", ok)
		}
		labels := []string{"A1", "A2", "A3", "A4", "A5"}
		ok, _ = race(t, func(i int) error {
			_, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-07", to: "2027-07-07", lines: []lineSpec{{zone: f.seat, spot: f.spots[labels[i%5]]}}}, nil)
			return err
		}, isAvailabilityConflict)
		if ok != 2 {
			t.Fatalf("online seat ok = %d, want 2", ok)
		}
		f.checkInvariants(t)
	})
}

// A panel hold is converted exactly once, even with concurrent releases
// and conversions to different bookings.
func TestConcurrency_HoldConversion(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-08"), EndsOn: d("2027-07-09"), Slots: calendar.SlotsFull,
			Items: []store.HoldItem{{SpotID: f.spots["A5"].ID}, {ZoneID: f.qtyZone.ID, Quantity: 3}},
		})
		must(t, err)
		var released int64
		ok, refused := race(t, func(i int) error {
			if i%10 == 9 {
				err := f.svc.ReleaseHold(ctx, f.cashier, f.est.ID, h.ID, "")
				if err == nil {
					atomic.AddInt64(&released, 1)
					return errSkip
				}
				return err
			}
			return store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
				tb := f.insertBooking(t, tx, bookingSpec{from: "2027-07-08", to: "2027-07-09", lines: []lineSpec{{zone: f.seat, spot: f.spots["A5"]}, {zone: f.qtyZone, qty: 3}}})
				_, err := f.svc.ConvertHold(ctx, tx, f.es, inventory.ConvertHoldRequest{
					HoldID: h.ID, BookingID: tb.id,
					Lines: []inventory.LineRequest{{LineID: tb.lineIDs[0], SpotID: f.spots["A5"].ID}, {LineID: tb.lineIDs[1], ZoneID: f.qtyZone.ID, Quantity: 3}},
				})
				return err
			})
		}, func(err error) bool {
			return errors.Is(err, inventory.ErrHoldClosed) || errors.Is(err, errSkip) || isAvailabilityConflict(err)
		})
		_ = refused
		v, err := f.svc.GetHold(ctx, f.owner, f.est.ID, h.ID)
		must(t, err)
		bookings := f.count(t, "bookings", "1 = 1")
		switch v.Status {
		case inventory.HoldConverted:
			if ok != 1 || bookings != 1 {
				t.Fatalf("converted: ok=%d bookings=%d", ok, bookings)
			}
			if n := f.count(t, "spot_allocations", "booking_id = ?", *v.ConvertedBookingID); n != 4 {
				t.Fatalf("rows = %d", n)
			}
		case inventory.HoldReleased:
			if ok != 0 || bookings != 0 || f.count(t, "spot_allocations", "1 = 1") != 0 {
				t.Fatalf("released: ok=%d bookings=%d", ok, bookings)
			}
		default:
			t.Fatalf("status = %s", v.Status)
		}
		if u, held, _ := f.usage(t, f.qtyPool.ID, "2027-07-08", calendar.AM); held != 0 || (u != 0 && u != 3) {
			t.Fatalf("pool used %d held %d", u, held)
		}
		f.checkInvariants(t)
	})
}

var errSkip = errors.New("skip")

// The sweeper and users release expired holds concurrently: each hold is
// released once and the counters stay consistent.
func TestConcurrency_HoldSweeper(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		for i := 0; i < 5; i++ {
			_, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
				StartsOn: d("2027-07-10"), EndsOn: d("2027-07-10"), Slots: calendar.SlotsAM, TTL: time.Minute,
				Items: []store.HoldItem{{PoolID: f.sunbeds.ID, Quantity: 2}},
			})
			must(t, err)
		}
		f.clock.Advance(2 * time.Minute)
		var total int64
		race(t, func(int) error {
			n, err := f.svc.SweepExpiredHolds(ctx)
			atomic.AddInt64(&total, int64(n))
			return err
		}, func(error) bool { return false })
		if total != 5 || f.count(t, "outbox_events", "type = 'hold.released'") != 5 {
			t.Fatalf("released %d", total)
		}
		f.checkInvariants(t)
	})
}
