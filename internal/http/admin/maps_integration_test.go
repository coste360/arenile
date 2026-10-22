//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// mapsAllocations marks spots as allocated for the HTTP tests.
type mapsAllocations struct{ busy map[string]bool }

func (m *mapsAllocations) SpotsWithFutureAllocations(_ context.Context, _ bun.IDB, _ store.EstablishmentScope, ids []string, _ calendar.Date) ([]string, error) {
	var out []string
	for _, id := range ids {
		if m.busy[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

type mapsEnv struct {
	h             *admin.Handlers
	alloc         *mapsAllocations
	estA, estB    *store.Establishment
	owner, owner2 *auth.Principal
	viewer        *auth.Principal
}

func newMapsEnv(t *testing.T, db *store.DB) *mapsEnv {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2027, 5, 2, 9, 0, 0, 0, time.UTC)
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(now)
	alloc := &mapsAllocations{busy: map[string]bool{}}
	svc := mapping.NewService(mapping.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c), Allocations: alloc})
	user := func(org, est string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
	}
	return &mapsEnv{
		h:     &admin.Handlers{Mapping: svc, Log: slog.New(slog.DiscardHandler)},
		alloc: alloc, estA: estA, estB: estB,
		owner: user(orgA.ID, "", auth.RoleOwner), owner2: user(orgB.ID, "", auth.RoleOwner),
		viewer: user(orgA.ID, estA.ID, auth.RoleViewer),
	}
}

// mapsCall runs a handler with p authenticated; it returns status, headers
// and the raw body.
func mapsCall(t *testing.T, p *auth.Principal, method string, body any, fn func(w http.ResponseWriter, r *http.Request)) (int, http.Header, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, "/", &buf)
	if p != nil {
		r = r.WithContext(auth.WithPrincipal(r.Context(), p))
	}
	w := httptest.NewRecorder()
	fn(w, r)
	return w.Code, w.Header(), w.Body.Bytes()
}

func mapsDecode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

func mapsPtr[T any](v T) *T { return &v }

func TestMapsHTTP_DocumentFlow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newMapsEnv(t, db)
		a := e.estA.ID

		// Create the map: 201, ETag "1", empty document.
		mapIn := api.MapInput{
			Name: api.I18nText{"it": "Spiaggia"}, Width: 1200, Height: 800, GridSize: 10, SeaSide: "bottom",
			Background: &api.MapBackground{Type: "image", Url: mapsPtr("https://example.org/lido.png"), Opacity: mapsPtr(0.5)},
		}
		status, hdr, body := mapsCall(t, e.owner, http.MethodPost, mapIn, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreateMap(w, r, a, api.CreateMapParams{})
		})
		if status != 201 || hdr.Get("ETag") != `"1"` {
			t.Fatalf("create map: %d %s", status, body)
		}
		doc := mapsDecode[api.MapDocument](t, body)
		if doc.Map.Background == nil || *doc.Map.Background.Url != "https://example.org/lido.png" || len(doc.Spots) != 0 {
			t.Fatalf("created document: %s", body)
		}
		mapID := doc.Map.Id

		// Invalid map: field errors.
		status, _, body = mapsCall(t, e.owner, http.MethodPost, api.MapInput{Name: api.I18nText{}, Width: 0, Height: 10, GridSize: 1, SeaSide: "north"},
			func(w http.ResponseWriter, r *http.Request) { e.h.CreateMap(w, r, a, api.CreateMapParams{}) })
		if p := mapsDecode[api.Problem](t, body); status != 400 || p.Type != "validation_failed" || p.Errors == nil || len(*p.Errors) < 3 {
			t.Fatalf("invalid map: %d %s", status, body)
		}

		// Zone in `zone` mode: implicit pool.
		status, hdr, body = mapsCall(t, e.owner, http.MethodPost, api.ZoneInput{
			MapId: &mapID, Code: "row1", Name: api.I18nText{"it": "Prima fila"}, AssignmentMode: "zone", PriceCategory: "row1",
			OnlineQuota: &api.OnlineQuota{Type: "percent", Value: 50},
		}, func(w http.ResponseWriter, r *http.Request) { e.h.CreateZone(w, r, a, api.CreateZoneParams{}) })
		if status != 201 || hdr.Get("ETag") != `"1"` {
			t.Fatalf("create zone: %d %s", status, body)
		}
		zone := mapsDecode[api.Zone](t, body)
		if zone.PoolId == nil || !zone.OnlineVisible || zone.OnlineQuota.Value != 50 {
			t.Fatalf("zone: %s", body)
		}

		// Preview the generator, then apply it.
		gen := api.RowGeneratorRequest{
			ZoneId: zone.Id, Rows: 8, SpotsPerRow: 15, OriginX: mapsPtr(50.0), OriginY: mapsPtr(750.0), SpacingX: 70, SpacingY: 80,
			Capacity: &api.ComponentQuantities{"umbrella": 1, "sunbed": 2},
		}
		status, _, body = mapsCall(t, e.owner, http.MethodPost, gen, func(w http.ResponseWriter, r *http.Request) {
			e.h.GenerateMapRows(w, r, a, mapID, api.GenerateMapRowsParams{})
		})
		res := mapsDecode[api.RowGeneratorResult](t, body)
		if status != 200 || len(res.Spots) != 120 || res.Applied || res.Document != nil || res.Spots[0].Label != "A1" {
			t.Fatalf("preview: %d applied=%v", status, res.Applied)
		}
		gen.Apply = mapsPtr(true)
		status, _, body = mapsCall(t, e.owner, http.MethodPost, gen, func(w http.ResponseWriter, r *http.Request) {
			e.h.GenerateMapRows(w, r, a, mapID, api.GenerateMapRowsParams{})
		})
		if p := mapsDecode[api.Problem](t, body); status != 428 || p.Type != "precondition_required" {
			t.Fatalf("apply without If-Match: %d %s", status, body)
		}
		status, hdr, body = mapsCall(t, e.owner, http.MethodPost, gen, func(w http.ResponseWriter, r *http.Request) {
			e.h.GenerateMapRows(w, r, a, mapID, api.GenerateMapRowsParams{IfMatch: mapsPtr(`"1"`)})
		})
		res = mapsDecode[api.RowGeneratorResult](t, body)
		if status != 200 || !res.Applied || res.Document == nil || len(res.Document.Spots) != 120 || hdr.Get("ETag") != `"2"` {
			t.Fatalf("apply: %d %s", status, hdr.Get("ETag"))
		}

		// GET returns the document with ETag.
		status, hdr, body = mapsCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetMap(w, r, a, mapID) })
		doc = mapsDecode[api.MapDocument](t, body)
		if status != 200 || hdr.Get("ETag") != `"2"` || doc.Map.SpotCount != 120 || doc.Map.Version != 2 {
			t.Fatalf("get map: %d %s", status, hdr.Get("ETag"))
		}

		// Save: stale If-Match -> 412 with current_version.
		save := api.MapSaveRequest{Map: mapIn, Decorations: []api.DecorationInput{{Kind: "bar", X: 600, Y: 20, Shape: api.Shape{Type: "rect", W: 60, H: 30}, Label: &api.I18nText{"it": "Bar"}}}}
		for _, sp := range doc.Spots {
			save.Spots = append(save.Spots, api.SpotInput{
				Id: mapsPtr(sp.Id), ZoneId: sp.ZoneId, Label: sp.Label, RowLabel: mapsPtr(sp.RowLabel),
				Number: mapsPtr(sp.Number), Kind: sp.Kind, X: sp.X, Y: sp.Y, Shape: sp.Shape, Capacity: &sp.Capacity,
			})
		}
		status, _, body = mapsCall(t, e.owner, http.MethodPut, save, func(w http.ResponseWriter, r *http.Request) {
			e.h.SaveMap(w, r, a, mapID, api.SaveMapParams{IfMatch: `"1"`})
		})
		if p := mapsDecode[api.Problem](t, body); status != 412 || p.Type != "version_conflict" || p.CurrentVersion == nil || *p.CurrentVersion != 2 {
			t.Fatalf("stale save: %d %s", status, body)
		}
		status, _, body = mapsCall(t, e.owner, http.MethodPut, save, func(w http.ResponseWriter, r *http.Request) {
			e.h.SaveMap(w, r, a, mapID, api.SaveMapParams{IfMatch: ""})
		})
		if p := mapsDecode[api.Problem](t, body); status != 428 || p.Type != "precondition_required" {
			t.Fatalf("save without If-Match: %d %s", status, body)
		}

		// Deleting an allocated spot -> 409 spot_has_future_allocations with ids.
		busy := doc.Spots[0].Id
		e.alloc.busy[busy] = true
		withoutBusy := save
		withoutBusy.Spots = save.Spots[1:]
		status, _, body = mapsCall(t, e.owner, http.MethodPut, withoutBusy, func(w http.ResponseWriter, r *http.Request) {
			e.h.SaveMap(w, r, a, mapID, api.SaveMapParams{IfMatch: `"2"`})
		})
		if p := mapsDecode[api.Problem](t, body); status != 409 || p.Type != "spot_has_future_allocations" || p.SpotIds == nil || (*p.SpotIds)[0] != busy {
			t.Fatalf("delete allocated: %d %s", status, body)
		}
		// Deactivating it instead works: ETag "3", pool shrinks to 119.
		save.Spots[0].Active = mapsPtr(false)
		status, hdr, body = mapsCall(t, e.owner, http.MethodPut, save, func(w http.ResponseWriter, r *http.Request) {
			e.h.SaveMap(w, r, a, mapID, api.SaveMapParams{IfMatch: `W/"2"`})
		})
		doc = mapsDecode[api.MapDocument](t, body)
		if status != 200 || hdr.Get("ETag") != `"3"` || len(doc.Decorations) != 1 || (*doc.Decorations[0].Label)["it"] != "Bar" {
			t.Fatalf("save: %d %s", status, hdr.Get("ETag"))
		}
		status, hdr, body = mapsCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetPool(w, r, a, *zone.PoolId) })
		pool := mapsDecode[api.Pool](t, body)
		if status != 200 || !pool.SystemManaged || pool.Capacity != 119 || hdr.Get("ETag") == "" {
			t.Fatalf("system pool: %d %s", status, body)
		}
		status, _, body = mapsCall(t, e.owner, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.DeletePool(w, r, a, pool.Id, api.DeletePoolParams{})
		})
		if p := mapsDecode[api.Problem](t, body); status != 409 || p.Type != "conflict" {
			t.Fatalf("delete system pool: %d %s", status, body)
		}

		// Lists.
		status, _, body = mapsCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListMaps(w, r, a, api.ListMapsParams{})
		})
		if l := mapsDecode[api.MapList](t, body); status != 200 || len(l.Items) != 1 || l.Items[0].SpotCount != 120 {
			t.Fatalf("list maps: %d %s", status, body)
		}
		status, _, body = mapsCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListZones(w, r, a, api.ListZonesParams{MapId: &mapID})
		})
		if l := mapsDecode[api.ZoneList](t, body); status != 200 || len(l.Items) != 1 || l.Items[0].SpotCount != 119 {
			t.Fatalf("list zones: %d %s", status, body)
		}
		status, _, body = mapsCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListPools(w, r, a, api.ListPoolsParams{Limit: mapsPtr(0)})
		})
		if status != 400 {
			t.Fatalf("bad limit: %d %s", status, body)
		}

		// Viewer cannot mutate.
		status, _, _ = mapsCall(t, e.viewer, http.MethodPost, mapIn, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreateMap(w, r, a, api.CreateMapParams{})
		})
		if status != 403 {
			t.Fatalf("viewer create: %d", status)
		}
	})
}

func TestMapsHTTP_CrossTenantAndPools(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newMapsEnv(t, db)
		a, b := e.estA.ID, e.estB.ID
		status, _, body := mapsCall(t, e.owner, http.MethodPost, api.ZoneInput{
			Code: "beds", Name: api.I18nText{"it": "Lettini"}, AssignmentMode: "quantity", PriceCategory: "beds",
		}, func(w http.ResponseWriter, r *http.Request) { e.h.CreateZone(w, r, a, api.CreateZoneParams{}) })
		if status != 201 {
			t.Fatalf("zone: %d %s", status, body)
		}
		zone := mapsDecode[api.Zone](t, body)
		poolIn := api.PoolInput{
			ZoneId: &zone.Id, ComponentCode: "sunbed", Name: api.I18nText{"it": "Lettini"}, Capacity: 30,
			CapacityOverrides: &[]api.CapacityOverride{{From: calendar.MustParse("2027-08-01"), To: calendar.MustParse("2027-08-31"), Capacity: 50}},
		}
		status, hdr, body := mapsCall(t, e.owner, http.MethodPost, poolIn, func(w http.ResponseWriter, r *http.Request) {
			e.h.CreatePool(w, r, a, api.CreatePoolParams{})
		})
		if status != 201 || hdr.Get("ETag") != `"1"` {
			t.Fatalf("pool: %d %s", status, body)
		}
		pool := mapsDecode[api.Pool](t, body)
		poolIn.Capacity = 35
		status, hdr, body = mapsCall(t, e.owner, http.MethodPut, poolIn, func(w http.ResponseWriter, r *http.Request) {
			e.h.UpdatePool(w, r, a, pool.Id, api.UpdatePoolParams{IfMatch: mapsPtr(`"1"`)})
		})
		if status != 200 || hdr.Get("ETag") != `"2"` {
			t.Fatalf("update pool: %d %s", status, body)
		}
		status, _, body = mapsCall(t, e.owner, http.MethodPut, poolIn, func(w http.ResponseWriter, r *http.Request) {
			e.h.UpdatePool(w, r, a, pool.Id, api.UpdatePoolParams{IfMatch: mapsPtr("abc")})
		})
		if status != 400 {
			t.Fatalf("malformed If-Match: %d %s", status, body)
		}

		// Owner B: 404 everywhere on A's resources.
		for name, fn := range map[string]func(w http.ResponseWriter, r *http.Request){
			"get pool via A":  func(w http.ResponseWriter, r *http.Request) { e.h.GetPool(w, r, a, pool.Id) },
			"get pool via B":  func(w http.ResponseWriter, r *http.Request) { e.h.GetPool(w, r, b, pool.Id) },
			"get zone via B":  func(w http.ResponseWriter, r *http.Request) { e.h.GetZone(w, r, b, zone.Id) },
			"list zones of A": func(w http.ResponseWriter, r *http.Request) { e.h.ListZones(w, r, a, api.ListZonesParams{}) },
			"delete zone":     func(w http.ResponseWriter, r *http.Request) { e.h.DeleteZone(w, r, b, zone.Id, api.DeleteZoneParams{}) },
			"delete pool":     func(w http.ResponseWriter, r *http.Request) { e.h.DeletePool(w, r, b, pool.Id, api.DeletePoolParams{}) },
		} {
			status, _, body := mapsCall(t, e.owner2, http.MethodGet, nil, fn)
			if p := mapsDecode[api.Problem](t, body); status != 404 || p.Type != "not_found" {
				t.Fatalf("%s: %d %s", name, status, body)
			}
		}
		status, _, body = mapsCall(t, e.owner2, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListPools(w, r, b, api.ListPoolsParams{})
		})
		if l := mapsDecode[api.PoolList](t, body); status != 200 || len(l.Items) != 0 {
			t.Fatalf("pools of B: %d %s", status, body)
		}

		// Unauthenticated.
		status, _, _ = mapsCall(t, nil, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListMaps(w, r, a, api.ListMapsParams{})
		})
		if status != 401 {
			t.Fatalf("anonymous: %d", status)
		}

		// Delete with the right version.
		status, _, _ = mapsCall(t, e.owner, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.DeletePool(w, r, a, pool.Id, api.DeletePoolParams{IfMatch: mapsPtr(`"2"`)})
		})
		if status != 204 {
			t.Fatalf("delete pool: %d", status)
		}
	})
}
