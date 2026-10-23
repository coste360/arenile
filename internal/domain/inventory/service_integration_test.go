//go:build integration

package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// INV-27 / INV-26: permissions are checked in the service and every query
// is scoped by the establishment of the principal.
func TestInventoryService_PermissionsAndTenancy(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		in := inventory.HoldInput{StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsAM, Items: []store.HoldItem{{SpotID: f.spots["A1"].ID}}}
		if _, err := f.svc.CreateHold(ctx, f.viewer, f.est.ID, in); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer hold: %v", err)
		}
		if _, err := f.svc.CreateHold(ctx, f.ownerB, f.est.ID, in); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant hold: %v", err)
		}
		if _, err := f.svc.CreateHold(ctx, nil, f.est.ID, in); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, in)
		must(t, err)
		if _, err := f.svc.GetHold(ctx, f.ownerB, f.estB.ID, h.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("hold of another establishment: %v", err)
		}
		blk := inventory.SpotBlockInput{SpotIDs: []string{f.spots["A2"].ID}, StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsFull}
		if _, err := f.svc.CreateSpotBlocks(ctx, f.viewer, f.est.ID, blk); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer block: %v", err)
		}
		if _, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, inventory.AvailabilityQuery{From: d("2027-07-02"), To: d("2027-07-02")}); err != nil {
			t.Fatalf("viewer reads availability: %v", err)
		}
		if _, err := f.svc.GetPlanning(ctx, f.ownerB, f.est.ID, inventory.PlanningQuery{From: d("2027-07-02"), To: d("2027-07-02")}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant planning: %v", err)
		}
		// the other establishment sees nothing of A
		res, err := f.svc.GetAvailability(ctx, f.ownerB, f.estB.ID, inventory.AvailabilityQuery{From: d("2027-07-02"), To: d("2027-07-02")})
		must(t, err)
		if len(res.Zones) != 0 || len(res.Pools) != 0 {
			t.Fatalf("leak: %+v", res)
		}
	})
}

func TestHolds_LifecycleAndSweeper(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		in := inventory.HoldInput{
			StartsOn: d("2027-07-03"), EndsOn: d("2027-07-04"), Slots: calendar.SlotsFull, TTL: time.Minute, Note: " cliente al telefono ",
			Items: []store.HoldItem{{SpotID: f.spots["A1"].ID}, {ZoneID: f.zoneMode.ID}},
		}
		h, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, in)
		must(t, err)
		if h.Status != inventory.HoldActive || h.Note != "cliente al telefono" || !h.ExpiresAt.Equal(t0.Add(time.Minute)) {
			t.Fatalf("hold = %+v", h)
		}
		if n := f.count(t, "spot_allocations", "hold_id = ? AND kind = 'hold' AND expires_at IS NOT NULL", h.ID); n != 4 {
			t.Fatalf("hold rows = %d", n)
		}
		if n := f.count(t, "outbox_events", "type = 'hold.created'"); n != 1 {
			t.Fatalf("events = %d", n)
		}
		if n := f.count(t, "audit_log", "action = 'hold.created'"); n != 1 {
			t.Fatalf("audit = %d", n)
		}
		// conflicting hold
		_, err = f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{StartsOn: d("2027-07-04"), EndsOn: d("2027-07-04"), Slots: calendar.SlotsPM, Items: []store.HoldItem{{SpotID: f.spots["A1"].ID}}})
		conflictOf(t, err, availability.ErrSpotUnavailable)
		// release is idempotent
		must(t, f.svc.ReleaseHold(ctx, f.cashier, f.est.ID, h.ID, ""))
		must(t, f.svc.ReleaseHold(ctx, f.cashier, f.est.ID, h.ID, ""))
		if n := f.count(t, "spot_allocations", "hold_id = ?", h.ID); n != 0 {
			t.Fatalf("rows after release = %d", n)
		}
		if n := f.count(t, "outbox_events", "type = 'hold.released'"); n != 1 {
			t.Fatalf("release events = %d", n)
		}
		v, err := f.svc.GetHold(ctx, f.viewer, f.est.ID, h.ID)
		must(t, err)
		if v.Status != inventory.HoldReleased {
			t.Fatalf("status = %s", v.Status)
		}
		// sweeper: two holds expire, one stays
		h1, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"), Slots: calendar.SlotsAM, TTL: time.Minute,
			Items: []store.HoldItem{{PoolID: f.sunbeds.ID, Quantity: 4}},
		})
		must(t, err)
		_, err = f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"), Slots: calendar.SlotsAM, TTL: time.Minute,
			Items: []store.HoldItem{{SpotID: f.spots["A2"].ID}},
		})
		must(t, err)
		keep, err := f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{
			StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"), Slots: calendar.SlotsAM, TTL: time.Hour,
			Items: []store.HoldItem{{SpotID: f.spots["A3"].ID}},
		})
		must(t, err)
		f.clock.Advance(2 * time.Minute)
		n, err := f.svc.SweepExpiredHolds(ctx)
		must(t, err)
		if n != 2 {
			t.Fatalf("swept = %d", n)
		}
		n, err = f.svc.SweepExpiredHolds(ctx) // idempotent
		must(t, err)
		if n != 0 {
			t.Fatalf("second sweep = %d", n)
		}
		if _, held, _ := f.usage(t, f.sunbeds.ID, "2027-07-05", calendar.AM); held != 0 {
			t.Fatalf("held after sweep = %d", held)
		}
		v, err = f.svc.GetHold(ctx, f.viewer, f.est.ID, h1.ID)
		must(t, err)
		if v.Status != inventory.HoldExpired { // closed by the sweeper, not by an operator
			t.Fatalf("swept status = %s", v.Status)
		}
		if n := f.count(t, "spot_allocations", "hold_id = ?", keep.ID); n != 1 {
			t.Fatalf("active hold touched: %d", n)
		}
		if len(f.svc.Jobs()) != 1 {
			t.Fatal("sweeper job not registered")
		}
		f.checkInvariants(t)
		// validation
		_, err = f.svc.CreateHold(ctx, f.cashier, f.est.ID, inventory.HoldInput{StartsOn: d("2027-07-05"), EndsOn: d("2027-07-04"), Slots: "X"})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) < 3 {
			t.Fatalf("validation: %v", err)
		}
	})
}

func TestSpotBlocks_CreateListDelete(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{from: "2027-07-06", to: "2027-07-06", slots: calendar.SlotsPM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		in := inventory.SpotBlockInput{SpotIDs: []string{f.spots["A1"].ID, f.spots["A2"].ID}, StartsOn: d("2027-07-06"), EndsOn: d("2027-07-07"), Slots: calendar.SlotsFull, Reason: "manutenzione"}
		_, err = f.svc.CreateSpotBlocks(ctx, f.cashier, f.est.ID, in)
		ce := conflictOf(t, err, availability.ErrSpotUnavailable)
		if len(ce.Conflicts) != 1 || ce.Conflicts[0].SpotID != f.spots["A2"].ID {
			t.Fatalf("conflicts = %+v", ce.Conflicts)
		}
		if n := f.count(t, "spot_blocks", "1 = 1"); n != 0 {
			t.Fatal("partial block written")
		}
		in.SpotIDs = []string{f.spots["A1"].ID, f.spots["A3"].ID}
		blocks, err := f.svc.CreateSpotBlocks(ctx, f.cashier, f.est.ID, in)
		must(t, err)
		if len(blocks) != 2 || f.count(t, "spot_allocations", "kind = 'block'") != 8 {
			t.Fatalf("blocks = %+v", blocks)
		}
		page, err := f.svc.ListSpotBlocks(ctx, f.viewer, f.est.ID, store.SpotBlockFilter{From: d("2027-07-07"), SpotID: f.spots["A3"].ID}, "", 10)
		must(t, err)
		if len(page.Items) != 1 || page.Items[0].Reason != "manutenzione" {
			t.Fatalf("list = %+v", page)
		}
		page, err = f.svc.ListSpotBlocks(ctx, f.viewer, f.est.ID, store.SpotBlockFilter{To: d("2027-07-05")}, "", 10)
		must(t, err)
		if len(page.Items) != 0 {
			t.Fatalf("list before = %+v", page)
		}
		must(t, f.svc.DeleteSpotBlock(ctx, f.cashier, f.est.ID, blocks[0].ID, ""))
		if err := f.svc.DeleteSpotBlock(ctx, f.cashier, f.est.ID, blocks[0].ID, ""); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("delete twice: %v", err)
		}
		if f.count(t, "spot_allocations", "kind = 'block'") != 4 || f.count(t, "outbox_events", "type LIKE 'spot_block.%'") != 3 {
			t.Fatal("after delete")
		}
	})
}

// INV-04: a spot is unavailable iff a valid allocation exists.
func TestAvailabilityQuery_ReflectsAllocations(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{from: "2027-07-21", to: "2027-07-21", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, map[string]int{"sunbed": 3})
		must(t, err)
		exp := f.clock.Now().Add(time.Minute)
		_, err = f.book(t, bookingSpec{status: "held", expires: &exp, from: "2027-07-20", to: "2027-07-21", lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-20", to: "2027-07-21", lines: []lineSpec{{zone: f.zoneMode}}}, nil)
		must(t, err)
		q := inventory.AvailabilityQuery{From: d("2027-07-20"), To: d("2027-07-21")}
		res, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, q)
		must(t, err)
		if len(res.Zones) != 3 || len(res.Pools) != 3 {
			t.Fatalf("result = %+v", res)
		}
		seat := res.Zones[0]
		state := map[string]bool{}
		for _, sp := range seat.Spots {
			state[sp.Label] = sp.Available
		}
		if state["A1"] || state["A2"] || !state["A3"] || seat.Available != 3 || seat.Capacity != 5 {
			t.Fatalf("seat zone = %+v", seat)
		}
		zone := res.Zones[1]
		if zone.Mode != "zone" || zone.Available != 2 || zone.Capacity != 3 || len(zone.Spots) != 3 {
			t.Fatalf("zone-mode = %+v", zone)
		}
		qty := res.Zones[2]
		if qty.Spots != nil || qty.Available != 4 {
			t.Fatalf("quantity = %+v", qty)
		}
		// closure PM of 07-20 is not a half-day of the query
		for _, pa := range res.Pools {
			if pa.PoolID == f.sunbeds.ID && (pa.Available != 7 || len(pa.Days) != 3) {
				t.Fatalf("sunbeds = %+v", pa)
			}
		}
		// the hold expires: A2 becomes available again (INV-04/05)
		f.clock.Advance(2 * time.Minute)
		res, err = f.svc.GetAvailability(ctx, f.viewer, f.est.ID, q)
		must(t, err)
		for _, sp := range res.Zones[0].Spots {
			if sp.Label == "A2" && !sp.Available {
				t.Fatalf("expired hold still blocks: %+v", sp)
			}
		}
		// pack filter: requires umbrella spots, no zone restriction; out of
		// season dates are closed.
		res, err = f.svc.GetAvailability(ctx, f.viewer, f.est.ID, inventory.AvailabilityQuery{From: d("2027-10-01"), To: d("2027-10-02"), PackID: f.pack.ID, Channel: store.ChannelOnline})
		must(t, err)
		if len(res.ClosedDates) != 2 || res.Zones[0].Available != 0 {
			t.Fatalf("out of season = %+v", res)
		}
		if _, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, inventory.AvailabilityQuery{From: d("2027-07-02"), To: d("2027-07-01")}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("reversed range: %v", err)
		}
	})
}

func TestOperationsDay_StatesAndTotals(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, err := f.book(t, bookingSpec{status: "checked_in", from: "2027-07-02", to: "2027-07-03", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-02", slots: calendar.SlotsPM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-02", lines: []lineSpec{{zone: f.zoneMode}}}, nil)
		must(t, err)
		pass, err := f.book(t, bookingSpec{season: true, from: "2027-06-01", to: "2027-09-30", lines: []lineSpec{{zone: f.seat, spot: f.spots["A4"]}}}, nil)
		must(t, err)
		_, err = f.svc.CreateSpotBlocks(ctx, f.cashier, f.est.ID, inventory.SpotBlockInput{SpotIDs: []string{f.spots["A3"].ID}, StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: calendar.SlotsFull, Reason: "rotto"})
		must(t, err)
		// A4: the pass released 07-02 AM
		must(t, f.releaseDay(t, pass, f.spots["A4"], "2027-07-02", calendar.AM))
		day, err := f.svc.GetOperationsDay(ctx, f.viewer, f.est.ID, inventory.OperationsQuery{Date: d("2027-07-02")})
		must(t, err)
		by := map[string]inventory.OperationalSpot{}
		for _, sp := range day.Spots {
			by[sp.Label] = sp
		}
		if by["A1"].Status != inventory.SpotCheckedIn || by["A1"].Booking == nil || by["A1"].Booking.CustomerDisplayName != "Rossi M." || by["A1"].Booking.BalanceDue != 4000 {
			t.Fatalf("A1 = %+v", by["A1"])
		}
		if by["A2"].AMStatus != inventory.SpotFree || by["A2"].PMStatus != inventory.SpotBooked || by["A2"].Status != inventory.SpotBooked {
			t.Fatalf("A2 = %+v", by["A2"])
		}
		if by["A3"].Status != inventory.SpotBlocked || by["A3"].BlockReason != "rotto" {
			t.Fatalf("A3 = %+v", by["A3"])
		}
		if by["A4"].AMStatus != inventory.SpotReleased || by["A4"].PMStatus != inventory.SpotPassHolder || by["A4"].ReleaseID == "" {
			t.Fatalf("A4 = %+v", by["A4"])
		}
		if len(day.PendingAssignments) != 1 || day.PendingAssignments[0].CustomerDisplayName != "Rossi M." {
			t.Fatalf("pending = %+v", day.PendingAssignments)
		}
		tot := day.Totals
		if tot.Spots != 8 || tot.Arrivals != 3 || tot.Departures != 2 || tot.CheckedIn != 1 || tot.PendingAssignments != 1 || tot.Occupied != 4 {
			t.Fatalf("totals = %+v", tot)
		}
		if day.Closed {
			t.Fatal("not a closed day")
		}
		closed, err := f.svc.GetOperationsDay(ctx, f.viewer, f.est.ID, inventory.OperationsQuery{Date: d("2027-07-20"), Slot: calendar.SlotsPM, MapID: f.mapID})
		must(t, err)
		if !closed.Closed || closed.Spots[0].Status != inventory.SpotClosed && closed.Spots[0].Status != inventory.SpotPassHolder {
			t.Fatalf("closed day = %+v", closed.Spots[0])
		}
	})
}

// releaseDay records a season release the way the pass service does:
// remove the allocation, insert the season_releases row.
func (f *fixture) releaseDay(t *testing.T, pass testBooking, spot *store.Spot, date string, slot calendar.Slot) error {
	t.Helper()
	ctx := context.Background()
	return store.RunInTx(ctx, f.db, func(ctx context.Context, tx bun.Tx) error {
		if err := f.svc.RemoveSeasonDays(ctx, tx, f.es, inventory.SeasonDays{
			BookingID: pass.id, BookingLineID: pass.lineIDs[0], SpotID: spot.ID,
			Days: []availability.DateSlot{{Date: d(date), Slot: slot}},
		}); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(&store.SeasonRelease{
			ID: store.NewID(), EstablishmentID: f.est.ID, BookingID: pass.id, BookingLineID: pass.lineIDs[0],
			SpotID: spot.ID, Date: d(date), Slot: slot, CreatedAt: t0, UpdatedAt: t0,
		}).Exec(ctx)
		return err
	})
}

func TestPlanning_Grid(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		tb, err := f.book(t, bookingSpec{from: "2027-07-02", to: "2027-07-04", lines: []lineSpec{{zone: f.seat, spot: f.spots["A1"]}, {zone: f.seat, spot: f.spots["A2"]}}}, nil)
		must(t, err)
		_, err = f.book(t, bookingSpec{from: "2027-07-03", to: "2027-07-03", slots: calendar.SlotsAM, lines: []lineSpec{{zone: f.seat, spot: f.spots["A3"]}}}, nil)
		must(t, err)
		_, err = f.svc.CreateSpotBlocks(ctx, f.cashier, f.est.ID, inventory.SpotBlockInput{SpotIDs: []string{f.spots["A3"].ID}, StartsOn: d("2027-07-03"), EndsOn: d("2027-07-03"), Slots: calendar.SlotsPM})
		must(t, err)
		pl, err := f.svc.GetPlanning(ctx, f.viewer, f.est.ID, inventory.PlanningQuery{From: d("2027-07-01"), To: d("2027-07-05"), ZoneID: f.seat.ID})
		must(t, err)
		if len(pl.Days) != 5 || len(pl.Spots) != 5 || len(pl.Rows) != 3 || len(pl.Entries) != 3 {
			t.Fatalf("planning = %+v", pl)
		}
		first := pl.Rows[0]
		if pl.Spots[first.Spot].Label != "A1" || len(first.Segments) != 1 || first.Segments[0].From != 1 || first.Segments[0].To != 3 || first.Segments[0].Slots != calendar.SlotsFull {
			t.Fatalf("A1 row = %+v", first)
		}
		e := pl.Entries[first.Segments[0].Entry]
		if e.Kind != inventory.EntryBooking || e.BookingID != tb.id || e.Label != "Rossi M." || e.BalanceDue == nil || !*e.BalanceDue {
			t.Fatalf("entry = %+v", e)
		}
		a3 := pl.Rows[2]
		if len(a3.Segments) != 2 || a3.Segments[0].Slots != calendar.SlotsAM || pl.Entries[a3.Segments[1].Entry].Kind != inventory.EntryBlock {
			t.Fatalf("A3 row = %+v", a3)
		}
		if _, err := f.svc.GetPlanning(ctx, f.viewer, f.est.ID, inventory.PlanningQuery{From: d("2027-06-01"), To: d("2027-08-31")}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("range too long: %v", err)
		}
		pl, err = f.svc.GetPlanning(ctx, f.viewer, f.est.ID, inventory.PlanningQuery{From: d("2027-09-29"), To: d("2027-10-02")})
		must(t, err)
		if len(pl.ClosedDays) != 2 || pl.ClosedDays[0] != 2 {
			t.Fatalf("closed days = %v", pl.ClosedDays)
		}
	})
}
