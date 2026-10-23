//go:build integration

package checkout_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

const parallel = 120

// TestConcurrency_OnlineVsDesk is the acceptance criterion "no double sale
// between the cash desk and online under parallel load" (spec 28, M4):
// 120 parallel attempts on the same spot and day, alternating public
// checkouts (held booking + session) and desk sales (confirmed booking),
// produce exactly one sale; every loser fails with a typed conflict and
// leaves nothing behind (no booking, no orphan hold, no session).
func TestConcurrency_OnlineVsDesk(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		spot, day := e.F.Spots[0], "2027-07-10"
		var (
			wg                       sync.WaitGroup
			mu                       sync.Mutex
			online, desk, conflicts  int
			other                    []error
			winnerOnline, winnerDesk string
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				var err error
				var id string
				isOnline := i%2 == 0
				if isOnline {
					var s *checkout.Session
					s, err = e.Svc.Create(e.Ctx, slug, checkout.CreateInput{Selection: e.Seat(spot, day)})
					if err == nil {
						id = s.BookingID
					}
				} else {
					var d *booking.Detail
					d, err = e.Book.Create(e.Ctx, e.F.Cashier, e.F.Est.ID, e.F.Input(spot, day, day))
					if err == nil {
						id = d.Booking.ID
					}
				}
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil && isOnline:
					online++
					winnerOnline = id
				case err == nil:
					desk++
					winnerDesk = id
				case errors.As(err, &ce) && errors.Is(err, availability.ErrSpotUnavailable):
					conflicts++
				default:
					other = append(other, err)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if len(other) > 0 {
			t.Fatalf("unexpected errors (%d), first: %v", len(other), other[0])
		}
		if online+desk != 1 || conflicts != parallel-1 {
			t.Fatalf("online = %d, desk = %d, conflicts = %d", online, desk, conflicts)
		}
		winner := winnerOnline + winnerDesk
		counts := map[string]int{
			"bookings":    e.Count((*store.Booking)(nil), ""),
			"allocations": e.Count((*store.SpotAllocation)(nil), "spot_id = ? AND date = ?", spot.ID, calendar.MustParse(day)),
			"winner rows": e.Count((*store.SpotAllocation)(nil), "booking_id = ?", winner),
			"sessions":    e.Count((*store.CheckoutSession)(nil), ""),
			"orphan held": e.Count((*store.Booking)(nil), "status = 'held' AND id NOT IN (SELECT booking_id FROM spot_allocations WHERE booking_id IS NOT NULL)"),
		}
		want := map[string]int{"bookings": 1, "allocations": 2, "winner rows": 2, "sessions": online, "orphan held": 0}
		for k, v := range want {
			if counts[k] != v {
				t.Errorf("%s = %d, want %d", k, counts[k], v)
			}
		}
		// The losers' placeholder customers rolled back with their tx: at
		// most the one of the online winner remains, and it is hidden.
		if n := e.Count((*store.Customer)(nil), "id = ?", customer.PlaceholderID(e.F.Est.ID)); n > 1 {
			t.Fatalf("placeholders %d", n)
		}
	})
}

// TestConcurrency_ZoneAutoAssign: a zone-mode zone with 6 spots and an
// online quota of 4; 120 parallel public checkouts on the same day sell
// exactly 4 (the rest fail with quota_exceeded or pool_exhausted from the
// conditional pool UPDATE); confirming the 4 in parallel assigns 4
// distinct spots of the zone at confirmation (spec 7.5) and the quota
// counter never exceeds 4.
func TestConcurrency_ZoneAutoAssign(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		const quota = 4
		e := checkouttest.New(t, db, checkouttest.Options{ZoneSpots: 4, Quota: quota})
		day := "2027-07-25"
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			sessions  []*checkout.Session
			conflicts int
			other     []error
		)
		start := make(chan struct{})
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				s, err := e.Svc.Create(e.Ctx, slug, checkout.CreateInput{Selection: e.Zone(day)})
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil:
					sessions = append(sessions, s)
				case errors.As(err, &ce) && (errors.Is(err, availability.ErrQuotaExceeded) || errors.Is(err, availability.ErrPoolExhausted)):
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
		if len(sessions) != quota || conflicts != parallel-quota {
			t.Fatalf("sold %d, conflicts %d", len(sessions), conflicts)
		}
		var usage []store.PoolUsage
		if err := db.NewSelect().Model(&usage).Where("pool_id = ?", e.Pool.ID).Scan(e.Ctx); err != nil {
			t.Fatal(err)
		}
		for _, u := range usage {
			if u.OnlineUsed > quota || u.Used+u.Held > e.Pool.Capacity {
				t.Fatalf("pool usage %+v", u)
			}
		}

		// Confirm the sold ones in parallel: each gets its own spot.
		errs := make(chan error, len(sessions))
		for i, s := range sessions {
			wg.Add(1)
			go func(i int, s *checkout.Session) {
				defer wg.Done()
				if _, err := e.Svc.UpdateCustomer(e.Ctx, slug, s.BookingID, s.Token, checkout.CustomerInput{
					FirstName: "Cliente", LastName: fmt.Sprint(i), Email: fmt.Sprintf("c%d@example.org", i),
				}); err != nil {
					errs <- err
					return
				}
				_, err := e.Svc.Pay(e.Ctx, slug, s.BookingID, s.Token, checkout.PayInput{})
				errs <- err
			}(i, s)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("confirm: %v", err)
			}
		}
		seen := map[string]bool{}
		for _, s := range sessions {
			d, err := e.Book.DetailIn(e.Ctx, db, e.ES, s.BookingID)
			if err != nil {
				t.Fatal(err)
			}
			l := d.Lines[0]
			if d.Booking.Status != "confirmed" || l.AssignmentStatus != store.AssignmentAssigned || l.SpotID == nil {
				t.Fatalf("booking %s %s line %+v", d.Booking.ID, d.Booking.Status, l)
			}
			if seen[*l.SpotID] {
				t.Fatalf("spot %s assigned twice", *l.SpotID)
			}
			seen[*l.SpotID] = true
		}
		if n := e.Count((*store.SpotAllocation)(nil), "date = ? AND kind = ?", calendar.MustParse(day), store.AllocationBooking); n != 2*quota {
			t.Fatalf("spot rows %d", n)
		}
	})
}
