//go:build integration

package booking_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// groupInput is a group booking made from the operational map (ADR 0019 C):
// one booking with one line per selected spot.
func groupInput(f *bookingtest.Fixture, day string, spots ...*store.Spot) booking.CreateInput {
	in := f.Input(spots[0], day, day)
	in.Lines = in.Lines[:0]
	for _, s := range spots {
		in.Lines = append(in.Lines, booking.LineInput{PackID: f.Base.ID, SpotID: s.ID})
	}
	return in
}

// TestGroupBooking_AllOrNothing: a group booking on three spots creates one
// booking with three lines and their allocations, priced by the engine; a
// second group overlapping on a single spot fails as a whole and writes
// nothing (no partial lines, no allocations, no audit, no outbox).
func TestGroupBooking_AllOrNothing(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		det, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, groupInput(f, "2027-07-10", f.Spots[0], f.Spots[1], f.Spots[2]))
		if err != nil {
			t.Fatalf("group create: %v", err)
		}
		if len(det.Lines) != 3 || len(det.SpotAllocations) != 6 || det.Booking.TotalAmount != 3*bookingtest.BasePerDay {
			t.Fatalf("group: %d lines, %d allocations, total %d", len(det.Lines), len(det.SpotAllocations), det.Booking.TotalAmount)
		}
		before := map[string]int{
			"bookings":    f.Count((*store.Booking)(nil), ""),
			"lines":       f.Count((*store.BookingLine)(nil), ""),
			"allocations": f.Count((*store.SpotAllocation)(nil), ""),
			"audit":       f.Count((*store.AuditEntry)(nil), "entity_type = 'booking'"),
			"outbox":      f.Count((*store.OutboxEvent)(nil), "type LIKE 'booking.%'"),
		}
		_, err = r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, groupInput(f, "2027-07-10", f.Spots[3], f.Spots[2]))
		var ce *availability.ConflictError
		if !errors.As(err, &ce) || !errors.Is(err, availability.ErrSpotUnavailable) {
			t.Fatalf("overlapping group: %v", err)
		}
		after := map[string]int{
			"bookings":    f.Count((*store.Booking)(nil), ""),
			"lines":       f.Count((*store.BookingLine)(nil), ""),
			"allocations": f.Count((*store.SpotAllocation)(nil), ""),
			"audit":       f.Count((*store.AuditEntry)(nil), "entity_type = 'booking'"),
			"outbox":      f.Count((*store.OutboxEvent)(nil), "type LIKE 'booking.%'"),
		}
		for k, v := range before {
			if after[k] != v {
				t.Errorf("%s: %d after the refused group, want %d", k, after[k], v)
			}
		}
		if n := f.Count((*store.SpotAllocation)(nil), "spot_id = ?", f.Spots[3].ID); n != 0 {
			t.Fatalf("free spot of the refused group has %d allocations", n)
		}
	})
}

// TestConcurrency_GroupBookingOverlap: 100 parallel creations on the same
// day, half group bookings of spots A1+A2+A3 and half single bookings of A2
// (the overlapping spot). Exactly one wins; a winning group holds all three
// spots, and no spot/date/slot is ever allocated twice (INV-01). Losers leave
// no row behind (all-or-nothing).
func TestConcurrency_GroupBookingOverlap(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		r := newReal(f)
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			groupWins int
			singleOK  int
			conflicts int
			other     []error
		)
		start := make(chan struct{})
		for i := range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				group := i%2 == 0
				in := f.Input(f.Spots[1], "2027-07-10", "2027-07-10")
				if group {
					in = groupInput(f, "2027-07-10", f.Spots[0], f.Spots[1], f.Spots[2])
				}
				<-start
				_, err := r.svc.Create(f.Ctx, f.Cashier, f.Est.ID, in)
				mu.Lock()
				defer mu.Unlock()
				var ce *availability.ConflictError
				switch {
				case err == nil && group:
					groupWins++
				case err == nil:
					singleOK++
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
		if groupWins+singleOK != 1 || conflicts != parallel-1 {
			t.Fatalf("group wins %d, single wins %d, conflicts %d", groupWins, singleOK, conflicts)
		}
		wantLines, wantAlloc := 1, 2
		if groupWins == 1 {
			wantLines, wantAlloc = 3, 6
		}
		if n := f.Count((*store.Booking)(nil), ""); n != 1 {
			t.Errorf("bookings = %d, want 1", n)
		}
		if n := f.Count((*store.BookingLine)(nil), ""); n != wantLines {
			t.Errorf("lines = %d, want %d", n, wantLines)
		}
		if n := f.Count((*store.SpotAllocation)(nil), ""); n != wantAlloc {
			t.Errorf("allocations = %d, want %d", n, wantAlloc)
		}
		// The overlapping spot is allocated once per slot, whoever won.
		if n := f.Count((*store.SpotAllocation)(nil), "spot_id = ?", f.Spots[1].ID); n != 2 {
			t.Errorf("A2 allocations = %d, want 2", n)
		}
	})
}
