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

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type invEnv struct {
	h       *admin.Handlers
	est     *store.Establishment
	estB    *store.Establishment
	cashier *auth.Principal
	viewer  *auth.Principal
	ownerB  *auth.Principal
	spots   []store.Spot
	poolID  string
	mapID   string
}

func newInvEnv(t *testing.T, db *store.DB) *invEnv {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2027, 7, 1, 8, 0, 0, 0, time.UTC)
	chk := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	orgA, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-http-inv", "Lido", now)
	chk(err)
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-http-inv-b", "Lido B", now)
	chk(err)
	es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
	chk(err)
	user := func(name, org string, role auth.Role) *auth.Principal {
		u := &store.User{ID: store.NewID(), Email: name + "@example.it", PasswordHash: "x", Name: name, Locale: "it", Active: true, CreatedAt: now, UpdatedAt: now}
		chk(store.InsertUser(ctx, db, u))
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: u.ID, Grants: []auth.Grant{{OrganizationID: org, Role: role}}}
	}
	chk(store.InsertSeason(ctx, db, es, &store.Season{ID: store.NewID(), Year: 2027, OpensOn: calendar.MustParse("2027-05-15"), ClosesOn: calendar.MustParse("2027-09-30"), Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now}))
	m := &store.Map{ID: store.NewID(), Name: store.I18nText{"it": "Spiaggia"}, Width: 100, Height: 60, GridSize: 1, SeaSide: "bottom", Version: 1, CreatedAt: now, UpdatedAt: now}
	chk(store.InsertMap(ctx, db, es, m))
	z := &store.Zone{ID: store.NewID(), MapID: &m.ID, Code: "row1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", OnlineVisible: true, PriceCategory: "row1", Version: 1, CreatedAt: now, UpdatedAt: now}
	chk(store.InsertZone(ctx, db, es, z))
	spots := []store.Spot{}
	for _, l := range []string{"A1", "A2"} {
		spots = append(spots, store.Spot{
			ID: store.NewID(), MapID: m.ID, ZoneID: z.ID, Label: l, RowLabel: "A", Number: len(spots) + 1, Kind: "umbrella",
			Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
		})
	}
	chk(store.InsertSpots(ctx, db, es, spots))
	pool := &store.Pool{ID: store.NewID(), ComponentCode: "sunbed", Name: store.I18nText{"it": "Lettini"}, Capacity: 2, Version: 1, CreatedAt: now, UpdatedAt: now}
	chk(store.InsertPool(ctx, db, es, pool))
	c := clock.NewFake(now)
	svc := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	return &invEnv{
		h: &admin.Handlers{Inventory: svc, Log: slog.New(slog.DiscardHandler)}, est: est, estB: estB,
		cashier: user("cashier", orgA.ID, auth.RoleCashier), viewer: user("viewer", orgA.ID, auth.RoleViewer), ownerB: user("ownerb", orgB.ID, auth.RoleOwner),
		spots: spots, poolID: pool.ID, mapID: m.ID,
	}
}

func invCall(t *testing.T, p *auth.Principal, method string, body any, fn func(w http.ResponseWriter, r *http.Request)) (int, []byte) {
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
	return w.Code, w.Body.Bytes()
}

func invDecode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

func invProblemType(t *testing.T, b []byte) string {
	t.Helper()
	p := invDecode[api.Problem](t, b)
	return string(p.Type)
}

func TestAvailabilityHTTP_HoldsBlocksAndViews(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newInvEnv(t, db)
		a := e.est.ID
		d := calendar.MustParse
		qty := 2
		hold := api.HoldCreate{
			StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: "FULL",
			Items: []api.HoldItem{{SpotId: &e.spots[0].ID}, {PoolId: &e.poolID, Quantity: &qty}},
		}
		status, body := invCall(t, e.cashier, http.MethodPost, hold, func(w http.ResponseWriter, r *http.Request) { e.h.CreateHold(w, r, a, api.CreateHoldParams{}) })
		if status != http.StatusCreated {
			t.Fatalf("create hold: %d %s", status, body)
		}
		h := invDecode[api.Hold](t, body)
		if h.Status != api.HoldStatusActive || len(h.Items) != 2 || h.Channel != "desk" {
			t.Fatalf("hold = %+v", h)
		}
		// the same spot again: 409 spot_unavailable with conflicts
		status, body = invCall(t, e.cashier, http.MethodPost, api.HoldCreate{
			StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: "AM",
			Items: []api.HoldItem{{SpotId: &e.spots[0].ID}},
		}, func(w http.ResponseWriter, r *http.Request) { e.h.CreateHold(w, r, a, api.CreateHoldParams{}) })
		if status != http.StatusConflict || invProblemType(t, body) != "spot_unavailable" {
			t.Fatalf("conflict: %d %s", status, body)
		}
		if p := invDecode[api.Problem](t, body); p.Conflicts == nil || len(*p.Conflicts) != 1 || *(*p.Conflicts)[0].SpotId != e.spots[0].ID {
			t.Fatalf("conflicts = %s", body)
		}
		// pool exhausted
		one := 1
		status, body = invCall(t, e.cashier, http.MethodPost, api.HoldCreate{
			StartsOn: d("2027-07-02"), EndsOn: d("2027-07-02"), Slots: "PM",
			Items: []api.HoldItem{{PoolId: &e.poolID, Quantity: &one}},
		}, func(w http.ResponseWriter, r *http.Request) { e.h.CreateHold(w, r, a, api.CreateHoldParams{}) })
		if status != http.StatusConflict || invProblemType(t, body) != "pool_exhausted" {
			t.Fatalf("pool: %d %s", status, body)
		}
		// viewer cannot hold, can read
		status, _ = invCall(t, e.viewer, http.MethodPost, hold, func(w http.ResponseWriter, r *http.Request) { e.h.CreateHold(w, r, a, api.CreateHoldParams{}) })
		if status != http.StatusForbidden {
			t.Fatalf("viewer hold: %d", status)
		}
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetHold(w, r, a, h.Id) })
		if status != http.StatusOK {
			t.Fatalf("get hold: %d %s", status, body)
		}
		status, _ = invCall(t, e.ownerB, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { e.h.GetHold(w, r, a, h.Id) })
		if status != http.StatusNotFound {
			t.Fatalf("cross tenant: %d", status)
		}
		// availability: A1 unavailable, pool 0
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetAvailability(w, r, a, api.GetAvailabilityParams{From: d("2027-07-02"), To: d("2027-07-02")})
		})
		if status != http.StatusOK {
			t.Fatalf("availability: %d %s", status, body)
		}
		av := invDecode[api.AvailabilityResult](t, body)
		if len(av.Zones) != 1 || av.Zones[0].Available != 1 || (*av.Zones[0].Spots)[0].Status != api.SpotAvailabilityStatusUnavailable || av.Pools[0].Available != 0 {
			t.Fatalf("availability = %s", body)
		}
		// operations day and planning
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetOperationsDay(w, r, a, api.GetOperationsDayParams{Date: d("2027-07-02"), MapId: &e.mapID})
		})
		od := invDecode[api.OperationsDay](t, body)
		if status != http.StatusOK || len(od.Spots) != 2 || od.Spots[0].Status != api.OperationalSpotStatusHeld || od.Spots[0].HoldId == nil || od.Totals.Occupied != 1 {
			t.Fatalf("operations day: %d %s", status, body)
		}
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetPlanning(w, r, a, api.GetPlanningParams{From: d("2027-07-01"), To: d("2027-07-03")})
		})
		pl := invDecode[api.Planning](t, body)
		if status != http.StatusOK || len(pl.Rows) != 1 || pl.Entries[0].Kind != api.PlanningEntryKindHold || pl.Rows[0].Segments[0].From != 1 {
			t.Fatalf("planning: %d %s", status, body)
		}
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.GetPlanning(w, r, a, api.GetPlanningParams{From: d("2027-07-01"), To: d("2027-09-30")})
		})
		if status != http.StatusBadRequest || invProblemType(t, body) != "validation_failed" {
			t.Fatalf("planning range: %d %s", status, body)
		}
		// release (idempotent)
		for i := 0; i < 2; i++ {
			status, body = invCall(t, e.cashier, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) { e.h.ReleaseHold(w, r, a, h.Id, api.ReleaseHoldParams{}) })
			if status != http.StatusNoContent {
				t.Fatalf("release %d: %d %s", i, status, body)
			}
		}
		// blocks: cashier runs operations
		reason := "manutenzione"
		status, body = invCall(t, e.cashier, http.MethodPost, api.SpotBlockCreate{SpotIds: []string{e.spots[1].ID}, StartsOn: d("2027-07-02"), EndsOn: d("2027-07-03"), Slots: "FULL", Reason: &reason},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.CreateSpotBlock(w, r, a, api.CreateSpotBlockParams{})
			})
		if status != http.StatusCreated {
			t.Fatalf("block: %d %s", status, body)
		}
		bl := invDecode[api.SpotBlockList](t, body)
		status, body = invCall(t, e.viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.ListSpotBlocks(w, r, a, api.ListSpotBlocksParams{SpotId: &e.spots[1].ID})
		})
		if list := invDecode[api.SpotBlockList](t, body); status != http.StatusOK || len(list.Items) != 1 || list.Items[0].Reason != reason {
			t.Fatalf("list blocks: %d %s", status, body)
		}
		status, _ = invCall(t, e.viewer, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.DeleteSpotBlock(w, r, a, bl.Items[0].Id, api.DeleteSpotBlockParams{})
		})
		if status != http.StatusForbidden {
			t.Fatalf("viewer delete block: %d", status)
		}
		status, body = invCall(t, e.cashier, http.MethodDelete, nil, func(w http.ResponseWriter, r *http.Request) {
			e.h.DeleteSpotBlock(w, r, a, bl.Items[0].Id, api.DeleteSpotBlockParams{})
		})
		if status != http.StatusNoContent {
			t.Fatalf("delete block: %d %s", status, body)
		}
		// validation
		status, body = invCall(t, e.cashier, http.MethodPost, api.HoldCreate{StartsOn: d("2027-07-03"), EndsOn: d("2027-07-02"), Slots: "FULL"},
			func(w http.ResponseWriter, r *http.Request) { e.h.CreateHold(w, r, a, api.CreateHoldParams{}) })
		if status != http.StatusBadRequest || invProblemType(t, body) != "validation_failed" {
			t.Fatalf("validation: %d %s", status, body)
		}
	})
}
