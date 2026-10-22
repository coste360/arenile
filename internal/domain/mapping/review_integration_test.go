//go:build integration

package mapping

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// insert writes a row of any model directly (test fixtures of other areas).
func (e *env) insert(m any) {
	e.t.Helper()
	if _, err := e.db.NewInsert().Model(m).Exec(e.ctx); err != nil {
		e.t.Fatalf("insert %T: %v", m, err)
	}
}

func (e *env) season(est string) *store.Season {
	e.t.Helper()
	now := e.svc.now()
	sn := &store.Season{
		ID: store.NewID(), EstablishmentID: est, Year: 2027, OpensOn: calendar.MustParse("2027-05-15"),
		ClosesOn: calendar.MustParse("2027-09-15"), Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	e.insert(sn)
	return sn
}

func mustJSON(t *testing.T, v any) store.JSON {
	t.Helper()
	j, err := store.MarshalJSONValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// TestMappingIntegration_DeleteMapRefusesBoundPools: a map whose quantity
// zone has a hand-made pool cannot be deleted, exactly as the zone alone
// cannot (no pool vanishes silently, components keep their pool_id).
func TestMappingIntegration_DeleteMapRefusesBoundPools(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		qz := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "beds", ModeQuantity)
		pl, err := e.svc.CreatePool(e.ctx, e.ownerA, e.estA.ID, PoolInput{ZoneID: &qz.Zone.ID, ComponentCode: "sunbed", Name: store.I18nText{"it": "Lettini"}, Capacity: 40})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, qz.Zone.ID, nil); !errors.Is(err, ErrZoneHasPools) {
			t.Fatalf("delete zone: %v", err)
		}
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); !errors.Is(err, ErrZoneHasPools) || !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("delete map with bound pool: %v", err)
		}
		if _, err := e.svc.GetPool(e.ctx, e.ownerA, e.estA.ID, pl.ID); err != nil {
			t.Fatalf("pool must survive the refused deletion: %v", err)
		}
		if _, err := e.svc.GetMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID); err != nil {
			t.Fatalf("map must survive: %v", err)
		}
		// Nothing of the refused transaction leaked: no audit, no event.
		if n := e.count("audit_log", "action = ? AND establishment_id = ?", "map.deleted", e.estA.ID); n != 0 {
			t.Fatalf("map.deleted audit entries: %d", n)
		}
		// Unbinding the pool makes both deletions possible.
		if err := e.svc.DeletePool(e.ctx, e.ownerA, e.estA.ID, pl.ID, nil); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); err != nil {
			t.Fatalf("delete map: %v", err)
		}
		if _, err := e.svc.GetZone(e.ctx, e.ownerA, e.estA.ID, qz.Zone.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("zone deleted with its map: %v", err)
		}
	})
}

// TestMappingIntegration_ReferencedZoneCannotBeDeleted: price rules, promo
// codes, agreements (conditions.zone_ids), packs (allowed_zone_ids) and
// booking policies (scope.zones by id or code) keep a zone, and its map,
// from being deleted; the error lists the references.
func TestMappingIntegration_ReferencedZoneCannotBeDeleted(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		now := e.svc.now()
		sn := e.season(e.estA.ID)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeSeat)
		other := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row2", ModeSeat)

		rule := &store.PriceRule{
			ID: store.NewID(), EstablishmentID: e.estA.ID, SeasonID: sn.ID, Name: "Prima fila", TargetType: "pack",
			Conditions: mustJSON(t, pricing.Conditions{ZoneIDs: []string{z.Zone.ID}}),
			Action:     mustJSON(t, pricing.Action{Type: pricing.ActionAdd, Amount: 500}),
			Stage:      "modifier", Active: true, Seq: 1, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		e.insert(rule)
		promo := &store.PromoCode{
			ID: store.NewID(), EstablishmentID: e.estA.ID, Code: "FILA1", Action: mustJSON(t, pricing.Action{Type: pricing.ActionAdd, Amount: -100}),
			Conditions: mustJSON(t, pricing.Conditions{ZoneIDs: []string{z.Zone.ID}}), Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		e.insert(promo)
		agreement := &store.Agreement{
			ID: store.NewID(), EstablishmentID: e.estA.ID, Name: "Hotel Mare", Action: mustJSON(t, pricing.Action{Type: pricing.ActionAdd, Amount: -100}),
			Conditions: mustJSON(t, pricing.Conditions{ZoneIDs: []string{z.Zone.ID}}), Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		e.insert(agreement)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: e.estA.ID, Code: "vip", Name: store.I18nText{"it": "VIP"}, Description: store.I18nText{},
			Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"}, MaxPerBooking: 1,
			AllowedZoneIDs: []string{z.Zone.ID}, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		e.insert(pack)
		pol := &store.BookingPolicy{
			ID: store.NewID(), EstablishmentID: e.estA.ID, SeasonID: sn.ID, Name: "Prima fila solo in cassa",
			Scope: mustJSON(t, policy.Scope{Zones: []string{z.Zone.Code}}), Rules: mustJSON(t, policy.Rules{}),
			Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		e.insert(pol)

		var zr *ZoneReferencedError
		err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, z.Zone.ID, nil)
		if !errors.As(err, &zr) || !errors.Is(err, ErrZoneReferenced) || !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("delete referenced zone: %v", err)
		}
		kinds := map[string]string{}
		for _, r := range zr.References {
			kinds[r.Kind] = r.ID
		}
		want := map[string]string{RefPriceRule: rule.ID, RefPromoCode: promo.ID, RefAgreement: agreement.ID, RefPack: pack.ID, RefBookingPolicy: pol.ID}
		if len(kinds) != len(want) {
			t.Fatalf("references: %+v", zr.References)
		}
		for k, id := range want {
			if kinds[k] != id {
				t.Fatalf("reference %s: %q want %q (%+v)", k, kinds[k], id, zr.References)
			}
		}
		// The map holding the zone is refused for the same reason; the
		// unreferenced zone alone can go.
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); !errors.Is(err, ErrZoneReferenced) {
			t.Fatalf("delete map with referenced zone: %v", err)
		}
		if err := e.svc.DeleteZone(e.ctx, e.ownerA, e.estA.ID, other.Zone.ID, nil); err != nil {
			t.Fatalf("delete unreferenced zone: %v", err)
		}
		// Removing the references frees the zone.
		for _, m := range []any{rule, promo, agreement, pack, pol} {
			if _, err := db.NewDelete().Model(m).WherePK().Exec(e.ctx); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); err != nil {
			t.Fatalf("delete map once unreferenced: %v", err)
		}
	})
}

// TestMappingIntegration_BadTimezoneIsAnError: the future allocation check
// never falls back to UTC (same rule as the policy evaluation).
func TestMappingIntegration_BadTimezoneIsAnError(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		doc := e.newMap(e.ownerA, e.estA.ID)
		z := e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, "row1", ModeZone)
		in := docInput(doc)
		in.Spots = []SpotInput{spot(z.Zone.ID, "A1", 10)}
		v2, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 1, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewUpdate().TableExpr("establishments").Set("timezone = ?", "Mars/Olympus").Where("id = ?", e.estA.ID).Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		in = docInput(v2)
		in.Spots = nil
		_, err = e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in)
		if err == nil || errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("deleting a spot with a bad timezone must fail, got %v", err)
		}
		if len(e.alloc.from) != 0 {
			t.Fatalf("the allocation checker ran with a guessed date: %v", e.alloc.from)
		}
		if err := e.svc.DeleteMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, nil); err == nil {
			t.Fatal("deleting the map with a bad timezone must fail")
		}
		// Without deletions the timezone is not needed: the save works.
		in = docInput(v2)
		if _, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, doc.Map.ID, 2, in); err != nil {
			t.Fatalf("save without deletions: %v", err)
		}
	})
}

// TestConcurrency_ZoneUpdateVsSaveMap: a save that adds spots to a zone and
// an update that turns the zone into `quantity` race on 100 fresh pairs
// (one map and one zone each, so that saves never collide on the map
// version); one of the two must always lose, so that no quantity zone ever
// has spots and the implicit pool of a zone that stayed `zone` counts them.
func TestConcurrency_ZoneUpdateVsSaveMap(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		const n = 100
		type pair struct {
			doc  *Document
			zone *ZoneView
		}
		pairs := make([]pair, n)
		for i := range pairs {
			doc := e.newMap(e.ownerA, e.estA.ID)
			pairs[i] = pair{doc: doc, zone: e.newZone(e.ownerA, e.estA.ID, doc.Map.ID, fmt.Sprintf("z%03d", i), ModeZone)}
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2*n)
		for i := 0; i < n; i++ {
			pr := pairs[i]
			wg.Add(2)
			go func() {
				defer wg.Done()
				in := docInput(pr.doc)
				in.Spots = []SpotInput{spot(pr.zone.Zone.ID, "A1", 10)}
				_, err := e.svc.SaveMap(e.ctx, e.ownerA, e.estA.ID, pr.doc.Map.ID, 1, in)
				if err != nil && !errors.Is(err, configerr.ErrValidation) {
					errs <- err
				}
			}()
			go func() {
				defer wg.Done()
				in := ZoneInput{MapID: &pr.doc.Map.ID, Code: pr.zone.Zone.Code, Name: pr.zone.Zone.Name, AssignmentMode: ModeQuantity, OnlineVisible: true, PriceCategory: pr.zone.Zone.Code}
				_, err := e.svc.UpdateZone(e.ctx, e.ownerA, e.estA.ID, pr.zone.Zone.ID, nil, in)
				if err != nil && !errors.Is(err, ErrZoneHasSpots) {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("unexpected error: %v", err)
		}
		es, err := store.SystemEstablishmentScope(e.ctx, db, e.estA.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, n)
		for i, pr := range pairs {
			ids[i] = pr.zone.Zone.ID
		}
		counts, err := store.MappingSpotCountsByZone(e.ctx, db, es, ids)
		if err != nil {
			t.Fatal(err)
		}
		final, err := store.ListZonesByIDs(e.ctx, db, es, ids)
		if err != nil {
			t.Fatal(err)
		}
		quantityWithSpots, saves, updates := 0, 0, 0
		for _, z := range final {
			switch {
			case z.AssignmentMode == ModeQuantity && counts[z.ID] > 0:
				quantityWithSpots++
			case z.AssignmentMode == ModeQuantity:
				updates++
			default:
				saves++
				if pl := e.zonePool(z.ID); pl == nil || pl.Capacity != counts[z.ID] {
					t.Fatalf("zone %s: pool %+v, spots %d", z.Code, pl, counts[z.ID])
				}
			}
		}
		if quantityWithSpots != 0 {
			t.Fatalf("%d quantity zones with spots (saves won %d, updates won %d)", quantityWithSpots, saves, updates)
		}
		if saves+updates != n {
			t.Fatalf("saves %d + updates %d != %d", saves, updates, n)
		}
		t.Logf("saves won %d, updates won %d", saves, updates)
	})
}
