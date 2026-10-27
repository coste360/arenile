//go:build integration

package mapping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// busyChecker reports a fixed set of spots as allocated.
type busyChecker struct {
	mu   sync.Mutex
	busy map[string]bool
	from []calendar.Date
}

func (b *busyChecker) SpotsWithFutureAllocations(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, ids []string, from calendar.Date) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.from = append(b.from, from)
	var out []string
	for _, id := range ids {
		if b.busy[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

type env struct {
	t                  *testing.T
	ctx                context.Context
	db                 *store.DB
	svc                *Service
	alloc              *busyChecker
	estA, estB         *store.Establishment
	ownerA, cashierA   *auth.Principal
	ownerB, managerAll *auth.Principal
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 6, 15, 22, 30, 0, 0, time.UTC)) // 16 June in Rome
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	user := func(org string, est string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
	}
	alloc := &busyChecker{busy: map[string]bool{}}
	svc := NewService(Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c), Allocations: alloc})
	return &env{
		t: t, ctx: ctx, db: db, svc: svc, alloc: alloc, estA: estA, estB: estB,
		ownerA: user(orgA.ID, "", auth.RoleOwner), cashierA: user(orgA.ID, estA.ID, auth.RoleCashier),
		ownerB: user(orgB.ID, "", auth.RoleOwner), managerAll: user(orgA.ID, "", auth.RoleManager),
	}
}

func (e *env) newMap(p *auth.Principal, est string) *Document {
	e.t.Helper()
	doc, err := e.svc.CreateMap(e.ctx, p, est, MapInput{Name: store.I18nText{"it": "Spiaggia"}, Width: 1000, Height: 600, GridSize: 10, SeaSide: SeaBottom})
	if err != nil {
		e.t.Fatal(err)
	}
	return doc
}

func (e *env) newZone(p *auth.Principal, est, mapID, code, mode string) *ZoneView {
	e.t.Helper()
	in := ZoneInput{Code: code, Name: store.I18nText{"it": "Zona " + code}, AssignmentMode: mode, OnlineVisible: true, PriceCategory: code}
	if mapID != "" {
		in.MapID = &mapID
	}
	z, err := e.svc.CreateZone(e.ctx, p, est, in)
	if err != nil {
		e.t.Fatal(err)
	}
	return z
}

func spot(zoneID, label string, x float64) SpotInput {
	return SpotInput{
		ZoneID: zoneID, Label: label, RowLabel: label[:1], Kind: "umbrella", X: x, Y: 50,
		Shape: store.Shape{Type: ShapeCircle, W: 8, H: 8}, Capacity: map[string]int{"umbrella": 1, "sunbed": 2}, Active: true,
	}
}

func docInput(doc *Document) SaveInput {
	in := SaveInput{Map: mapInputOf(&doc.Map)}
	for _, sp := range doc.Spots {
		in.Spots = append(in.Spots, SpotInput{
			ID: sp.ID, ZoneID: sp.ZoneID, Label: sp.Label, RowLabel: sp.RowLabel, Number: sp.Number,
			Kind: sp.Kind, Asset: deref(sp.Asset), X: sp.X, Y: sp.Y, Rotation: sp.Rotation, Shape: sp.Shape, Capacity: sp.Capacity, MaxExtra: sp.MaxExtra,
			PriceCategoryOverride: sp.PriceCategoryOverride, Attributes: sp.Attributes, Active: sp.Active,
		})
	}
	for _, d := range doc.Decorations {
		in.Decorations = append(in.Decorations, DecorationInput{
			ID: d.ID, Kind: d.Kind, Asset: deref(d.Asset), Variant: deref(d.Variant), X: d.X, Y: d.Y, Rotation: d.Rotation, Shape: d.Shape, Label: d.Label,
		})
	}
	return in
}

func (e *env) count(table, where string, args ...any) int {
	e.t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	return int(n)
}

func (e *env) zonePool(zoneID string) *store.Pool {
	e.t.Helper()
	es, err := store.SystemEstablishmentScope(e.ctx, e.db, e.estA.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	pools, err := store.ListZonePools(e.ctx, e.db, es, []string{zoneID})
	if err != nil {
		e.t.Fatal(err)
	}
	for i := range pools {
		if pools[i].SystemManaged {
			return &pools[i]
		}
	}
	return nil
}

func spotByLabel(doc *Document, label string) *store.Spot {
	for i := range doc.Spots {
		if doc.Spots[i].Label == label {
			return &doc.Spots[i]
		}
	}
	return nil
}

func TestMappingIntegration_SaveDiffAndVersioning(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		if doc.Map.Version != 1 || len(doc.Spots) != 0 {
			t.Fatalf("new map: %+v", doc.Map)
		}
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)

		in := docInput(doc)
		in.Spots = []SpotInput{spot(z.Zone.ID, "A1", 10), spot(z.Zone.ID, "A2", 20), spot(z.Zone.ID, "A3", 30)}
		in.Decorations = []DecorationInput{{Kind: "shower", X: 5, Y: 5, Shape: store.Shape{Type: ShapeRect, W: 4, H: 4}, Label: store.I18nText{"it": "Doccia"}}}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		if v2.Map.Version != 2 || len(v2.Spots) != 3 || len(v2.Decorations) != 1 || v2.SpotCount != 3 {
			t.Fatalf("after first save: v%d %d spots %d decorations", v2.Map.Version, len(v2.Spots), len(v2.Decorations))
		}

		// Stale If-Match: 412 with the current version, nothing written.
		_, err = e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		var vc *configerr.VersionConflictError
		if !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale save: %v", err)
		}

		// Swap labels A1 <-> A2 (unique index), delete A3, add B1, move the
		// decoration, rename the map.
		a1, a2, a3 := spotByLabel(v2, "A1"), spotByLabel(v2, "A2"), spotByLabel(v2, "A3")
		in = docInput(v2)
		in.Map.Name = store.I18nText{"it": "Spiaggia grande", "en": "Big beach"}
		var spots []SpotInput
		for _, sp := range in.Spots {
			switch sp.ID {
			case a1.ID:
				sp.Label = "A2"
			case a2.ID:
				sp.Label = "A1"
			case a3.ID:
				continue
			}
			spots = append(spots, sp)
		}
		spots = append(spots, spot(z.Zone.ID, "B1", 40))
		in.Spots = spots
		in.Decorations[0].X = 50
		v3, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		if err != nil {
			t.Fatal(err)
		}
		if v3.Map.Version != 3 || len(v3.Spots) != 3 || v3.Map.Name["en"] != "Big beach" {
			t.Fatalf("after second save: %+v", v3.Map)
		}
		if got := spotByLabel(v3, "A2"); got == nil || got.ID != a1.ID {
			t.Fatal("label swap lost the spot identity")
		}
		if spotByLabel(v3, "A3") != nil || spotByLabel(v3, "B1") == nil {
			t.Fatal("diff not applied")
		}
		if v3.Decorations[0].ID != v2.Decorations[0].ID || v3.Decorations[0].X != 50 {
			t.Fatal("decoration not updated in place")
		}

		// Unchanged save still bumps the version.
		v4, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 3, docInput(v3))
		if err != nil || v4.Map.Version != 4 {
			t.Fatalf("noop save: %v", err)
		}
		// Empty document deletes everything.
		empty := docInput(v4)
		empty.Spots, empty.Decorations = nil, nil
		v5, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 4, empty)
		if err != nil || len(v5.Spots) != 0 || len(v5.Decorations) != 0 {
			t.Fatalf("empty save: %v", err)
		}

		if n := e.count("audit_log", "action IN ('map.created', 'map.saved') AND establishment_id = ?", e.estA.ID); n != 5 {
			t.Fatalf("audit entries: %d", n)
		}
		if n := e.count("outbox_events", "type = ?", event.ConfigMapChanged); n != 6 { // map created, zone created, 4 saves
			t.Fatalf("outbox events: %d", n)
		}
	})
}

// Kit illustrations (ADR 0012): asset and variant round-trip, a change of
// asset alone is a change, the new spot kinds pass the database CHECK.
func TestMappingIntegration_KitAssets(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)
		in := docInput(doc)
		a1 := spot(z.Zone.ID, "A1", 10)
		a1.Asset = "ombrellone_2_lettini"
		d1 := spot(z.Zone.ID, "D1", 20)
		d1.Kind, d1.Asset = "daybed", "daybed"
		h1 := spot(z.Zone.ID, "H1", 30)
		h1.Kind = "hammock"
		in.Spots = []SpotInput{a1, d1, h1}
		in.Decorations = []DecorationInput{
			{Kind: "custom", Asset: "bandiera_mare", Variant: "yellow", X: 5, Y: 5, Shape: store.Shape{Type: ShapeRect, W: 6, H: 6}},
			{Kind: "walkway", X: 9, Y: 9, Shape: store.Shape{Type: ShapeRect, W: 6, H: 6}},
		}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := spotByLabel(v2, "A1"); got.Asset == nil || *got.Asset != "ombrellone_2_lettini" {
			t.Fatalf("A1 asset: %v", got.Asset)
		}
		if got := spotByLabel(v2, "H1"); got.Asset != nil || got.Kind != "hammock" {
			t.Fatalf("H1: %v %s", got.Asset, got.Kind)
		}
		var flag *store.MapDecoration
		for i := range v2.Decorations {
			if v2.Decorations[i].Kind == "custom" {
				flag = &v2.Decorations[i]
			} else if v2.Decorations[i].Asset != nil || v2.Decorations[i].Variant != nil {
				t.Fatal("empty asset/variant must be stored as NULL")
			}
		}
		if flag == nil || deref(flag.Asset) != "bandiera_mare" || deref(flag.Variant) != "yellow" {
			t.Fatalf("flag: %+v", flag)
		}
		// Changing only the illustration and the variant updates in place.
		in = docInput(v2)
		for i := range in.Spots {
			if in.Spots[i].Label == "A1" {
				in.Spots[i].Asset = "ombrellone_3_lettini"
			}
		}
		for i := range in.Decorations {
			if in.Decorations[i].Asset == "bandiera_mare" {
				in.Decorations[i].Variant = "red"
			}
		}
		v3, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := spotByLabel(v3, "A1"); deref(got.Asset) != "ombrellone_3_lettini" || got.ID != spotByLabel(v2, "A1").ID {
			t.Fatalf("A1 after asset change: %+v", got)
		}
		for _, d := range v3.Decorations {
			if d.ID == flag.ID && deref(d.Variant) != "red" {
				t.Fatalf("variant not updated: %+v", d)
			}
		}
		// A non-bookable illustration on a spot is refused.
		in = docInput(v3)
		in.Spots[0].Asset = "bar"
		_, err = e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 3, in)
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("bar on a spot: %v", err)
		}
	})
}

func TestMappingIntegration_SaveChecksAgainstDatabase(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		m1 := e.newMap(e.ownerA, e.estA.ID)
		m2 := e.newMap(e.ownerA, e.estA.ID)
		z1 := e.newZone(e.ownerA, e.estA.ID, m1.Map.ID, "row1", ModeSeat)
		z2 := e.newZone(e.ownerA, e.estA.ID, m2.Map.ID, "row2", ModeSeat)
		zq := e.newZone(e.ownerA, e.estA.ID, "", "sunbeds", ModeQuantity)
		mB := e.newMap(e.ownerB, e.estB.ID)
		zB := e.newZone(e.ownerB, e.estB.ID, mB.Map.ID, "row1", ModeSeat)

		tests := []struct {
			name  string
			spots []SpotInput
			field string
		}{
			{"zone of another map", []SpotInput{spot(z2.Zone.ID, "A1", 1)}, "spots[0].zone_id"},
			{"quantity zone", []SpotInput{spot(zq.Zone.ID, "A1", 1)}, "spots[0].zone_id"},
			{"zone of another establishment", []SpotInput{spot(zB.Zone.ID, "A1", 1)}, "spots[0].zone_id"},
			{"unknown spot id", []SpotInput{func() SpotInput { s := spot(z1.Zone.ID, "A1", 1); s.ID = store.NewID(); return s }()}, "spots[0].id"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				in := docInput(m1)
				in.Spots = tt.spots
				_, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, m1.Map.ID, 1, in)
				var ve *configerr.ValidationError
				if !errors.As(err, &ve) || !hasField(ve, tt.field) {
					t.Fatalf("want %s, got %v", tt.field, err)
				}
			})
		}
		// Spot id of another map.
		in := docInput(m2)
		in.Spots = []SpotInput{spot(z2.Zone.ID, "A1", 1)}
		saved, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, m2.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		in = docInput(m1)
		s := spot(z1.Zone.ID, "A1", 1)
		s.ID = saved.Spots[0].ID
		in.Spots = []SpotInput{s}
		if _, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, m1.Map.ID, 1, in); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign spot id: %v", err)
		}
		// Map version untouched by the failed saves.
		got, err := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, m1.Map.ID)
		if err != nil || got.Map.Version != 1 {
			t.Fatalf("version after failed saves: %v %v", got, err)
		}
	})
}

func TestMappingIntegration_FutureAllocations(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeZone)
		in := docInput(doc)
		in.Spots = []SpotInput{spot(z.Zone.ID, "A1", 10), spot(z.Zone.ID, "A2", 20)}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		busy := spotByLabel(v2, "A1")
		e.alloc.busy[busy.ID] = true

		// Deleting the allocated spot is refused and lists it.
		in = docInput(v2)
		in.Spots = slices.DeleteFunc(in.Spots, func(s SpotInput) bool { return s.ID == busy.ID })
		_, err = e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		var sa *configerr.SpotsAllocatedError
		if !errors.As(err, &sa) || !slices.Equal(sa.SpotIDs, []string{busy.ID}) {
			t.Fatalf("delete allocated spot: %v", err)
		}
		if want := calendar.MustParse("2027-06-16"); !e.alloc.from[len(e.alloc.from)-1].Equal(want) {
			t.Fatalf("allocation check from %s, want %s (establishment timezone)", e.alloc.from[len(e.alloc.from)-1], want)
		}
		// Deleting the map is refused too.
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); !errors.Is(err, configerr.ErrSpotHasFutureAllocations) {
			t.Fatalf("delete map: %v", err)
		}

		// Deactivation is always allowed and shrinks the implicit pool.
		in = docInput(v2)
		for i := range in.Spots {
			if in.Spots[i].ID == busy.ID {
				in.Spots[i].Active = false
			}
		}
		v3, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		if err != nil || v3.Map.Version != 3 {
			t.Fatalf("deactivate: %v", err)
		}
		if pl := e.zonePool(z.Zone.ID); pl == nil || pl.Capacity != 1 {
			t.Fatalf("pool after deactivation: %+v", pl)
		}
	})
}

func TestMappingIntegration_ImplicitZonePool(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "front", ModeZone)
		seat := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "back", ModeSeat)
		pl := e.zonePool(z.Zone.ID)
		if pl == nil || pl.Capacity != 0 || pl.ComponentCode != SystemPoolComponentCode || z.PoolID == nil || *z.PoolID != pl.ID {
			t.Fatalf("pool of a new zone zone: %+v view %+v", pl, z)
		}
		if e.zonePool(seat.Zone.ID) != nil {
			t.Fatal("seat zone got a pool")
		}

		in := docInput(doc)
		for i := 1; i <= 6; i++ {
			s := spot(z.Zone.ID, fmt.Sprintf("A%d", i), float64(i*10))
			s.Active = i != 6
			in.Spots = append(in.Spots, s)
		}
		in.Spots = append(in.Spots, spot(seat.Zone.ID, "B1", 10))
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		if pl := e.zonePool(z.Zone.ID); pl.Capacity != 5 || pl.Version != 2 {
			t.Fatalf("pool after save: %+v", pl)
		}
		got, err := e.svc.GetZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID)
		if err != nil || got.SpotCount != 5 {
			t.Fatalf("zone view: %+v %v", got, err)
		}

		// Moving a spot to the seat zone shrinks the pool.
		in = docInput(v2)
		in.Spots[0].ZoneID = seat.Zone.ID
		if _, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in); err != nil {
			t.Fatal(err)
		}
		if pl := e.zonePool(z.Zone.ID); pl.Capacity != 4 {
			t.Fatalf("pool after move: %+v", pl)
		}

		// System pools are read-only through the API.
		sys := e.zonePool(z.Zone.ID)
		if _, err := e.svc.UpdatePool(e.ctx, e.ownerA, e.estA.ID, sys.ID, nil, PoolInput{ComponentCode: "spot", Name: store.I18nText{"it": "x"}, Capacity: 99}); !errors.Is(err, ErrSystemPoolReadOnly) {
			t.Fatalf("update system pool: %v", err)
		}
		if err := e.svc.DeletePool(e.ctx, e.ownerA, e.estA.ID, sys.ID, nil); !errors.Is(err, ErrSystemPoolReadOnly) {
			t.Fatalf("delete system pool: %v", err)
		}

		// Mode change: zone -> seat removes the pool, back recreates it; a
		// rename follows into the pool name.
		zin := ZoneInput{MapID: &doc.Map.ID, Code: "front", Name: store.I18nText{"it": "Fronte mare"}, AssignmentMode: ModeSeat, OnlineVisible: true, PriceCategory: "front"}
		upd, err := e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, &z.Zone.Version, zin)
		if err != nil {
			t.Fatal(err)
		}
		if e.zonePool(z.Zone.ID) != nil || upd.PoolID != nil || upd.Zone.Version != 2 {
			t.Fatalf("pool kept after switching to seat: %+v", upd)
		}
		zin.AssignmentMode = ModeZone
		zin.AutoAssign = true
		upd, err = e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, nil, zin)
		if err != nil {
			t.Fatal(err)
		}
		if pl := e.zonePool(z.Zone.ID); pl == nil || pl.Capacity != 4 || pl.Name["it"] != "Fronte mare" || upd.PoolID == nil {
			t.Fatalf("pool not recreated: %+v", pl)
		}
		// Stale zone version.
		if _, err := e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, &z.Zone.Version, zin); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale zone update: %v", err)
		}
		// A zone with spots cannot become a quantity zone, move map, or be deleted.
		zin.AssignmentMode, zin.AutoAssign = ModeQuantity, false
		if _, err := e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, nil, zin); !errors.Is(err, ErrZoneHasSpots) {
			t.Fatalf("to quantity with spots: %v", err)
		}
		other := e.newMap(e.ownerA, e.estA.ID)
		zin.AssignmentMode, zin.MapID = ModeZone, &other.Map.ID
		if _, err := e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, nil, zin); !errors.Is(err, ErrZoneHasSpots) {
			t.Fatalf("move zone with spots: %v", err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, nil); !errors.Is(err, ErrZoneHasSpots) {
			t.Fatalf("delete zone with spots: %v", err)
		}

		// Deleting the map removes spots, zones and their pools.
		cur, _ := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID)
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, &cur.Map.Version); err != nil {
			t.Fatal(err)
		}
		if n := e.count("zones", "map_id = ?", doc.Map.ID); n != 0 {
			t.Fatalf("zones left: %d", n)
		}
		if n := e.count("pools", "zone_id = ?", z.Zone.ID); n != 0 {
			t.Fatalf("pools left: %d", n)
		}
		if _, err := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("deleted map: %v", err)
		}
	})
}

func TestMappingIntegration_GenerateRows(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.managerAll, e.estA.ID)
		z := e.newZone(e.managerAll, e.estA.ID, doc.Map.ID, "rows", ModeZone)
		params := GenerateParams{
			ZoneID: z.Zone.ID, Rows: 8, SpotsPerRow: 15, OriginX: 40, OriginY: 560, SpacingX: 60, SpacingY: 60,
			Capacity: map[string]int{"umbrella": 1, "sunbed": 2},
		}

		prev, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(prev.Spots) != 120 || prev.Applied || prev.Document != nil || len(prev.ConflictingLabels) != 0 {
			t.Fatalf("preview: %d spots applied=%v", len(prev.Spots), prev.Applied)
		}
		if n := e.count("spots", "map_id = ?", doc.Map.ID); n != 0 {
			t.Fatal("preview wrote spots")
		}
		if _, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, true, nil); !errors.Is(err, configerr.ErrPreconditionRequired) {
			t.Fatalf("apply without If-Match: %v", err)
		}
		stale := int64(7)
		if _, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, true, &stale); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("apply with stale If-Match: %v", err)
		}
		v := int64(1)
		res, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, true, &v)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Applied || res.Document == nil || res.Document.Map.Version != 2 || len(res.Document.Spots) != 120 {
			t.Fatalf("apply: %+v", res.Document.Map)
		}
		if h8 := spotByLabel(res.Document, "H15"); h8 == nil || h8.RowLabel != "H" || h8.Number != 15 || h8.Y != 560-7*60 {
			t.Fatalf("H15: %+v", h8)
		}
		if pl := e.zonePool(z.Zone.ID); pl.Capacity != 120 {
			t.Fatalf("pool after generation: %+v", pl)
		}
		if n := e.count("audit_log", "action = 'map.rows_generated'"); n != 1 {
			t.Fatalf("audit rows_generated: %d", n)
		}

		// Same block again: every label conflicts.
		prev, err = e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, false, nil)
		if err != nil || len(prev.ConflictingLabels) != 120 {
			t.Fatalf("preview conflicts: %d %v", len(prev.ConflictingLabels), err)
		}
		v = 2
		if _, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, true, &v); !errors.Is(err, ErrLabelsTaken) {
			t.Fatalf("apply conflicting: %v", err)
		}
		// A block that leaves the map is a validation error.
		params.OriginY, params.FirstRowLabel = 100, "J"
		if _, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, doc.Map.ID, params, false, nil); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("out of bounds: %v", err)
		}
		// Zone of another map.
		other := e.newMap(e.managerAll, e.estA.ID)
		if _, err := e.svc.GenerateRows(e.ctx, e.managerAll, e.estA.ID, other.Map.ID, params, false, nil); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("zone of another map: %v", err)
		}
	})
}

func TestMappingIntegration_CrossTenantAndPermissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeZone)
		docB := e.newMap(e.ownerB, e.estB.ID)

		// Owner B cannot reach establishment A at all.
		if _, err := e.svc.GetMap(e.ctx, e.ownerB, e.estA.ID, doc.Map.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign establishment: %v", err)
		}
		if _, err := e.svc.ListZones(e.ctx, e.ownerB, e.estA.ID, nil, "", 10); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign zones: %v", err)
		}
		// Through its own establishment, A's resources do not exist.
		if _, err := e.svc.GetMap(e.ctx, e.ownerB, e.estB.ID, doc.Map.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign map via own establishment: %v", err)
		}
		if _, err := e.svc.GetZone(e.ctx, e.ownerB, e.estB.ID, z.Zone.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign zone: %v", err)
		}
		if _, err := e.svc.SaveMap(e.ctx, e.ownerB, e.estB.ID, doc.Map.ID, 1, docInput(doc)); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("save foreign map: %v", err)
		}
		if err := e.svc.DeleteMap(e.ctx, e.ownerB, e.estB.ID, doc.Map.ID, nil); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("delete foreign map: %v", err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerB, e.estB.ID, z.Zone.ID, nil); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("delete foreign zone: %v", err)
		}
		pool := e.zonePool(z.Zone.ID)
		if _, err := e.svc.GetPool(e.ctx, e.ownerB, e.estB.ID, pool.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign pool: %v", err)
		}
		// A zone of A cannot be bound to B's map nor used by B's spots.
		zin := ZoneInput{MapID: &doc.Map.ID, Code: "x", Name: store.I18nText{"it": "X"}, AssignmentMode: ModeSeat, PriceCategory: "x"}
		if _, err := e.svc.CreateZone(e.ctx, e.ownerB, e.estB.ID, zin); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("zone on foreign map: %v", err)
		}
		inB := docInput(docB)
		inB.Spots = []SpotInput{spot(z.Zone.ID, "A1", 1)}
		if _, err := e.svc.SaveMap(e.ctx, e.ownerB, e.estB.ID, docB.Map.ID, 1, inB); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign zone in spots: %v", err)
		}
		// Lists are scoped.
		page, err := e.svc.ListMaps(e.ctx, e.ownerB, e.estB.ID, "", 50)
		if err != nil || len(page.Items) != 1 || page.Items[0].Map.ID != docB.Map.ID {
			t.Fatalf("maps of B: %+v %v", page, err)
		}
		pools, err := e.svc.ListPools(e.ctx, e.ownerB, e.estB.ID, nil, "", 50)
		if err != nil || len(pools.Items) != 0 {
			t.Fatalf("pools of B: %+v %v", pools, err)
		}

		// Cashier: read yes, configure no.
		if _, err := e.svc.GetMap(e.ctx, e.cashierA, e.estA.ID, doc.Map.ID); err != nil {
			t.Fatalf("cashier read: %v", err)
		}
		if _, err := e.svc.CreateMap(e.ctx, e.cashierA, e.estA.ID, mapInputOf(&doc.Map)); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier create: %v", err)
		}
		if _, err := e.svc.SaveMap(e.ctx, e.cashierA, e.estA.ID, doc.Map.ID, 1, docInput(doc)); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier save: %v", err)
		}
		if _, err := e.svc.GenerateRows(e.ctx, e.cashierA, e.estA.ID, doc.Map.ID, GenerateParams{}, false, nil); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier generate: %v", err)
		}
		if _, err := e.svc.GetMap(e.ctx, nil, e.estA.ID, doc.Map.ID); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
	})
}

func TestMappingIntegration_ZonesAndPoolsCRUD(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)
		if _, err := e.svc.CreateZone(e.ctx, e.ownerA, e.estA.ID, ZoneInput{MapID: &doc.Map.ID, Code: "row1", Name: store.I18nText{"it": "Dup"}, AssignmentMode: ModeSeat, PriceCategory: "row1"}); !errors.Is(err, ErrZoneCodeTaken) {
			t.Fatalf("duplicate code: %v", err)
		}
		bogus := store.NewID()
		if _, err := e.svc.CreateZone(e.ctx, e.ownerA, e.estA.ID, ZoneInput{MapID: &bogus, Code: "row9", Name: store.I18nText{"it": "X"}, AssignmentMode: ModeSeat, PriceCategory: "row9"}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("unknown map: %v", err)
		}
		seat := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row2", ModeSeat)
		qz := e.newZone(e.ownerA, e.estA.ID, "", "beds", ModeQuantity)

		// Pools.
		d := calendar.MustParse
		in := PoolInput{
			ZoneID: &qz.Zone.ID, ComponentCode: "sunbed", Name: store.I18nText{"it": "Lettini"}, Capacity: 40,
			CapacityOverrides: []store.CapacityOverride{{From: d("2027-08-01"), To: d("2027-08-31"), Capacity: 60}, {From: d("2027-06-01"), To: d("2027-06-30"), Capacity: 30}},
		}
		pl, err := e.svc.CreatePool(e.ctx, e.ownerA, e.estA.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.svc.GetPool(e.ctx, e.ownerA, e.estA.ID, pl.ID)
		if err != nil || got.Version != 1 || len(got.CapacityOverrides) != 2 || !got.CapacityOverrides[0].From.Equal(d("2027-06-01")) || got.SystemManaged {
			t.Fatalf("pool: %+v %v", got, err)
		}
		bad := in
		bad.ZoneID = &seat.Zone.ID
		if _, err := e.svc.CreatePool(e.ctx, e.ownerA, e.estA.ID, bad); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("pool on seat zone: %v", err)
		}
		bad = in
		bad.CapacityOverrides = append(slices.Clone(in.CapacityOverrides), store.CapacityOverride{From: d("2027-08-15"), To: d("2027-09-15"), Capacity: 1})
		if _, err := e.svc.CreatePool(e.ctx, e.ownerA, e.estA.ID, bad); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("overlapping overrides: %v", err)
		}
		in.Capacity = 45
		upd, err := e.svc.UpdatePool(e.ctx, e.ownerA, e.estA.ID, pl.ID, &pl.Version, in)
		if err != nil || upd.Version != 2 || upd.Capacity != 45 {
			t.Fatalf("update pool: %+v %v", upd, err)
		}
		if _, err := e.svc.UpdatePool(e.ctx, e.ownerA, e.estA.ID, pl.ID, &pl.Version, in); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale pool update: %v", err)
		}
		// Quantity zone view points at its bound pool; the zone cannot be
		// deleted nor switched to seat while bound.
		qv, err := e.svc.GetZone(e.ctx, e.ownerA, e.estA.ID, qz.Zone.ID)
		if err != nil || qv.PoolID == nil || *qv.PoolID != pl.ID {
			t.Fatalf("quantity zone pool: %+v %v", qv, err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, qz.Zone.ID, nil); !errors.Is(err, ErrZoneHasPools) {
			t.Fatalf("delete zone with pool: %v", err)
		}
		if err := e.svc.DeletePool(e.ctx, e.ownerA, e.estA.ID, pl.ID, &pl.Version); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale pool delete: %v", err)
		}
		if err := e.svc.DeletePool(e.ctx, e.ownerA, e.estA.ID, pl.ID, &upd.Version); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, qz.Zone.ID, &qz.Zone.Version); err != nil {
			t.Fatal(err)
		}

		// Zone listing with filter and pagination.
		page, err := e.svc.ListZones(e.ctx, e.ownerA, e.estA.ID, &doc.Map.ID, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("zones page 1: %+v %v", page, err)
		}
		page2, err := e.svc.ListZones(e.ctx, e.ownerA, e.estA.ID, &doc.Map.ID, page.NextCursor, 1)
		if err != nil || len(page2.Items) != 1 || page2.NextCursor != "" || page2.Items[0].Zone.ID == page.Items[0].Zone.ID {
			t.Fatalf("zones page 2: %+v %v", page2, err)
		}
		if _, err := e.svc.ListZones(e.ctx, e.ownerA, e.estA.ID, nil, "garbage!", 1); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}
		if n := e.count("audit_log", "entity_type IN ('zone', 'pool') AND establishment_id = ?", e.estA.ID); n != 7 {
			t.Fatalf("zone/pool audit entries: %d", n)
		}
	})
}

// TestConcurrency_SaveMap: parallel saves with the same If-Match version:
// exactly one wins, the others get a version conflict.
func TestConcurrency_SaveMap(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeZone)
		const n = 20
		var wg sync.WaitGroup
		results := make(chan error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				in := docInput(doc)
				in.Spots = []SpotInput{spot(z.Zone.ID, fmt.Sprintf("A%d", i), 10)}
				_, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
				results <- err
			}(i)
		}
		wg.Wait()
		close(results)
		ok := 0
		for err := range results {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, configerr.ErrVersionConflict):
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if ok != 1 {
			t.Fatalf("%d saves succeeded, want 1", ok)
		}
		got, err := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID)
		if err != nil || got.Map.Version != 2 || len(got.Spots) != 1 {
			t.Fatalf("final document: %+v %v", got, err)
		}
		if pl := e.zonePool(z.Zone.ID); pl.Capacity != 1 {
			t.Fatalf("pool: %+v", pl)
		}
	})
}

// TestMappingIntegration_OnlineBookableRoundtrip (spec 30.11, INV-105): the
// map save writes spots.online_excluded and the document reads it back; a
// save that only toggles the flag is a change.
func TestMappingIntegration_OnlineBookableRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)
		in := docInput(doc)
		hidden := spot(z.Zone.ID, "A1", 10)
		hidden.OnlineExcluded = true
		in.Spots = []SpotInput{hidden, spot(z.Zone.ID, "A2", 20)}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		if a1, a2 := spotByLabel(v2, "A1"), spotByLabel(v2, "A2"); !a1.OnlineExcluded || a2.OnlineExcluded {
			t.Fatalf("after save: A1 excluded=%v A2 excluded=%v", a1.OnlineExcluded, a2.OnlineExcluded)
		}
		in = docInput(v2)
		for i := range in.Spots {
			in.Spots[i].OnlineExcluded = in.Spots[i].Label == "A2"
		}
		v3, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		if err != nil {
			t.Fatal(err)
		}
		if a1, a2 := spotByLabel(v3, "A1"), spotByLabel(v3, "A2"); v3.Map.Version != 3 || a1.OnlineExcluded || !a2.OnlineExcluded {
			t.Fatalf("after toggling: v%d A1 excluded=%v A2 excluded=%v", v3.Map.Version, a1.OnlineExcluded, a2.OnlineExcluded)
		}
	})
}
