//go:build integration

package catalog_test

import (
	"reflect"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var t0 = time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)

type env struct {
	ctx        context.Context
	db         *store.DB
	svc        *catalog.Service
	estA, estB *store.Establishment
	ownerA     *auth.Principal
	managerA   *auth.Principal
	viewerA    *auth.Principal
	ownerB     *auth.Principal
	zoneA      *store.Zone
	zoneB      *store.Zone
	poolA      *store.Pool // component "umbrella"
	poolB      *store.Pool
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(t0)
	e := &env{
		ctx: ctx, db: db, estA: estA, estB: estB,
		svc: catalog.NewService(catalog.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}),
		ownerA: &auth.Principal{
			Kind: auth.PrincipalUser, UserID: store.NewID(),
			Grants: []auth.Grant{{OrganizationID: orgA.ID, Role: auth.RoleOwner}},
		},
		managerA: &auth.Principal{
			Kind: auth.PrincipalUser, UserID: store.NewID(),
			Grants: []auth.Grant{{OrganizationID: orgA.ID, EstablishmentID: estA.ID, Role: auth.RoleManager}},
		},
		viewerA: &auth.Principal{
			Kind: auth.PrincipalUser, UserID: store.NewID(),
			Grants: []auth.Grant{{OrganizationID: orgA.ID, EstablishmentID: estA.ID, Role: auth.RoleViewer}},
		},
		ownerB: &auth.Principal{
			Kind: auth.PrincipalUser, UserID: store.NewID(),
			Grants: []auth.Grant{{OrganizationID: orgB.ID, Role: auth.RoleOwner}},
		},
	}
	e.zoneA, e.poolA = e.inventory(t, estA.ID)
	e.zoneB, e.poolB = e.inventory(t, estB.ID)
	return e
}

func (e *env) insert(t *testing.T, m any) {
	t.Helper()
	if _, err := e.db.NewInsert().Model(m).Exec(e.ctx); err != nil {
		t.Fatalf("insert %T: %v", m, err)
	}
}

func (e *env) inventory(t *testing.T, estID string) (*store.Zone, *store.Pool) {
	t.Helper()
	z := &store.Zone{
		ID: store.NewID(), EstablishmentID: estID, Code: "row1", Name: store.I18nText{"it": "Prima fila"},
		AssignmentMode: "seat", PriceCategory: "first", Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, z)
	p := &store.Pool{
		ID: store.NewID(), EstablishmentID: estID, ComponentCode: "umbrella", Name: store.I18nText{"it": "Ombrelloni"},
		Capacity: 10, CapacityOverrides: []store.CapacityOverride{}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	e.insert(t, p)
	return z, p
}

func (e *env) count(t *testing.T, table, where string, args ...any) int {
	t.Helper()
	n, err := e.db.NewSelect().TableExpr(table).Where(where, args...).Count(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func it(s string) store.I18nText { return store.I18nText{"it": s} }

func ptr[T any](v T) *T { return &v }

func mustValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("want field %q, got %+v", field, ve.Fields)
}

func packInput(code string) catalog.PackInput {
	return catalog.PackInput{
		Code: code, Name: it("Ombrellone + 2 lettini"),
		Composition:     map[string]int{"umbrella": 1, "sunbed": 2},
		AllowedFormulas: []string{"day", "half_day", "season"}, Channels: []string{"desk", "online"},
	}
}

func TestCatalogIntegration_CRUD(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx, a := e.ctx, e.estA.ID

		// Components.
		_, err := e.svc.CreateComponent(ctx, e.ownerA, "1.2.3.4", a, catalog.ComponentInput{Code: "umbrella", Name: it("Ombrellone"), Tracked: true})
		mustValidation(t, err, "pool_id")
		_, err = e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "umbrella", Name: it("Ombrellone"), Tracked: true, PoolID: &e.poolB.ID})
		mustValidation(t, err, "pool_id") // pool of another establishment
		_, err = e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino"), Tracked: true, PoolID: &e.poolA.ID})
		mustValidation(t, err, "pool_id") // pool holds umbrellas
		umb, err := e.svc.CreateComponent(ctx, e.ownerA, "1.2.3.4", a, catalog.ComponentInput{Code: "umbrella", Name: it("Ombrellone"), Tracked: true, PoolID: &e.poolA.ID})
		if err != nil || umb.Version != 1 || !umb.Tracked {
			t.Fatalf("create umbrella: %+v %v", umb, err)
		}
		sun, err := e.svc.CreateComponent(ctx, e.managerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Doppio")}); !errors.Is(err, catalog.ErrDuplicateCode) || !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate code: %v", err)
		}
		// Same code in another establishment is fine.
		if _, err := e.svc.CreateComponent(ctx, e.ownerB, "", e.estB.ID, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")}); err != nil {
			t.Fatal(err)
		}
		page, err := e.svc.ListComponents(ctx, e.viewerA, a, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("page 1: %+v %v", page, err)
		}
		page, err = e.svc.ListComponents(ctx, e.viewerA, a, page.NextCursor, 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor != "" || page.Items[0].ID != sun.ID {
			t.Fatalf("page 2: %+v %v", page, err)
		}

		// Packs.
		bad := packInput("std")
		bad.Composition = map[string]int{"umbrella": 1, "deckchair": 1}
		_, err = e.svc.CreatePack(ctx, e.ownerA, "", a, bad)
		mustValidation(t, err, "composition.deckchair")
		bad = packInput("std")
		bad.AllowedZoneIDs = []string{e.zoneB.ID}
		_, err = e.svc.CreatePack(ctx, e.ownerA, "", a, bad)
		mustValidation(t, err, "allowed_zone_ids[0]")

		in := packInput("std")
		in.AllowedZoneIDs = []string{e.zoneA.ID}
		in.RequiresSpotKind = []string{"umbrella"}
		pk, err := e.svc.CreatePack(ctx, e.ownerA, "", a, in)
		if err != nil {
			t.Fatal(err)
		}
		if !pk.Active || pk.MaxPerBooking != 1 || pk.VATRateBP != nil || pk.Version != 1 {
			t.Fatalf("defaults: %+v", pk)
		}
		got, err := e.svc.GetPack(ctx, e.viewerA, a, pk.ID)
		if err != nil || got.Composition["sunbed"] != 2 || len(got.AllowedZoneIDs) != 1 || got.RequiresSpotKind[0] != "umbrella" {
			t.Fatalf("get pack: %+v %v", got, err)
		}

		// A pack may require a kit spot kind (ADR 0012): the catalog and the
		// map share one list, so the pack is accepted and read back.
		kit := packInput("lettino")
		kit.RequiresSpotKind = []string{"sunbed", "daybed"}
		kp, err := e.svc.CreatePack(ctx, e.ownerA, "", a, kit)
		if err != nil {
			t.Fatalf("kit kind pack: %v", err)
		}
		if got, err := e.svc.GetPack(ctx, e.viewerA, a, kp.ID); err != nil || !reflect.DeepEqual(got.RequiresSpotKind, []string{"sunbed", "daybed"}) {
			t.Fatalf("kit kind pack read back: %+v %v", got, err)
		}
		if _, err := e.svc.DeletePack(ctx, e.ownerA, "", a, kp.ID, nil); err != nil {
			t.Fatalf("delete kit kind pack: %v", err)
		}

		in.Name = it("Standard")
		in.VATRateBP = ptr(int64(2200))
		in.MaxPerBooking = ptr(4)
		in.AllowedZoneIDs = nil
		var vc *configerr.VersionConflictError
		if _, err := e.svc.UpdatePack(ctx, e.ownerA, "", a, pk.ID, ptr(int64(7)), in); !errors.As(err, &vc) || vc.Current != 1 {
			t.Fatalf("stale if-match: %v", err)
		}
		pk2, err := e.svc.UpdatePack(ctx, e.ownerA, "", a, pk.ID, ptr(int64(1)), in)
		if err != nil || pk2.Version != 2 || pk2.Name["it"] != "Standard" || *pk2.VATRateBP != 2200 || pk2.MaxPerBooking != 4 || pk2.AllowedZoneIDs != nil {
			t.Fatalf("update pack: %+v %v", pk2, err)
		}
		in.Active = ptr(false)
		if _, err := e.svc.UpdatePack(ctx, e.ownerA, "", a, pk.ID, nil, in); err != nil {
			t.Fatal(err)
		}
		in.Active = nil // keeps current state
		pk4, err := e.svc.UpdatePack(ctx, e.ownerA, "", a, pk.ID, nil, in)
		if err != nil || pk4.Active || pk4.Version != 4 {
			t.Fatalf("active kept: %+v %v", pk4, err)
		}
		inactive, err := e.svc.ListPacks(ctx, e.viewerA, a, ptr(false), "", 50)
		if err != nil || len(inactive.Items) != 1 {
			t.Fatalf("inactive filter: %+v %v", inactive, err)
		}
		active, err := e.svc.ListPacks(ctx, e.viewerA, a, ptr(true), "", 50)
		if err != nil || len(active.Items) != 0 {
			t.Fatalf("active filter: %+v %v", active, err)
		}

		// Components used by a pack can be neither renamed nor deleted.
		if _, err := e.svc.UpdateComponent(ctx, e.ownerA, "", a, sun.ID, nil, catalog.ComponentInput{Code: "lounger", Name: it("Lettino")}); !errors.Is(err, catalog.ErrComponentInUse) {
			t.Fatalf("rename used component: %v", err)
		}
		if err := e.svc.DeleteComponent(ctx, e.ownerA, "", a, sun.ID, nil); !errors.Is(err, catalog.ErrComponentInUse) {
			t.Fatalf("delete used component: %v", err)
		}
		// Renaming the name only is fine.
		sun2, err := e.svc.UpdateComponent(ctx, e.ownerA, "", a, sun.ID, ptr(int64(1)), catalog.ComponentInput{Code: "sunbed", Name: it("Lettino prendisole")})
		if err != nil || sun2.Version != 2 {
			t.Fatalf("update component: %+v %v", sun2, err)
		}

		// Extras.
		_, err = e.svc.CreateExtra(ctx, e.ownerA, "", a, catalog.ExtraInput{Code: "cabin", Name: it("Cabina"), PricingUnit: "per_day", AttachTo: "line", Channels: []string{"desk"}, ComponentCode: ptr("cabin")})
		mustValidation(t, err, "component_code")
		otherPack, err := e.svc.CreatePack(ctx, e.ownerB, "", e.estB.ID, catalog.PackInput{Code: "b", Name: it("B"), Composition: map[string]int{"sunbed": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.svc.CreateExtra(ctx, e.ownerA, "", a, catalog.ExtraInput{Code: "extra-bed", Name: it("Lettino extra"), PricingUnit: "per_day", Channels: []string{"desk"}, AllowedPackIDs: []string{pk.ID, otherPack.ID}})
		mustValidation(t, err, "allowed_pack_ids[1]")
		xb, err := e.svc.CreateExtra(ctx, e.ownerA, "", a, catalog.ExtraInput{
			Code: "extra-bed", Name: it("Lettino extra"), PricingUnit: "per_day", Channels: []string{"desk", "online"},
			ComponentCode: ptr("sunbed"), AllowedPackIDs: []string{pk.ID}, MaxQuantity: ptr(3),
		})
		if err != nil || xb.AttachTo != "line" || xb.MaxQuantity != 3 || *xb.ComponentCode != "sunbed" {
			t.Fatalf("create extra: %+v %v", xb, err)
		}
		towel, err := e.svc.CreateExtra(ctx, e.ownerA, "", a, catalog.ExtraInput{Code: "towel", Name: it("Telo"), PricingUnit: "per_booking", AttachTo: "booking", Channels: []string{"online"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.UpdateExtra(ctx, e.ownerA, "", a, towel.ID, ptr(int64(1)), catalog.ExtraInput{Code: "towel", Name: it("Telo mare"), PricingUnit: "per_day", AttachTo: "booking", Channels: []string{"online"}}); err != nil {
			t.Fatal(err)
		}
		// Unreferenced extra: hard delete.
		archived, err := e.svc.DeleteExtra(ctx, e.ownerA, "", a, towel.ID, ptr(int64(2)))
		if err != nil || archived {
			t.Fatalf("delete extra: %v %v", archived, err)
		}
		if _, err := e.svc.GetExtra(ctx, e.ownerA, a, towel.ID); !errors.Is(err, catalog.ErrExtraNotFound) {
			t.Fatalf("deleted extra: %v", err)
		}

		// Pack referenced by an extra: archived, not deleted.
		archived, err = e.svc.DeletePack(ctx, e.ownerA, "", a, pk.ID, nil)
		if err != nil || !archived {
			t.Fatalf("delete referenced pack: %v %v", archived, err)
		}

		// Extra targeted by a price rule: archived.
		season := &store.Season{
			ID: store.NewID(), EstablishmentID: a, Year: 2027, OpensOn: calendar.MustParse("2027-05-01"),
			ClosesOn: calendar.MustParse("2027-09-30"), Status: "draft", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		e.insert(t, season)
		e.insert(t, &store.PriceRule{
			ID: store.NewID(), EstablishmentID: a, SeasonID: season.ID, Name: "Lettino", TargetType: "extra",
			TargetIDs: []string{xb.ID}, Conditions: store.JSON("{}"), Action: store.JSON(`{"type":"set","amount":500}`), Stage: "base",
			Active: true, Seq: 1, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		archived, err = e.svc.DeleteExtra(ctx, e.ownerA, "", a, xb.ID, ptr(int64(1)))
		if err != nil || !archived {
			t.Fatalf("delete targeted extra: %v %v", archived, err)
		}
		xbArch, err := e.svc.GetExtra(ctx, e.ownerA, a, xb.ID)
		if err != nil || xbArch.Active || xbArch.Version != 2 {
			t.Fatalf("archived extra: %+v %v", xbArch, err)
		}

		// Pack with fixed prices only: archived (fixed prices would cascade).
		pk2b, err := e.svc.CreatePack(ctx, e.ownerA, "", a, packInput("season-pass"))
		if err != nil {
			t.Fatal(err)
		}
		e.insert(t, &store.FixedDurationPrice{
			ID: store.NewID(), EstablishmentID: a, SeasonID: season.ID, PackID: pk2b.ID,
			Formula: "season", PriceCategory: "first", Amount: 150000, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		if archived, err := e.svc.DeletePack(ctx, e.ownerA, "", a, pk2b.ID, nil); err != nil || !archived {
			t.Fatalf("pack with fixed prices: %v %v", archived, err)
		}
		// Unreferenced pack: hard delete.
		pk3, err := e.svc.CreatePack(ctx, e.ownerA, "", a, packInput("tmp"))
		if err != nil {
			t.Fatal(err)
		}
		if archived, err := e.svc.DeletePack(ctx, e.ownerA, "", a, pk3.ID, ptr(int64(1))); err != nil || archived {
			t.Fatalf("unreferenced pack: %v %v", archived, err)
		}

		// Pack code listed in a policy scope cannot be renamed.
		e.insert(t, &store.BookingPolicy{
			ID: store.NewID(), EstablishmentID: a, SeasonID: season.ID, Name: "Pass",
			Scope: store.JSON(`{"packs":["season-pass"]}`), Rules: store.JSON("{}"), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		if _, err := e.svc.UpdatePack(ctx, e.ownerA, "", a, pk2b.ID, nil, packInput("pass")); !errors.Is(err, catalog.ErrPackCodeInUse) {
			t.Fatalf("rename pack in policy: %v", err)
		}

		// Unused component: deletable. Pools still reference "umbrella".
		cabin, err := e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "cabin", Name: it("Cabina")})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.svc.DeleteComponent(ctx, e.ownerA, "", a, cabin.ID, ptr(int64(2))); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale delete: %v", err)
		}
		if err := e.svc.DeleteComponent(ctx, e.ownerA, "", a, cabin.ID, ptr(int64(1))); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.GetComponent(ctx, e.ownerA, a, cabin.ID); !errors.Is(err, catalog.ErrComponentNotFound) {
			t.Fatalf("deleted component: %v", err)
		}

		// Audit and outbox, written with the changes.
		if n := e.count(t, "audit_log", "establishment_id = ? AND action = ?", a, "component.created"); n != 3 {
			t.Fatalf("component.created audit rows = %d", n)
		}
		if n := e.count(t, "audit_log", "establishment_id = ? AND action = ? AND ip = ?", a, "component.created", "1.2.3.4"); n != 1 {
			t.Fatalf("audit ip rows = %d", n)
		}
		for _, action := range []string{"pack.created", "pack.updated", "pack.archived", "pack.deleted", "extra.created", "extra.updated", "extra.deleted", "extra.archived", "component.updated", "component.deleted"} {
			if n := e.count(t, "audit_log", "establishment_id = ? AND action = ?", a, action); n == 0 {
				t.Errorf("missing audit action %s", action)
			}
		}
		// Packs of A: 4 created, 3 updated, 1 archived (the first pack was already
		// inactive), 2 deleted (the kit-kind pack among them); extras: 2 created,
		// 1 updated, 1 deleted, 1 archived.
		if n := e.count(t, "outbox_events", "establishment_id = ? AND type = ?", a, event.ConfigPricingChanged); n != 15 {
			t.Fatalf("pricing_changed events = %d", n)
		}
		var payload string
		if err := db.NewSelect().TableExpr("outbox_events").ColumnExpr("payload").Where("establishment_id = ?", a).
			OrderExpr("id DESC").Limit(1).Scan(ctx, &payload); err != nil {
			t.Fatal(err)
		}
		var cd catalog.ChangeData
		if err := json.Unmarshal([]byte(payload), &cd); err != nil || cd.Entity != "pack" || cd.Change != catalog.ChangeDeleted || cd.EntityID != pk3.ID {
			t.Fatalf("payload %s: %+v %v", payload, cd, err)
		}

		// Pricing view.
		cat, err := e.svc.PricingCatalog(ctx, e.viewerA, a)
		if err != nil {
			t.Fatal(err)
		}
		if len(cat.Packs) != 2 || len(cat.Extras) != 1 || len(cat.Zones) != 1 || cat.Settings.Currency != "EUR" {
			t.Fatalf("pricing catalog: %+v", cat)
		}
		for _, p := range cat.Packs {
			if p.Active {
				t.Fatalf("archived pack must be inactive: %+v", p)
			}
		}
	})
}

func TestCatalogIntegration_Permissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx, a := e.ctx, e.estA.ID
		if _, err := e.svc.CreateComponent(ctx, e.viewerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer create: %v", err)
		}
		if _, err := e.svc.CreatePack(ctx, e.viewerA, "", a, packInput("std")); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer create pack: %v", err)
		}
		if _, err := e.svc.CreateExtra(ctx, nil, "", a, catalog.ExtraInput{}); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
		cashier := &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: e.estA.OrganizationID, EstablishmentID: a, Role: auth.RoleCashier}}}
		if _, err := e.svc.CreateExtra(ctx, cashier, "", a, catalog.ExtraInput{}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		if _, err := e.svc.ListPacks(ctx, cashier, a, nil, "", 10); err != nil {
			t.Fatalf("cashier read: %v", err)
		}
		if _, err := e.svc.CreateComponent(ctx, e.managerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")}); err != nil {
			t.Fatalf("manager create: %v", err)
		}
		apiKey := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "k1", APIKeyEstablishmentID: a, APIKeyOrganizationID: e.estA.OrganizationID, Scopes: []auth.Permission{auth.PermBookingsRead}}
		if _, err := e.svc.ListComponents(ctx, apiKey, a, "", 10); err != nil {
			t.Fatalf("api key read: %v", err)
		}
		if _, err := e.svc.CreateComponent(ctx, apiKey, "", a, catalog.ComponentInput{Code: "x", Name: it("X")}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("api key write: %v", err)
		}
		if n := e.count(t, "audit_log", "establishment_id = ?", a); n != 1 {
			t.Fatalf("only the manager's change is audited, got %d", n)
		}
	})
}

func TestCatalogIntegration_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx, a, b := e.ctx, e.estA.ID, e.estB.ID
		if _, err := e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")}); err != nil {
			t.Fatal(err)
		}
		comp, err := e.svc.CreateComponent(ctx, e.ownerA, "", a, catalog.ComponentInput{Code: "umbrella", Name: it("Ombrellone")})
		if err != nil {
			t.Fatal(err)
		}
		pk, err := e.svc.CreatePack(ctx, e.ownerA, "", a, packInput("std"))
		if err != nil {
			t.Fatal(err)
		}
		x, err := e.svc.CreateExtra(ctx, e.ownerA, "", a, catalog.ExtraInput{Code: "towel", Name: it("Telo"), PricingUnit: "per_day", Channels: []string{"desk"}})
		if err != nil {
			t.Fatal(err)
		}
		// Owner B addressing establishment A: not found (no existence leak).
		if _, err := e.svc.GetPack(ctx, e.ownerB, a, pk.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign establishment: %v", err)
		}
		if _, err := e.svc.ListPacks(ctx, e.ownerB, a, nil, "", 10); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign list: %v", err)
		}
		if _, err := e.svc.PricingCatalog(ctx, e.ownerB, a); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign pricing catalog: %v", err)
		}
		// Owner B addressing A's rows through establishment B: not found.
		if _, err := e.svc.GetPack(ctx, e.ownerB, b, pk.ID); !errors.Is(err, catalog.ErrPackNotFound) {
			t.Fatalf("foreign pack: %v", err)
		}
		if _, err := e.svc.GetExtra(ctx, e.ownerB, b, x.ID); !errors.Is(err, catalog.ErrExtraNotFound) {
			t.Fatalf("foreign extra: %v", err)
		}
		if _, err := e.svc.UpdateComponent(ctx, e.ownerB, "", b, comp.ID, nil, catalog.ComponentInput{Code: "umbrella", Name: it("Hack")}); !errors.Is(err, catalog.ErrComponentNotFound) {
			t.Fatalf("foreign update: %v", err)
		}
		if _, err := e.svc.DeletePack(ctx, e.ownerB, "", b, pk.ID, nil); !errors.Is(err, catalog.ErrPackNotFound) {
			t.Fatalf("foreign delete: %v", err)
		}
		if err := e.svc.DeleteComponent(ctx, e.ownerB, "", b, comp.ID, nil); !errors.Is(err, catalog.ErrComponentNotFound) {
			t.Fatalf("foreign delete component: %v", err)
		}
		page, err := e.svc.ListComponents(ctx, e.ownerB, b, "", 50)
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("B sees A's components: %+v %v", page, err)
		}
		// A's pack is untouched.
		got, err := e.svc.GetPack(ctx, e.ownerA, a, pk.ID)
		if err != nil || got.Version != 1 || !got.Active {
			t.Fatalf("pack A changed: %+v %v", got, err)
		}
	})
}

// TestCatalogIntegration_WithDB checks that the service composes into a
// caller's transaction (guided setup): a rollback undoes the catalog writes.
func TestCatalogIntegration_WithDB(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		errAbort := errors.New("abort")
		err := db.RunInTx(e.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if _, err := e.svc.WithDB(tx).CreateComponent(ctx, e.ownerA, "", e.estA.ID, catalog.ComponentInput{Code: "sunbed", Name: it("Lettino")}); err != nil {
				return err
			}
			return errAbort
		})
		if !errors.Is(err, errAbort) {
			t.Fatal(err)
		}
		if n := e.count(t, "components", "establishment_id = ?", e.estA.ID); n != 0 {
			t.Fatalf("rolled back component persisted: %d", n)
		}
		if n := e.count(t, "audit_log", "establishment_id = ?", e.estA.ID); n != 0 {
			t.Fatalf("rolled back audit persisted: %d", n)
		}
	})
}
