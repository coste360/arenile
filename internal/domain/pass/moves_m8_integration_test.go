//go:build integration

package pass_test

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// Season passes moved, swapped, group-moved and put back (M8, spec 30.7,
// INV-10, INV-101): the half-days the holder released stay free and the
// release rows follow the pass, so a revocation restores the day where the
// pass is on that date.

type passMoves struct {
	e        *env
	bookings *booking.Service
	passes   *pass.Service
	mapID    string
}

func newPassMoves(t *testing.T, db *store.DB) *passMoves {
	t.Helper()
	e := newEnv(t, db)
	ctx := e.ctx
	e.estA.Settings = store.JSON(`{"pricing":{"half_day_factor":"0.5","rounding":"none","default_vat_rate_bp":2200}}`)
	if _, err := db.NewUpdate().Model(e.estA).Column("settings").WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	c := e.clock
	c.Set(time.Date(2027, 6, 5, 8, 0, 0, 0, time.UTC))
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	prices := pricingconfig.NewService(pricingconfig.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	if _, err := prices.CreateFixedPrice(ctx, e.owner, e.estA.ID, pricingconfig.FixedPriceInput{
		SeasonID: e.season.ID, PackID: e.pack.ID, Formula: pricing.FormulaSeason, PriceCategory: "row1", Amount: 90000,
	}); err != nil {
		t.Fatal(err)
	}
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	resale := pass.NewResale(pass.ResaleDeps{Clock: c, Credits: e.customers})
	bookings := booking.NewService(booking.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: prices, Customers: e.customers,
		Payments: noPayments{}, Releases: resale,
	})
	passes := pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: bookings, Inventory: inv, Credits: e.customers})
	m := new(store.Map)
	if err := db.NewSelect().Model(m).Where("establishment_id = ?", e.estA.ID).Limit(1).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	return &passMoves{e: e, bookings: bookings, passes: passes, mapID: m.ID}
}

func (pm *passMoves) scope(t *testing.T) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(pm.e.ctx, pm.e.db, pm.e.estA.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func (pm *passMoves) spot(t *testing.T, label string) *store.Spot {
	t.Helper()
	return pm.e.newSpot(t, pm.e.estA.ID, pm.mapID, pm.e.zone.ID, label, true)
}

func (pm *passMoves) pass(t *testing.T, spotID string) *pass.Detail {
	t.Helper()
	dt, err := pm.passes.Create(pm.e.ctx, pm.e.cashier, "", pm.e.estA.ID, pass.CreateInput{CustomerID: pm.e.holder.ID, SeasonID: pm.e.season.ID, SpotID: spotID, PackID: pm.e.pack.ID})
	if err != nil {
		t.Fatal(err)
	}
	return dt
}

func (pm *passMoves) release(t *testing.T, passID string, days ...string) []store.SeasonRelease {
	t.Helper()
	var dates []calendar.Date
	for _, x := range days {
		dates = append(dates, d(x))
	}
	rel, err := pm.passes.Release(pm.e.ctx, pm.e.staff, "", pm.e.estA.ID, passID, pass.ReleaseInput{Dates: dates})
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

// spotOn is the spot of the season allocation of lineID on day (AM), "" if none.
func (pm *passMoves) spotOn(t *testing.T, lineID, day string) string {
	t.Helper()
	rows, err := store.SeasonAllocationsOfLine(pm.e.ctx, pm.e.db, pm.scope(t), lineID, d(day), d(day))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Slot == calendar.AM {
			return r.SpotID
		}
	}
	return ""
}

// releaseSpot is the spot recorded on the active release of lineID on day (AM).
func (pm *passMoves) releaseSpot(t *testing.T, lineID, day string) string {
	t.Helper()
	rels, err := store.ActiveSeasonReleases(pm.e.ctx, pm.e.db, pm.scope(t), lineID, []calendar.Date{d(day)})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rels {
		if r.Slot == calendar.AM {
			return r.SpotID
		}
	}
	return ""
}

// TestSwap_SeasonPassKeepsReleases: a pass that released 10 June is swapped
// from 8 June with a second pass; the released day stays free on both
// spots, the release follows the pass and its revocation restores the day
// on the new spot.
func TestSwap_SeasonPassKeepsReleases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		pm := newPassMoves(t, db)
		e := pm.e
		a := pm.pass(t, e.spot.ID)
		r1 := pm.spot(t, "R1")
		b := pm.pass(t, r1.ID)
		rel := pm.release(t, a.Pass.Booking.ID, "2027-06-10")
		_, err := pm.bookings.Swap(e.ctx, e.cashier, e.estA.ID, booking.SwapInput{
			A: booking.SwapSide{BookingID: a.Pass.Booking.ID, LineID: a.Pass.LineID},
			B: booking.SwapSide{BookingID: b.Pass.Booking.ID, LineID: b.Pass.LineID}, From: d("2027-06-08"),
		})
		if err != nil {
			t.Fatalf("swap: %v", err)
		}
		e.assertINV10(t)
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-09"); got != r1.ID {
			t.Fatalf("pass A on 9 June: %s, want R1", got)
		}
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-10"); got != "" {
			t.Fatalf("pass A got its released day back on %s", got)
		}
		if got := pm.spotOn(t, b.Pass.LineID, "2027-06-10"); got != e.spot.ID {
			t.Fatalf("pass B on 10 June: %s, want A1", got)
		}
		if got := pm.releaseSpot(t, a.Pass.LineID, "2027-06-10"); got != r1.ID {
			t.Fatalf("release spot %s, want R1 (follows the pass)", got)
		}
		// Revoking puts the day back where the pass is now.
		for _, r := range rel {
			if err := pm.passes.Revoke(e.ctx, e.staff, "", e.estA.ID, a.Pass.Booking.ID, r.ID); err != nil {
				t.Fatalf("revoke: %v", err)
			}
		}
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-10"); got != r1.ID {
			t.Fatalf("revoked day on %s, want R1", got)
		}
		e.assertINV10(t)
	})
}

// TestGroupMove_SeasonPassKeepsReleases: a group move of a pass with a
// released day from 8 June onto a free spot keeps the day free.
func TestGroupMove_SeasonPassKeepsReleases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		pm := newPassMoves(t, db)
		e := pm.e
		a := pm.pass(t, e.spot.ID)
		r2 := pm.spot(t, "R2")
		pm.release(t, a.Pass.Booking.ID, "2027-06-10")
		from := d("2027-06-08")
		res, err := pm.bookings.GroupMove(e.ctx, e.cashier, e.estA.ID, booking.GroupMoveInput{
			Moves: []booking.GroupMoveItem{{BookingID: a.Pass.Booking.ID, LineID: a.Pass.LineID, SpotID: r2.ID}}, From: &from,
		})
		if err != nil || !res.Applied {
			t.Fatalf("group move: %v", err)
		}
		e.assertINV10(t)
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-10"); got != "" {
			t.Fatalf("released day re-allocated on %s", got)
		}
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-11"); got != r2.ID {
			t.Fatalf("11 June on %s, want R2", got)
		}
		if got := pm.releaseSpot(t, a.Pass.LineID, "2027-06-10"); got != r2.ID {
			t.Fatalf("release spot %s, want R2", got)
		}
		// Moving the pass with Move from a date behaves the same.
		r3 := pm.spot(t, "R3")
		from2 := d("2027-06-09")
		if _, err := pm.bookings.Move(e.ctx, e.cashier, e.estA.ID, a.Pass.Booking.ID, booking.MoveInput{LineID: a.Pass.LineID, SpotID: r3.ID, From: &from2}); err != nil {
			t.Fatalf("move: %v", err)
		}
		e.assertINV10(t)
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-10"); got != "" {
			t.Fatalf("released day re-allocated by move on %s", got)
		}
		if got := pm.releaseSpot(t, a.Pass.LineID, "2027-06-10"); got != r3.ID {
			t.Fatalf("release spot after move %s, want R3", got)
		}
	})
}

// TestUndoMove_SeasonPassKeepsReleases: a swap of two passes undone within
// the window puts the passes back without re-allocating the released day,
// and the release returns to the original spot.
func TestUndoMove_SeasonPassKeepsReleases(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		pm := newPassMoves(t, db)
		e := pm.e
		a := pm.pass(t, e.spot.ID)
		r1 := pm.spot(t, "R1")
		b := pm.pass(t, r1.ID)
		pm.release(t, a.Pass.Booking.ID, "2027-06-10")
		sw, err := pm.bookings.Swap(e.ctx, e.cashier, e.estA.ID, booking.SwapInput{
			A: booking.SwapSide{BookingID: a.Pass.Booking.ID, LineID: a.Pass.LineID},
			B: booking.SwapSide{BookingID: b.Pass.Booking.ID, LineID: b.Pass.LineID}, From: d("2027-06-08"),
		})
		if err != nil || sw.Undo == nil {
			t.Fatalf("swap: %v %+v", err, sw)
		}
		if _, err := pm.bookings.UndoMove(e.ctx, e.cashier, e.estA.ID, sw.Undo.MoveID); err != nil {
			t.Fatalf("undo: %v", err)
		}
		e.assertINV10(t)
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-10"); got != "" {
			t.Fatalf("released day re-allocated by undo on %s", got)
		}
		if got := pm.spotOn(t, a.Pass.LineID, "2027-06-12"); got != e.spot.ID {
			t.Fatalf("12 June on %s after undo, want A1", got)
		}
		if got := pm.releaseSpot(t, a.Pass.LineID, "2027-06-10"); got != e.spot.ID {
			t.Fatalf("release spot after undo %s, want A1", got)
		}
	})
}

// TestRelease_RangeOnMovedPass (spec 30.8): a pass moved from 15 June
// releases days on both its spots in one call; each release carries the
// spot of its day and the revocation restores each day on its own spot.
func TestRelease_RangeOnMovedPass(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		pm := newPassMoves(t, db)
		e := pm.e
		a := pm.pass(t, e.spot.ID)
		r4 := pm.spot(t, "R4")
		from := d("2027-06-15")
		if _, err := pm.bookings.Move(e.ctx, e.cashier, e.estA.ID, a.Pass.Booking.ID, booking.MoveInput{LineID: a.Pass.LineID, SpotID: r4.ID, From: &from}); err != nil {
			t.Fatalf("move: %v", err)
		}
		rel := pm.release(t, a.Pass.Booking.ID, "2027-06-12", "2027-06-20")
		e.assertINV10(t)
		spots := map[string]string{}
		for _, r := range rel {
			spots[r.Date.String()] = r.SpotID
		}
		if spots["2027-06-12"] != e.spot.ID || spots["2027-06-20"] != r4.ID {
			t.Fatalf("release spots %v, want 12 June on A1 and 20 June on R4", spots)
		}
		for _, r := range rel {
			if err := pm.passes.Revoke(e.ctx, e.staff, "", e.estA.ID, a.Pass.Booking.ID, r.ID); err != nil {
				t.Fatalf("revoke %s: %v", r.Date, err)
			}
		}
		if pm.spotOn(t, a.Pass.LineID, "2027-06-12") != e.spot.ID || pm.spotOn(t, a.Pass.LineID, "2027-06-20") != r4.ID {
			t.Fatal("revoked days not restored on their own spots")
		}
		e.assertINV10(t)
	})
}
