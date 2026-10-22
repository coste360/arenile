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

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/catalog"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type catalogEnv struct {
	h          *admin.Handlers
	estA, estB *store.Establishment
	owner      *auth.Principal
	viewer     *auth.Principal
}

func newCatalogEnv(t *testing.T, db *store.DB) *catalogEnv {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
	if err != nil {
		t.Fatal(err)
	}
	_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFake(now)
	svc := catalog.NewService(catalog.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	return &catalogEnv{
		h:     &admin.Handlers{Catalog: svc, Log: slog.New(slog.DiscardHandler)},
		estA:  estA,
		estB:  estB,
		owner: &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgA.ID, Role: auth.RoleOwner}}},
		viewer: &auth.Principal{
			Kind: auth.PrincipalUser, UserID: store.NewID(),
			Grants: []auth.Grant{{OrganizationID: orgA.ID, EstablishmentID: estA.ID, Role: auth.RoleViewer}},
		},
	}
}

// call runs handler fn with p authenticated and returns status, headers and
// the decoded JSON body (nil for empty bodies).
func call(t *testing.T, p *auth.Principal, method string, body any, fn func(w http.ResponseWriter, r *http.Request)) (int, http.Header, map[string]any) {
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
	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %q: %v", w.Body.String(), err)
		}
	}
	return w.Code, w.Header(), out
}

func strp(s string) *string { return &s }

func TestCatalogHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newCatalogEnv(t, db)
		a := e.estA.ID

		// Validation: field errors with JSON paths.
		status, _, body := call(t, e.owner, http.MethodPost, api.ComponentInput{Code: "Bad Code", Name: api.I18nText{"it": "x"}},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.CreateComponent(w, r, a, api.CreateComponentParams{})
			})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("validation: %d %v", status, body)
		}

		for _, code := range []string{"umbrella", "sunbed"} {
			status, hdr, body := call(t, e.owner, http.MethodPost, api.ComponentInput{Code: code, Name: api.I18nText{"it": code}},
				func(w http.ResponseWriter, r *http.Request) {
					e.h.CreateComponent(w, r, a, api.CreateComponentParams{})
				})
			if status != 201 || hdr.Get("ETag") != `"1"` || body["code"] != code {
				t.Fatalf("create component: %d %v %v", status, hdr, body)
			}
		}
		status, _, body = call(t, e.owner, http.MethodPost, api.ComponentInput{Code: "sunbed", Name: api.I18nText{"it": "x"}},
			func(w http.ResponseWriter, r *http.Request) {
				e.h.CreateComponent(w, r, a, api.CreateComponentParams{})
			})
		if status != 409 || body["type"] != "conflict" {
			t.Fatalf("duplicate: %d %v", status, body)
		}

		packIn := api.PackInput{
			Code: "std", Name: api.I18nText{"it": "Standard"}, Composition: api.ComponentQuantities{"umbrella": 1, "sunbed": 2},
			AllowedFormulas: []api.Formula{"day", "season"}, Channels: []api.Channel{"desk", "online"},
		}
		status, hdr, body := call(t, e.owner, http.MethodPost, packIn,
			func(w http.ResponseWriter, r *http.Request) { e.h.CreatePack(w, r, a, api.CreatePackParams{}) })
		if status != 201 || hdr.Get("ETag") != `"1"` || body["active"] != true || body["max_per_booking"].(float64) != 1 {
			t.Fatalf("create pack: %d %v", status, body)
		}
		if _, ok := body["allowed_zone_ids"]; ok {
			t.Fatalf("absent allowed_zone_ids must be omitted: %v", body)
		}
		packID := body["id"].(string)

		status, hdr, body = call(t, e.viewer, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.GetPack(w, r, a, packID) })
		if status != 200 || hdr.Get("ETag") != `"1"` || body["composition"].(map[string]any)["sunbed"].(float64) != 2 {
			t.Fatalf("get pack: %d %v", status, body)
		}

		// Stale If-Match: 412 with current_version.
		packIn.Name = api.I18nText{"it": "Standard 2"}
		status, _, body = call(t, e.owner, http.MethodPut, packIn,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.UpdatePack(w, r, a, packID, api.UpdatePackParams{IfMatch: strp(`"5"`)})
			})
		if status != 412 || body["type"] != "version_conflict" || body["current_version"].(float64) != 1 {
			t.Fatalf("stale update: %d %v", status, body)
		}
		status, hdr, body = call(t, e.owner, http.MethodPut, packIn,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.UpdatePack(w, r, a, packID, api.UpdatePackParams{IfMatch: strp(`"1"`)})
			})
		if status != 200 || hdr.Get("ETag") != `"2"` || body["name"].(map[string]any)["it"] != "Standard 2" {
			t.Fatalf("update pack: %d %v", status, body)
		}
		status, _, body = call(t, e.owner, http.MethodPut, packIn,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.UpdatePack(w, r, a, packID, api.UpdatePackParams{IfMatch: strp("garbage")})
			})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("malformed if-match: %d %v", status, body)
		}

		// Extras.
		extraIn := api.ExtraInput{
			Code: "extra-bed", Name: api.I18nText{"it": "Lettino extra"}, PricingUnit: "per_day", AttachTo: "line",
			Channels: []api.Channel{"desk"}, ComponentCode: strp("sunbed"), AllowedPackIds: &[]string{packID},
		}
		status, _, body = call(t, e.owner, http.MethodPost, extraIn,
			func(w http.ResponseWriter, r *http.Request) { e.h.CreateExtra(w, r, a, api.CreateExtraParams{}) })
		if status != 201 || body["allowed_pack_ids"].([]any)[0] != packID {
			t.Fatalf("create extra: %d %v", status, body)
		}
		extraID := body["id"].(string)

		// Lists.
		status, _, body = call(t, e.viewer, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.ListComponents(w, r, a, api.ListComponentsParams{Limit: func() *int { n := 1; return &n }()})
			})
		if status != 200 || len(body["items"].([]any)) != 1 || body["next_cursor"] == nil {
			t.Fatalf("list components: %d %v", status, body)
		}
		status, _, body = call(t, e.viewer, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.ListComponents(w, r, a, api.ListComponentsParams{Cursor: strp("!!")})
			})
		if status != 400 {
			t.Fatalf("bad cursor: %d %v", status, body)
		}
		status, _, body = call(t, e.viewer, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.ListExtras(w, r, a, api.ListExtrasParams{}) })
		if status != 200 || len(body["items"].([]any)) != 1 {
			t.Fatalf("list extras: %d %v", status, body)
		}

		// Permissions and isolation.
		status, _, body = call(t, e.viewer, http.MethodPost, extraIn,
			func(w http.ResponseWriter, r *http.Request) { e.h.CreateExtra(w, r, a, api.CreateExtraParams{}) })
		if status != 403 || body["type"] != "forbidden" {
			t.Fatalf("viewer create: %d %v", status, body)
		}
		status, _, _ = call(t, nil, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.ListPacks(w, r, a, api.ListPacksParams{}) })
		if status != 401 {
			t.Fatalf("anonymous: %d", status)
		}
		status, _, body = call(t, e.owner, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.GetPack(w, r, e.estB.ID, packID) })
		if status != 404 || body["type"] != "not_found" {
			t.Fatalf("foreign establishment: %d %v", status, body)
		}

		// Delete: a pack used by an extra is archived (204), then the
		// unreferenced extra is deleted.
		status, _, _ = call(t, e.owner, http.MethodDelete, nil,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.DeletePack(w, r, a, packID, api.DeletePackParams{IfMatch: strp(`"2"`)})
			})
		if status != 204 {
			t.Fatalf("delete pack: %d", status)
		}
		status, hdr, body = call(t, e.owner, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.GetPack(w, r, a, packID) })
		if status != 200 || body["active"] != false || hdr.Get("ETag") != `"3"` {
			t.Fatalf("archived pack: %d %v", status, body)
		}
		status, _, _ = call(t, e.owner, http.MethodDelete, nil,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.DeleteExtra(w, r, a, extraID, api.DeleteExtraParams{})
			})
		if status != 204 {
			t.Fatalf("delete extra: %d", status)
		}
		status, _, _ = call(t, e.owner, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.GetExtra(w, r, a, extraID) })
		if status != 404 {
			t.Fatalf("deleted extra: %d", status)
		}
		// The component is still used by the archived pack.
		status, _, body = call(t, e.owner, http.MethodGet, nil,
			func(w http.ResponseWriter, r *http.Request) { e.h.ListComponents(w, r, a, api.ListComponentsParams{}) })
		if status != 200 {
			t.Fatal(status)
		}
		compID := body["items"].([]any)[0].(map[string]any)["id"].(string)
		status, _, body = call(t, e.owner, http.MethodDelete, nil,
			func(w http.ResponseWriter, r *http.Request) {
				e.h.DeleteComponent(w, r, a, compID, api.DeleteComponentParams{})
			})
		if status != 409 || body["type"] != "conflict" {
			t.Fatalf("delete used component: %d %v", status, body)
		}
	})
}
