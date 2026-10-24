//go:build integration

package pass_test

import (
	"encoding/json"
	"testing"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestPriceOverride_DeskModifyCannotClear (INV-77): a manual price set by
// the owner survives the re-pricing edits of the desk (a move to a spot
// priced differently, a PATCH), which only need bookings.write. The engine
// total beneath it follows the new stay, the audit of the edit records
// price_override_kept, and "Torna al listino" afterwards restores the engine
// price of the stay as it is now, never a stale one (INV-12).
func TestPriceOverride_DeskModifyCannotClear(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx := e.ctx
		c := e.clock
		aw, ob := audit.NewWriter(c), event.NewOutbox(c)
		prices := pricingconfig.NewService(pricingconfig.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		// Two price categories: row1 (90000, the zone of the fixture) and
		// row2 (60000, a second zone with one spot).
		for cat, amount := range map[string]money.Cents{"row1": 90000, "row2": 60000} {
			if _, err := prices.CreateFixedPrice(ctx, e.owner, e.estA.ID, pricingconfig.FixedPriceInput{
				SeasonID: e.season.ID, PackID: e.pack.ID, Formula: pricing.FormulaSeason, PriceCategory: cat, Amount: amount,
			}); err != nil {
				t.Fatal(err)
			}
		}
		zone2 := &store.Zone{ID: store.NewID(), EstablishmentID: e.estA.ID, MapID: e.zone.MapID, Code: "row2", Name: store.I18nText{"it": "Seconda fila"}, AssignmentMode: "seat", PriceCategory: "row2", Version: 1, CreatedAt: t0, UpdatedAt: t0}
		e.insert(t, zone2)
		spot2 := e.newSpot(t, e.estA.ID, *e.zone.MapID, zone2.ID, "B1", true)

		inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		bookings := booking.NewService(booking.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: prices, Customers: e.customers,
			Payments: noPayments{}, Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: e.customers}),
		})
		passes := pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: bookings, Inventory: inv, Credits: e.customers})

		dt, err := passes.Create(ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: e.spot.ID, PackID: e.pack.ID})
		if err != nil {
			t.Fatalf("create pass: %v", err)
		}
		pid := dt.Pass.Booking.ID
		if dt.Pass.Booking.TotalAmount != 90000 {
			t.Fatalf("engine price %d", dt.Pass.Booking.TotalAmount)
		}
		v := dt.Pass.Booking.Version
		over, err := passes.SetPriceOverride(ctx, e.owner, "", e.estA.ID, pid, &v, 50000, "cliente storico")
		if err != nil {
			t.Fatalf("override: %v", err)
		}
		snapshot := string(over.Pass.Booking.PriceSnapshot)

		lines, err := store.ListBookingLines(ctx, db, e.esA, pid)
		if err != nil || len(lines) != 1 {
			t.Fatalf("lines %v %d", err, len(lines))
		}
		// The cashier moves the pass to the cheaper row: the manual price stays.
		ver := over.Pass.Booking.Version
		moved, err := bookings.Move(ctx, e.cashier, e.estA.ID, pid, booking.MoveInput{ExpectedVersion: &ver, LineID: lines[0].ID, SpotID: spot2.ID})
		if err != nil {
			t.Fatalf("move by the desk: %v", err)
		}
		b := moved.Detail.Booking
		if b.TotalAmount != 50000 || moved.NewTotal != 50000 || b.PriceOverrideAmount == nil || *b.PriceOverrideAmount != 50000 ||
			b.PriceOverrideReason == nil || *b.PriceOverrideReason != "cliente storico" {
			t.Fatalf("manual price lost by a desk move: total %d override %v", b.TotalAmount, b.PriceOverrideAmount)
		}
		if b.PriceOverrideEngineAmount == nil || *b.PriceOverrideEngineAmount != 60000 {
			t.Fatalf("engine total beneath the override %v, want 60000", b.PriceOverrideEngineAmount)
		}
		if string(b.PriceSnapshot) != snapshot {
			t.Fatal("price_snapshot changed on a confirmed pass (INV-21)")
		}
		var cur pricing.PriceBreakdown
		if err := json.Unmarshal(b.CurrentPrice(), &cur); err != nil || cur.Total != b.TotalAmount {
			t.Fatalf("INV-12: price_current total %d != total_amount %d (%v)", cur.Total, b.TotalAmount, err)
		}
		if n := e.count(t, "audit_log", "entity_id = ? AND action = ? AND CAST(diff AS TEXT) LIKE ?", pid, "booking.moved", "%price_override_kept%"); n != 1 {
			t.Fatalf("booking.moved audit with price_override_kept: %d", n)
		}
		if n := e.count(t, "audit_log", "entity_id = ? AND action = ?", pid, "season_pass.price_override_cleared"); n != 0 {
			t.Fatalf("override cleared by the desk: %d audit rows", n)
		}

		// A PATCH by the desk keeps it too.
		ver = b.Version
		note := "ombrellone vicino al bar"
		patched, err := bookings.Update(ctx, e.cashier, e.estA.ID, pid, &ver, booking.PatchInput{NotesInternal: &note})
		if err != nil {
			t.Fatalf("patch by the desk: %v", err)
		}
		if patched.Detail.Booking.TotalAmount != 50000 || patched.Detail.Booking.PriceOverrideAmount == nil {
			t.Fatalf("manual price lost by a PATCH: %+v", patched.Detail.Booking)
		}

		// Back to the price list restores the engine price of the new row.
		ver = patched.Detail.Booking.Version
		back, err := passes.ClearPriceOverride(ctx, e.owner, "", e.estA.ID, pid, &ver)
		if err != nil {
			t.Fatalf("clear: %v", err)
		}
		if back.Pass.Booking.TotalAmount != 60000 || back.Pass.Booking.PriceOverrideAmount != nil {
			t.Fatalf("cleared pass total %d override %v, want 60000", back.Pass.Booking.TotalAmount, back.Pass.Booking.PriceOverrideAmount)
		}
		if err := json.Unmarshal(back.Pass.Booking.CurrentPrice(), &cur); err != nil || cur.Total != 60000 {
			t.Fatalf("INV-12 after clear: %d (%v)", cur.Total, err)
		}
	})
}
