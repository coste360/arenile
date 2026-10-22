//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// pricingServer serves the generated router with the admin handlers; the
// principal comes from the X-Test-Role header (no session machinery).
type unimplemented struct{ api.Unimplemented }

type pricingServer struct {
	*admin.Handlers
	unimplemented
}

type pricingEnv struct {
	t      *testing.T
	srv    *httptest.Server
	est    *store.Establishment
	season *store.Season
	front  *store.Zone
	base   *store.Pack
	roles  map[string]*auth.Principal
}

func newPricingEnv(t *testing.T, db *store.DB) *pricingEnv {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC))
	deps := pricingconfig.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}
	h := &admin.Handlers{Pricing: pricingconfig.NewService(deps), Policies: pricingconfig.NewPolicyService(deps)}
	now := c.Now()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-http", "Lido HTTP", now)
	if err != nil {
		t.Fatal(err)
	}
	orgB, _, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-http-b", "Lido B", now)
	if err != nil {
		t.Fatal(err)
	}
	e := &pricingEnv{t: t, est: est, roles: map[string]*auth.Principal{}}
	for _, r := range []auth.Role{auth.RoleOwner, auth.RoleManager, auth.RoleCashier, auth.RoleViewer} {
		e.roles[string(r)] = &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, Role: r}}}
	}
	e.roles["outsider"] = &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgB.ID, Role: auth.RoleOwner}}}
	insert := func(m any) {
		if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
			t.Fatalf("insert %T: %v", m, err)
		}
	}
	d := calendar.MustParse
	e.season = &store.Season{
		ID: store.NewID(), EstablishmentID: est.ID, Year: 2027, OpensOn: d("2027-06-01"), ClosesOn: d("2027-09-15"),
		Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	insert(e.season)
	insert(&store.RatePeriod{
		ID: store.NewID(), EstablishmentID: est.ID, SeasonID: e.season.ID, Code: "high", Name: store.I18nText{"it": "Alta"},
		Ranges: []store.DateRange{{From: d("2027-08-01"), To: d("2027-08-31")}}, Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	e.front = &store.Zone{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "row1", Name: store.I18nText{"it": "Fila 1"}, AssignmentMode: "seat",
		PriceCategory: "front", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	insert(e.front)
	e.base = &store.Pack{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"}, Description: store.I18nText{},
		Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"}, Channels: []string{"desk"}, MaxPerBooking: 2, Active: true,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	insert(e.base)

	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := reqctx.With(r.Context(), &reqctx.Info{ClientIP: "192.0.2.1", RequestID: "test"})
			if p := e.roles[r.Header.Get("X-Test-Role")]; p != nil {
				ctx = auth.WithPrincipal(ctx, p)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	e.srv = httptest.NewServer(inject(api.HandlerWithOptions(&pricingServer{Handlers: h}, api.ChiServerOptions{})))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *pricingEnv) do(role, method, path string, body any, headers map[string]string) (*http.Response, map[string]any) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+"/api/admin/v1/establishments/"+e.est.ID+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Role", role)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp, out
}

func expectStatus(t *testing.T, resp *http.Response, body map[string]any, status int, problemType string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d: %v", resp.StatusCode, status, body)
	}
	if problemType != "" && body["type"] != problemType {
		t.Fatalf("problem type %v, want %s", body["type"], problemType)
	}
}

func TestPricingHTTP_RulesToolsAndErrors(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newPricingEnv(t, db)
		rule := map[string]any{
			"season_id": e.season.ID, "name": "Alta", "target_type": "pack", "stage": "base",
			"conditions": map[string]any{"rate_periods": []string{"high"}},
			"action":     map[string]any{"type": "set", "amount": 3500},
		}
		resp, body := e.do("cashier", http.MethodPost, "/price-rules", rule, nil)
		expectStatus(t, resp, body, 403, "forbidden")
		resp, body = e.do("manager", http.MethodPost, "/price-rules", rule, nil)
		expectStatus(t, resp, body, 201, "")
		if resp.Header.Get("ETag") != `"1"` || body["seq"].(float64) != 1 || body["active"] != true {
			t.Fatalf("created %v etag %s", body, resp.Header.Get("ETag"))
		}
		id := body["id"].(string)
		mod := map[string]any{
			"season_id": e.season.ID, "name": "Fila 1", "target_type": "pack", "stage": "modifier", "stackable": true,
			"conditions": map[string]any{"price_categories": []string{"front"}},
			"action":     map[string]any{"type": "multiply", "factor": "1.2"},
		}
		resp, body = e.do("manager", http.MethodPost, "/price-rules", mod, nil)
		expectStatus(t, resp, body, 201, "")
		if body["action"].(map[string]any)["factor"] != "1.2" {
			t.Fatalf("decimal factor as string: %v", body["action"])
		}
		bad := map[string]any{
			"season_id": e.season.ID, "name": "x", "target_type": "pack", "stage": "base",
			"conditions": map[string]any{}, "action": map[string]any{"type": "multiply", "factor": "abc"},
		}
		resp, body = e.do("manager", http.MethodPost, "/price-rules", bad, nil)
		expectStatus(t, resp, body, 400, "validation_failed")

		resp, body = e.do("viewer", http.MethodGet, "/price-rules/"+id, nil, nil)
		expectStatus(t, resp, body, 200, "")
		rule["name"] = "Alta v2"
		resp, body = e.do("manager", http.MethodPut, "/price-rules/"+id, rule, map[string]string{"If-Match": `"1"`})
		expectStatus(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"2"` {
			t.Fatalf("etag after update %s", resp.Header.Get("ETag"))
		}
		resp, body = e.do("manager", http.MethodPut, "/price-rules/"+id, rule, map[string]string{"If-Match": `"1"`})
		expectStatus(t, resp, body, 412, "version_conflict")
		if body["current_version"].(float64) != 2 {
			t.Fatalf("current_version %v", body)
		}
		resp, body = e.do("viewer", http.MethodGet, "/price-rules?season_id="+e.season.ID+"&limit=1", nil, nil)
		expectStatus(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 1 || body["next_cursor"] == nil {
			t.Fatalf("list page %v", body)
		}
		resp, body = e.do("outsider", http.MethodGet, "/price-rules", nil, nil)
		expectStatus(t, resp, body, 404, "not_found")

		// Matrix and simulator agree: 3500 x 1.2 = 4200.
		resp, body = e.do("viewer", http.MethodGet, "/pricing/matrix?season_id="+e.season.ID, nil, nil)
		expectStatus(t, resp, body, 403, "forbidden")
		resp, body = e.do("manager", http.MethodGet, "/pricing/matrix?season_id="+e.season.ID, nil, nil)
		expectStatus(t, resp, body, 200, "")
		cells := body["cells"].([]any)
		cell := cells[0].(map[string]any)
		if len(cells) != 1 || cell["price"].(float64) != 4200 || cell["date"] != "2027-08-02" {
			t.Fatalf("matrix %v", body)
		}
		sim := map[string]any{
			"channel": "online", "starts_on": "2027-08-02", "ends_on": "2027-08-02", "slots": "FULL", "formula": "day",
			"today": "2027-07-30", "lines": []any{map[string]any{"pack_id": e.base.ID, "zone_id": e.front.ID}},
		}
		resp, body = e.do("manager", http.MethodPost, "/pricing/simulate", sim, nil)
		expectStatus(t, resp, body, 200, "")
		bd := body["breakdown"].(map[string]any)
		if bd["total"].(float64) != 4200 || body["deposit_due"].(float64) != 4200 {
			t.Fatalf("simulate %v", body)
		}
		day := bd["lines"].([]any)[0].(map[string]any)["days"].([]any)[0].(map[string]any)
		if len(day["applied"].([]any)) != 2 {
			t.Fatalf("applied steps %v", day)
		}
		if pol := body["policy"].(map[string]any); pol["policy_id"] != nil || len(pol["violations"].([]any)) != 0 {
			t.Fatalf("policy %v", pol)
		}
		sim["starts_on"], sim["ends_on"] = "2027-07-02", "2027-07-02"
		resp, body = e.do("manager", http.MethodPost, "/pricing/simulate", sim, nil)
		expectStatus(t, resp, body, 422, "no_price")

		resp, body = e.do("manager", http.MethodGet, "/pricing/coverage?season_id="+e.season.ID+"&channels=desk", nil, nil)
		expectStatus(t, resp, body, 200, "")
		gaps := body["gaps"].([]any)
		if body["complete"] != false || len(gaps) != 2 {
			t.Fatalf("coverage %v", body)
		}

		// Promo codes: case-insensitive conflict.
		promo := map[string]any{"code": "ESTATE", "action": map[string]any{"type": "percent_off", "percent": "10"}}
		resp, body = e.do("manager", http.MethodPost, "/promo-codes", promo, nil)
		expectStatus(t, resp, body, 201, "")
		promo["code"] = "estate"
		resp, body = e.do("manager", http.MethodPost, "/promo-codes", promo, nil)
		expectStatus(t, resp, body, 409, "conflict")

		// Policies.
		pol := map[string]any{
			"season_id": e.season.ID, "name": "Acconto agosto", "priority": 3,
			"scope": map[string]any{"rate_periods": []string{"high"}},
			"rules": map[string]any{"payment": map[string]any{"mode": "deposit", "deposit": map[string]any{"type": "percent", "value": 50}}},
		}
		resp, body = e.do("manager", http.MethodPost, "/policies", pol, nil)
		expectStatus(t, resp, body, 201, "")
		polID := body["id"].(string)
		sim["starts_on"], sim["ends_on"] = "2027-08-02", "2027-08-02"
		resp, body = e.do("manager", http.MethodPost, "/pricing/simulate", sim, nil)
		expectStatus(t, resp, body, 200, "")
		if body["deposit_due"].(float64) != 2100 || body["policy"].(map[string]any)["policy_id"] != polID {
			t.Fatalf("policy simulate %v", body)
		}
		resp, body = e.do("manager", http.MethodDelete, "/policies/"+polID, nil, map[string]string{"If-Match": "x"})
		expectStatus(t, resp, body, 400, "validation_failed")
		resp, _ = e.do("manager", http.MethodDelete, "/policies/"+polID, nil, map[string]string{"If-Match": `"1"`})
		if resp.StatusCode != 204 {
			t.Fatalf("delete policy %d", resp.StatusCode)
		}

		// Season copy.
		resp, body = e.do("manager", http.MethodPost, "/seasons/"+e.season.ID+"/copy", map[string]any{"target_year": 2028}, nil)
		expectStatus(t, resp, body, 201, "")
		if body["price_rules"].(float64) != 2 || body["rate_periods"].(float64) != 1 || body["season"].(map[string]any)["year"].(float64) != 2028 {
			t.Fatalf("copy %v", body)
		}
		resp, body = e.do("manager", http.MethodPost, "/seasons/"+e.season.ID+"/copy", map[string]any{"target_year": 2028}, nil)
		expectStatus(t, resp, body, 409, "conflict")

		resp, _ = e.do("manager", http.MethodDelete, "/price-rules/"+id, nil, map[string]string{"If-Match": `"2"`})
		if resp.StatusCode != 204 {
			t.Fatalf("delete rule %d", resp.StatusCode)
		}
	})
}
