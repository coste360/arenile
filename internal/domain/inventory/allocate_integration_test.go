//go:build integration

package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func conflictOf(t *testing.T, err error, kind error) *availability.ConflictError {
	t.Helper()
	var ce *availability.ConflictError
	if !errors.As(err, &ce) || !errors.Is(err, kind) {
		t.Fatalf("want conflict %v, got %v", kind, err)
	}
	return ce
}

// INV-07: a FULL line has AM and PM rows on every date, closures excluded.
func TestSpotHold_FullDayAllocatesBothSlots(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		tb, err := f.book(t, bookingSpec{from: "2027-07-19", to: "2027-07-21", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		// 3 days x 2 slots minus the closed PM of 2027-07-20.
		if n := f.count(t, "spot_allocations", "booking_id = ?", tb.id); n != 5 {
			t.Fatalf("rows = %d, want 5", n)
		}
		if n := f.count(t, "spot_allocations", "booking_id = ? AND date = ? AND slot = 'PM'", tb.id, d("2027-07-20")); n != 0 {
			t.Fatal("closed half-day allocated")
		}
		f.checkInvariants(t)
	})
}

// INV-01 / spec 7.2: a taken spot is refused with every conflicting
// half-day and the transaction leaves nothing behind.
func TestAllocate_SpotConflictListsDates(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-03", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-01", to: "2027-07-04", lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, map[string]int{"sunbed": 2})
		ce := conflictOf(t, err, availability.ErrSpotUnavailable)
		if len(ce.Conflicts) != 2 || !ce.Conflicts[0].Date.Equal(d("2027-07-02")) || ce.Conflicts[1].Slot != calendar.AM {
			t.Fatalf("conflicts = %+v", ce.Conflicts)
		}
		if n := f.count(t, "bookings", "1 = 1"); n != 1 {
			t.Fatalf("rolled back booking still there: %d", n)
		}
		if u, h, _ := f.usage(t, f.sunbeds.ID, "2027-07-01", calendar.AM); u != 0 || h != 0 {
			t.Fatalf("pool touched by a refused allocation: %d/%d", u, h)
		}
		f.checkInvariants(t)
	})
}

// INV-02 with capacity overrides; refused half-days are listed.
func TestPoolConsume_CapacityOverride(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{from: "2027-08-14", to: "2027-08-14", lines: []lineSpec{{zone: f.qtyZone, qty: 4}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-08-15", to: "2027-08-15", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 2}}}, nil)
		ce := conflictOf(t, err, availability.ErrPoolExhausted)
		if len(ce.Conflicts) != 1 || ce.Conflicts[0].PoolID != f.qtyPool.ID || ce.Conflicts[0].Reason != availability.ReasonPoolExhausted {
			t.Fatalf("conflicts = %+v", ce.Conflicts)
		}
		_, err = f.book(t, bookingSpec{from: "2027-08-15", to: "2027-08-15", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}, nil)
		must(t, err)
		if u, _, _ := f.usage(t, f.qtyPool.ID, "2027-08-15", calendar.AM); u != 1 {
			t.Fatalf("used = %d", u)
		}
		f.checkInvariants(t)
	})
}

func TestAllocate_ZoneModeAndComponents(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		var alloc *inventory.Allocation
		err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			spec := bookingSpec{from: "2027-07-05", to: "2027-07-06", lines: []lineSpec{{zone: f.zoneMode, status: store.AssignmentPending}}}
			tb := f.insertBooking(t, tx, spec)
			var err error
			alloc, err = f.svc.Allocate(ctx, tx, f.es, f.allocRequest(spec, tb, map[string]int{"sunbed": 2}))
			return err
		})
		must(t, err)
		if alloc.Lines[0].AssignmentStatus != store.AssignmentPending || alloc.SpotRows != 0 || len(alloc.PoolIDs) != 2 || len(alloc.Dates) != 2 {
			t.Fatalf("allocation = %+v", alloc)
		}
		if u, _, _ := f.usage(t, f.zonePool.ID, "2027-07-06", calendar.PM); u != 1 {
			t.Fatalf("zone pool used = %d", u)
		}
		if u, _, _ := f.usage(t, f.sunbeds.ID, "2027-07-05", calendar.AM); u != 2 {
			t.Fatalf("sunbeds used = %d", u)
		}
		// fill the zone: 2 more pending lines, the 4th is refused.
		for i := 0; i < 2; i++ {
			_, err := f.book(t, bookingSpec{from: "2027-07-05", to: "2027-07-05", lines: []lineSpec{{zone: f.zoneMode, status: store.AssignmentPending}}}, nil)
			must(t, err)
		}
		_, err = f.book(t, bookingSpec{from: "2027-07-05", to: "2027-07-05", slots: calendar.SlotsPM, lines: []lineSpec{{zone: f.zoneMode, status: store.AssignmentPending}}}, nil)
		conflictOf(t, err, availability.ErrPoolExhausted)
		f.checkInvariants(t)
	})
}

// INV-09: the online quota binds online/api, never the desk; pools use
// online_used, seat zones count online rows under the zone lock.
func TestInventory_OnlineQuota(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		// quantity zone: quota count 2 of capacity 4.
		_, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-08", to: "2027-07-08", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 2}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{channel: store.ChannelAPI, from: "2027-07-08", to: "2027-07-08", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}, nil)
		ce := conflictOf(t, err, availability.ErrQuotaExceeded)
		if ce.Conflicts[0].ZoneID != f.qtyZone.ID || ce.Conflicts[0].Reason != availability.ReasonQuotaExceeded {
			t.Fatalf("conflicts = %+v", ce.Conflicts)
		}
		_, err = f.book(t, bookingSpec{from: "2027-07-08", to: "2027-07-08", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.qtyZone, qty: 2}}}, nil)
		must(t, err) // desk ignores the quota
		if u, _, o := f.usage(t, f.qtyPool.ID, "2027-07-08", calendar.AM); u != 4 || o != 2 {
			t.Fatalf("used %d online %d", u, o)
		}
		// seat zone: 40% of 5 spots = 2 online per half-day.
		for _, label := range []string{"A1", "A2"} {
			_, err := f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-09", to: "2027-07-09", lines: []lineSpec{{zone: f.seat, spot: f.spots[label]}}}, nil)
			must(t, err)
		}
		_, err = f.book(t, bookingSpec{channel: store.ChannelOnline, from: "2027-07-09", to: "2027-07-09", slots: calendar.SlotsPM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}}}, nil)
		ce = conflictOf(t, err, availability.ErrQuotaExceeded)
		if len(ce.Conflicts) != 1 || ce.Conflicts[0].ZoneID != f.seat.ID || ce.Conflicts[0].Slot != calendar.PM {
			t.Fatalf("seat quota conflicts = %+v", ce.Conflicts)
		}
		_, err = f.book(t, bookingSpec{from: "2027-07-09", to: "2027-07-09", lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}}}, nil)
		must(t, err)
		// Check (dry run) agrees.
		ctx := context.Background()
		conflicts, err := f.svc.Check(ctx, db, f.es, inventory.AllocateRequest{
			BookingID: store.NewID(), Channel: store.ChannelOnline,
			StartsOn: d("2027-07-09"), EndsOn: d("2027-07-09"), Slots: calendar.SlotsAM, Kind: inventory.KindBooking,
			Lines: []inventory.LineRequest{{LineID: "x", SpotID: f.spots["A4"].ID}},
		})
		must(t, err)
		if len(conflicts) != 1 || conflicts[0].Reason != availability.ReasonQuotaExceeded {
			t.Fatalf("check = %+v", conflicts)
		}
		// A quote has no owner yet: Check works without one, Allocate does not.
		quote := inventory.AllocateRequest{
			Channel: store.ChannelDesk, StartsOn: d("2027-07-09"), EndsOn: d("2027-07-09"), Slots: calendar.SlotsFull, Kind: inventory.KindBooking,
			Lines: []inventory.LineRequest{{LineID: "q", SpotID: f.spots["A3"].ID}},
		}
		conflicts, err = f.svc.Check(ctx, db, f.es, quote)
		must(t, err)
		if len(conflicts) != 2 || conflicts[0].Reason != availability.ReasonSpotUnavailable { // A3 taken AM and PM
			t.Fatalf("ownerless check = %+v", conflicts)
		}
		if _, err := f.svc.Allocate(ctx, db, f.es, quote); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("ownerless allocate err = %v, want validation", err)
		}
		f.checkInvariants(t)
	})
}

// INV-05: expired holds never block a sale, even before the sweeper.
func TestSpotHold_ExpiredHoldIsReclaimed(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-10"), EndsOn: d("2027-07-10"), Slots: calendar.SlotsFull,
			Items: []store.HoldItem{{SpotID: f.spots["A1"].ID}, {PoolID: f.sunbeds.ID, Quantity: 10}},
		})
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-10", to: "2027-07-10", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		conflictOf(t, err, availability.ErrSpotUnavailable)
		f.clock.Advance(inventory.DefaultHoldTTL + time.Second)
		_, err = f.book(t, bookingSpec{from: "2027-07-10", to: "2027-07-10", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, map[string]int{"sunbed": 3})
		must(t, err)
		if u, held, _ := f.usage(t, f.sunbeds.ID, "2027-07-10", calendar.AM); u != 3 || held != 0 {
			t.Fatalf("pool after reclaim: used %d held %d", u, held)
		}
		v, err := f.svc.GetHold(ctx, f.viewer, f.est.ID, h.ID)
		must(t, err)
		if v.Status != inventory.HoldExpired {
			t.Fatalf("status = %s", v.Status)
		}
		f.checkInvariants(t)
	})
}

func TestInventory_ConfirmExtendRelease(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		exp := f.clock.Now().Add(10 * time.Minute)
		spec := bookingSpec{
			status: "held", channel: store.ChannelOnline, expires: &exp, from: "2027-07-12", to: "2027-07-13",
			lines: []lineSpec{{zone: f.seat, spot: f.spots["A5"]}},
		}
		tb, err := f.book(t, spec, map[string]int{"sunbed": 1})
		must(t, err)
		if _, h, o := f.usage(t, f.sunbeds.ID, "2027-07-12", calendar.AM); h != 1 || o != 1 {
			t.Fatalf("held %d online %d", h, o)
		}
		later := exp.Add(20 * time.Minute)
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.ExtendHold(ctx, tx, f.es, tb.id, later) }))
		f.clock.Advance(15 * time.Minute) // past the first expiry, before the extended one
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.NewUpdate().TableExpr("bookings").Set("hold_expires_at = ?", later).Where("id = ?", tb.id).Exec(ctx); err != nil {
				return err
			}
			return f.svc.Confirm(ctx, tx, f.es, tb.id)
		}))
		if n := f.count(t, "spot_allocations", "booking_id = ? AND kind = 'booking' AND expires_at IS NULL", tb.id); n != 4 {
			t.Fatalf("confirmed rows = %d", n)
		}
		if u, h, o := f.usage(t, f.sunbeds.ID, "2027-07-13", calendar.PM); u != 1 || h != 0 || o != 1 {
			t.Fatalf("after confirm used %d held %d online %d", u, h, o)
		}
		f.checkInvariants(t)
		from := d("2027-07-13")
		var rel *inventory.Allocation
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			var err error
			rel, err = f.svc.Release(ctx, tx, f.es, inventory.ReleaseRequest{BookingID: tb.id, From: &from})
			return err
		}))
		if rel.SpotRows != 2 || rel.PoolRows != 2 || len(rel.Dates) != 1 {
			t.Fatalf("release = %+v", rel)
		}
		if u, _, o := f.usage(t, f.sunbeds.ID, "2027-07-13", calendar.AM); u != 0 || o != 0 {
			t.Fatalf("after release used %d online %d", u, o)
		}
		f.checkInvariants(t)
	})
}

func TestInventoryConfirm_ExpiredHold(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		exp := f.clock.Now().Add(time.Minute)
		tb, err := f.book(t, bookingSpec{status: "held", expires: &exp, from: "2027-07-12", to: "2027-07-12", lines: []lineSpec{{zone: f.seat, spot: f.spots["A5"]}}}, nil)
		must(t, err)
		f.clock.Advance(2 * time.Minute)
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.Confirm(ctx, tx, f.es, tb.id) })
		if !errors.Is(err, availability.ErrHoldExpired) {
			t.Fatalf("confirm expired: %v", err)
		}
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			return f.svc.ExtendHold(ctx, tx, f.es, tb.id, f.clock.Now().Add(time.Hour))
		})
		if !errors.Is(err, availability.ErrHoldExpired) {
			t.Fatalf("extend expired: %v", err)
		}
	})
}

// TestInventoryConfirm_ReclaimedHold: a hold whose rows were reclaimed by a
// concurrent allocation (INV-05) cannot be confirmed, even when the booking
// row still says the hold is valid; a partial reclaim (some rows) fails too.
// Without this a confirmed booking could end up with no allocation (INV-20).
func TestInventoryConfirm_ReclaimedHold(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		exp := f.clock.Now().Add(time.Minute)
		hold, err := f.book(t, bookingSpec{status: "held", expires: &exp, from: "2027-07-13", to: "2027-07-13", lines: []lineSpec{{zone: f.seat, spot: f.spots["A4"]}}}, map[string]int{"sunbed": 1})
		must(t, err)
		// Two minutes later a sale that needs only the sunbed pool reclaims
		// the expired hold whole: its spot rows go too (never a partial set).
		f.clock.Advance(2 * time.Minute)
		_, err = f.book(t, bookingSpec{from: "2027-07-13", to: "2027-07-13", lines: []lineSpec{{zone: f.qtyZone, qty: 1}}}, map[string]int{"sunbed": 1})
		must(t, err)
		if n := f.count(t, "spot_allocations", "booking_id = ?", hold.id) + f.count(t, "pool_allocations", "booking_id = ?", hold.id); n != 0 {
			t.Fatalf("reclaimed hold still owns %d rows", n)
		}
		if _, h, _ := f.usage(t, f.sunbeds.ID, "2027-07-13", calendar.AM); h != 0 {
			t.Fatalf("held units still counted: %d", h)
		}
		// Even a confirmer that believes the hold is valid (no expiry on the
		// booking row, as when its clock reading precedes the expiry) finds
		// nothing to confirm and fails instead of confirming an empty booking.
		_, err = db.NewUpdate().TableExpr("bookings").Set("hold_expires_at = NULL").Where("id = ?", hold.id).Exec(ctx)
		must(t, err)
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.Confirm(ctx, tx, f.es, hold.id) })
		if !errors.Is(err, availability.ErrHoldExpired) {
			t.Fatalf("confirm after reclaim: %v", err)
		}
		// Confirm is idempotent on a booking already confirmed (no hold rows).
		_, err = db.NewUpdate().TableExpr("bookings").Set("status = 'confirmed'").Where("id = ?", hold.id).Exec(ctx)
		must(t, err)
		if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.Confirm(ctx, tx, f.es, hold.id) }); err != nil {
			t.Fatalf("confirm of a confirmed booking: %v", err)
		}
		f.checkInvariants(t)
	})
}

func TestConvertHold_ToBooking(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-14"), EndsOn: d("2027-07-15"), Slots: calendar.SlotsFull,
			Items: []store.HoldItem{{SpotID: f.spots["A3"].ID}, {ZoneID: f.qtyZone.ID, Quantity: 2}},
		})
		must(t, err)
		if _, held, _ := f.usage(t, f.qtyPool.ID, "2027-07-14", calendar.AM); held != 2 {
			t.Fatalf("held = %d", held)
		}
		var bookingID string
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			tb := f.insertBooking(t, tx, bookingSpec{from: "2027-07-14", to: "2027-07-15", lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}, {zone: f.qtyZone, qty: 2}}})
			bookingID = tb.id
			_, err := f.svc.ConvertHold(ctx, tx, f.es, inventory.ConvertHoldRequest{
				HoldID: h.ID, BookingID: tb.id, Kind: inventory.KindBooking,
				Lines: []inventory.LineRequest{{LineID: tb.lineIDs[0], SpotID: f.spots["A3"].ID}, {LineID: tb.lineIDs[1], ZoneID: f.qtyZone.ID, Quantity: 2}},
			})
			return err
		}))
		if n := f.count(t, "spot_allocations", "hold_id = ?", h.ID); n != 0 {
			t.Fatalf("hold rows left: %d", n)
		}
		if n := f.count(t, "spot_allocations", "booking_id = ? AND kind = 'booking'", bookingID); n != 4 {
			t.Fatalf("booking rows = %d", n)
		}
		if u, held, _ := f.usage(t, f.qtyPool.ID, "2027-07-15", calendar.PM); u != 2 || held != 0 {
			t.Fatalf("pool used %d held %d", u, held)
		}
		v, err := f.svc.GetHold(ctx, f.owner, f.est.ID, h.ID)
		must(t, err)
		if v.Status != inventory.HoldConverted || v.ConvertedBookingID == nil || *v.ConvertedBookingID != bookingID {
			t.Fatalf("hold = %+v", v)
		}
		if n := f.count(t, "outbox_events", "type = ?", "hold.converted"); n != 1 {
			t.Fatalf("hold.converted events = %d", n)
		}
		// converting again fails
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			_, err := f.svc.ConvertHold(ctx, tx, f.es, inventory.ConvertHoldRequest{HoldID: h.ID, BookingID: bookingID, Lines: []inventory.LineRequest{{LineID: "x", SpotID: f.spots["A3"].ID}}})
			return err
		})
		if !errors.Is(err, inventory.ErrHoldClosed) {
			t.Fatalf("second conversion: %v", err)
		}
		f.checkInvariants(t)
	})
}

// INV-10 (inventory part): a release removes the season rows of the
// half-days, resale re-occupies them, revoking fails once resold.
func TestSeasonRelease_RemovesAndRestoresAllocations(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		pass, err := f.book(t, bookingSpec{season: true, from: "2027-06-01", to: "2027-09-30", lines: []lineSpec{{zone: f.seat, spot: f.spots["A4"]}}}, nil)
		must(t, err)
		// 122 days x 2 slots - 1 closed PM.
		if n := f.count(t, "spot_allocations", "booking_id = ? AND kind = 'season'", pass.id); n != 243 {
			t.Fatalf("season rows = %d", n)
		}
		days := inventory.SeasonDays{
			BookingID: pass.id, BookingLineID: pass.lineIDs[0], SpotID: f.spots["A4"].ID,
			Days: []availability.DateSlot{{Date: d("2027-08-01"), Slot: calendar.AM}, {Date: d("2027-08-01"), Slot: calendar.PM}, {Date: d("2027-08-02"), Slot: calendar.AM}},
		}
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.RemoveSeasonDays(ctx, tx, f.es, days) }))
		if n := f.count(t, "spot_allocations", "booking_id = ?", pass.id); n != 240 {
			t.Fatalf("rows after release = %d", n)
		}
		// releasing again: the days are no longer occupied by the pass.
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.RemoveSeasonDays(ctx, tx, f.es, days) })
		if !errors.Is(err, inventory.ErrSeasonDaysMismatch) {
			t.Fatalf("double release: %v", err)
		}
		// resale of 2027-08-01 AM
		_, err = f.book(t, bookingSpec{from: "2027-08-01", to: "2027-08-01", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A4"]}}}, nil)
		must(t, err)
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.RestoreSeasonDays(ctx, tx, f.es, days) })
		ce := conflictOf(t, err, availability.ErrSpotUnavailable)
		if len(ce.Conflicts) != 1 || !ce.Conflicts[0].Date.Equal(d("2027-08-01")) || ce.Conflicts[0].Slot != calendar.AM {
			t.Fatalf("restore conflicts = %+v", ce.Conflicts)
		}
		// revoke only the unsold days
		days.Days = days.Days[1:]
		must(t, store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error { return f.svc.RestoreSeasonDays(ctx, tx, f.es, days) }))
		if n := f.count(t, "spot_allocations", "booking_id = ?", pass.id); n != 242 {
			t.Fatalf("rows after restore = %d", n)
		}
		f.checkInvariants(t)
	})
}

func TestAllocate_ValidationAndInactive(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, err := db.NewUpdate().TableExpr("spots").Set("active = ?", false).Where("id = ?", f.spots["A5"].ID).Exec(ctx)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-01", to: "2027-07-01", lines: []lineSpec{{zone: f.seat, spot: f.spots["A5"]}}}, nil)
		ce := conflictOf(t, err, availability.ErrSpotUnavailable)
		if ce.Conflicts[0].Reason != availability.ReasonInactive {
			t.Fatalf("conflicts = %+v", ce.Conflicts)
		}
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			_, err := f.svc.Allocate(ctx, tx, f.es, inventory.AllocateRequest{BookingID: "b", Channel: "desk", Kind: inventory.KindBooking})
			return err
		})
		if !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("validation: %v", err)
		}
	})
}
