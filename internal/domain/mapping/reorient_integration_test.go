//go:build integration

package mapping

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// block inserts a manual block of spotID on day with its allocation rows.
func (e *env) block(spotID string, day calendar.Date) {
	e.t.Helper()
	now := e.svc.now()
	b := &store.SpotBlock{ID: store.NewID(), EstablishmentID: e.estA.ID, SpotID: spotID, StartsOn: day, EndsOn: day, Slots: calendar.SlotsFull, Reason: "manutenzione", CreatedAt: now, UpdatedAt: now}
	if _, err := e.db.NewInsert().Model(b).Exec(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
		a := &store.SpotAllocation{ID: store.NewID(), EstablishmentID: e.estA.ID, SpotID: spotID, Date: day, Slot: slot, Kind: store.AllocationBlock, BlockID: &b.ID, CreatedAt: now}
		if _, err := e.db.NewInsert().Model(a).Exec(e.ctx); err != nil {
			e.t.Fatal(err)
		}
	}
}

func TestMappingIntegration_ReorientMap(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID) // 1000 x 600, sea bottom
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)
		in := docInput(doc)
		a1, a2, b1 := spot(z.Zone.ID, "A1", 100), spot(z.Zone.ID, "A2", 160), spot(z.Zone.ID, "B1", 100)
		a1.Y, a2.Y, b1.Y = 520, 520, 450 // row A nearest to the sea (bottom)
		in.Spots = []SpotInput{a1, a2, b1}
		in.Decorations = []DecorationInput{
			{Kind: "walkway", Asset: tileStraightV, X: 30, Y: 300, Shape: store.Shape{Type: ShapeRect, W: 60, H: 600}},
			{Kind: "bar", Asset: "bar", X: 900, Y: 60, Rotation: 30, Shape: store.Shape{Type: ShapeRect, W: 84, H: 84}},
		}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		audits := func() int {
			return e.count("audit_log", "action = 'map.reoriented' AND establishment_id = ?", e.estA.ID)
		}

		// Dry run: preview and counts, nothing written, no version check.
		prev, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 0, ReorientInput{SeaSide: SeaTop, DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		if prev.Applied || prev.Transform != TransformMirrorY || prev.Document.Map.Version != 2 || prev.Document.Map.SeaSide != SeaTop {
			t.Fatalf("dry run: %+v", prev)
		}
		if got := spotByLabel(prev.Document, "A1"); got.Y != 80 {
			t.Fatalf("dry run A1 y = %v", got.Y)
		}
		if cur, _ := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID); cur.Map.Version != 2 || cur.Map.SeaSide != SeaBottom {
			t.Fatalf("dry run wrote: %+v", cur.Map)
		}

		// Stale If-Match.
		_, err = e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, ReorientInput{SeaSide: SeaTop})
		var vc *configerr.VersionConflictError
		if !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale reorient: %v", err)
		}
		// Permission: the cashier cannot configure.
		if _, err := e.svc.ReorientMap(e.ctx, e.cashierA, e.estA.ID, doc.Map.ID, 2, ReorientInput{SeaSide: SeaTop}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		// Another tenant does not see the map.
		if _, err := e.svc.ReorientMap(e.ctx, e.ownerB, e.estA.ID, doc.Map.ID, 2, ReorientInput{SeaSide: SeaTop}); err == nil {
			t.Fatal("foreign owner reoriented the map")
		}

		// Future allocations (a block today, 16 June in Rome) need the
		// acknowledgement; past ones are not counted.
		today := calendar.MustParse("2027-06-16")
		spotA1 := spotByLabel(v2, "A1")
		e.block(spotA1.ID, today)
		e.block(spotByLabel(v2, "B1").ID, today.AddDays(-1))
		_, err = e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, ReorientInput{SeaSide: SeaTop})
		var sa *configerr.SpotsAllocatedError
		if !errors.As(err, &sa) || len(sa.SpotIDs) != 1 || sa.SpotIDs[0] != spotA1.ID {
			t.Fatalf("unacknowledged allocations: %v", err)
		}
		if audits() != 0 {
			t.Fatal("refused reorientation was audited")
		}
		prev, err = e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, ReorientInput{SeaSide: SeaTop, DryRun: true})
		if err != nil || prev.FutureAllocations != 1 || prev.SpotsWithFutureAllocations != 1 {
			t.Fatalf("dry run counts: %+v %v", prev, err)
		}

		// Acknowledged: one versioned save, geometry only.
		res, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, ReorientInput{SeaSide: SeaTop, AcknowledgeFutureAllocations: true})
		if err != nil {
			t.Fatal(err)
		}
		v3 := res.Document
		if !res.Applied || v3.Map.Version != 3 || v3.Map.SeaSide != SeaTop || v3.Map.Width != 1000 || v3.Map.Height != 600 {
			t.Fatalf("reoriented: applied=%v %+v", res.Applied, v3.Map)
		}
		for _, old := range v2.Spots {
			got := spotByLabel(v3, old.Label)
			if got == nil || got.ID != old.ID || got.ZoneID != old.ZoneID || got.X != old.X || got.Y != 600-old.Y {
				t.Fatalf("spot %s: %+v -> %+v", old.Label, old, got)
			}
		}
		if a, b := spotByLabel(v3, "A1"), spotByLabel(v3, "B1"); a.Y >= b.Y {
			t.Fatal("row A is not the nearest to the sea at the top")
		}
		for i, d := range v3.Decorations {
			if d.ID != v2.Decorations[i].ID {
				t.Fatal("decoration ids changed")
			}
		}
		if n := e.count("spot_allocations", "spot_id = ?", spotA1.ID); n != 2 {
			t.Fatalf("allocations of A1: %d", n)
		}
		if audits() != 1 {
			t.Fatalf("audit rows: %d", audits())
		}
		var events []store.OutboxEvent
		if err := e.db.NewSelect().Model(&events).Where("type = ?", event.ConfigMapChanged).Scan(e.ctx); err != nil {
			t.Fatal(err)
		}
		reoriented := 0
		for _, ev := range events {
			var payload event.ConfigChange
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Change == event.ChangeReoriented {
				reoriented++
				if payload.MapID != doc.Map.ID || payload.Version != 3 || payload.Entity != "map" {
					t.Fatalf("event payload: %+v", payload)
				}
			}
		}
		if reoriented != 1 {
			t.Fatalf("reoriented events: %d", reoriented)
		}

		// Same side: nothing saved.
		same, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 3, ReorientInput{SeaSide: SeaTop, AcknowledgeFutureAllocations: true})
		if err != nil || same.Applied || same.Transform != TransformNone || same.Document.Map.Version != 3 {
			t.Fatalf("same side: %+v %v", same, err)
		}

		// Adjacent side: width and height swap, back to the top restores it.
		left, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 3, ReorientInput{SeaSide: SeaLeft, AcknowledgeFutureAllocations: true})
		if err != nil {
			t.Fatal(err)
		}
		if left.Transform != TransformRotateCCW || left.Document.Map.Width != 600 || left.Document.Map.Height != 1000 {
			t.Fatalf("rotate: %s %+v", left.Transform, left.Document.Map)
		}
		back, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 4, ReorientInput{SeaSide: SeaTop, AcknowledgeFutureAllocations: true})
		if err != nil {
			t.Fatal(err)
		}
		for i, sp := range back.Document.Spots {
			if w := v3.Spots[i]; sp.X != w.X || sp.Y != w.Y || sp.Rotation != w.Rotation {
				t.Fatalf("round trip moved %s: %+v -> %+v", sp.Label, w, sp)
			}
		}
		for i, d := range back.Document.Decorations {
			if w := v3.Decorations[i]; d.X != w.X || d.Y != w.Y || d.Shape != w.Shape || deref(d.Asset) != deref(w.Asset) || d.Rotation != w.Rotation {
				t.Fatalf("round trip moved decoration %d: %+v -> %+v", i, w, d)
			}
		}

		// Invalid side.
		if _, err := e.svc.ReorientMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 5, ReorientInput{SeaSide: "north"}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("invalid side: %v", err)
		}
	})
}
