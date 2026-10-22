//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/domain/team"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// stEnv runs the full HTTP stack (sessions, CSRF, idempotency, routing) with
// the establishment, season and team services, for the handlers in
// establishments_write.go, seasons.go and team.go.
type stEnv struct {
	t     *testing.T
	db    *store.DB
	srv   *httptest.Server
	orgA  *store.Organization
	estA  *store.Establishment
	estB  *store.Establishment
	users map[string]string // role name -> user id
}

var stFastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

const stPassword = "password-1234"

type stReadiness struct{ db *store.DB }

func (r stReadiness) Ping(ctx context.Context) error                  { return r.db.PingContext(ctx) }
func (r stReadiness) MigrationsAligned(context.Context) (bool, error) { return true, nil }

// newStEnv creates org A (owner, manager, cashier, viewer, all
// organization-wide, on establishment lido-a) and org B (owner "ownerB",
// establishment lido-b). Every user's password is stPassword.
func newStEnv(t *testing.T, db *store.DB) *stEnv {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Now().UTC())
	aw := audit.NewWriter(c)
	ob := event.NewOutbox(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &stFastArgon})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner A", Password: stPassword, OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
	if err := store.InsertEstablishment(ctx, db, estA); err != nil {
		t.Fatal(err)
	}
	_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	resB, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-b.it", Name: "Owner B", Password: stPassword, OrganizationName: "Org B"})
	if err != nil {
		t.Fatal(err)
	}
	e := &stEnv{t: t, db: db, orgA: res.Organization, estA: estA, estB: estB, users: map[string]string{"owner": res.User.ID, "ownerB": resB.User.ID}}
	hash, err := as.Hasher().Hash(stPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []auth.Role{auth.RoleManager, auth.RoleCashier, auth.RoleViewer} {
		u := &store.User{ID: store.NewID(), Email: string(role) + "@lido-a.it", PasswordHash: hash, Name: string(role), Locale: "it", Active: true, CreatedAt: c.Now(), UpdatedAt: c.Now()}
		if err := store.InsertUser(ctx, db, u); err != nil {
			t.Fatal(err)
		}
		m := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: res.Organization.ID, Role: string(role), CreatedAt: c.Now(), UpdatedAt: c.Now()}
		if err := store.InsertMembership(ctx, db, m); err != nil {
			t.Fatal(err)
		}
		e.users[string(role)] = u.ID
	}
	h := httpserver.New(httpserver.Deps{
		DB: db, Clock: c, Version: "test", Auth: as, Bus: event.NewBroker(nil, 16), Readiness: stReadiness{db},
		Establishments: establishment.NewService(db, establishment.WithWriteDeps(establishment.WriteDeps{Clock: c, Audit: aw, Outbox: ob})),
		Seasons:        season.NewService(season.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
		Team:           team.NewService(team.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Auth: as}),
	})
	e.srv = httptest.NewServer(h)
	t.Cleanup(e.srv.Close)
	return e
}

type stClient struct {
	t    *testing.T
	base string
	http *http.Client
}

// as logs in the user with the given email (role@lido-a.it, owner@lido-b.it).
func (e *stEnv) as(email string) *stClient {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &stClient{t: e.t, base: e.srv.URL, http: &http.Client{Jar: jar}}
	resp, body := c.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": email, "password": stPassword}, nil)
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("login %s: %d %v", email, resp.StatusCode, body)
	}
	return c
}

func (c *stClient) csrf() string {
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "arenile_csrf" {
			return ck.Value
		}
	}
	return ""
}

// do sends a JSON request; mutations carry the CSRF token unless headers
// set X-CSRF-Token explicitly (empty value = omit).
func (c *stClient) do(method, path string, body any, headers map[string]string) (*http.Response, map[string]any) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", c.csrf())
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "json") {
		if err := json.Unmarshal(raw, &out); err != nil {
			c.t.Fatalf("invalid json %q: %v", raw, err)
		}
	}
	return resp, out
}

func stExpect(t *testing.T, resp *http.Response, body map[string]any, status int, problemType string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d: %v", resp.StatusCode, status, body)
	}
	if problemType != "" {
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" || body["type"] != problemType {
			t.Fatalf("problem %q %v, want %s", ct, body, problemType)
		}
	}
}

func TestEstablishmentHTTP_CreateUpdateArchive(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newStEnv(t, db)
		owner, manager, viewer, ownerB := e.as("owner@lido-a.it"), e.as("manager@lido-a.it"), e.as("viewer@lido-a.it"), e.as("owner@lido-b.it")

		// Create: owners only.
		resp, body := manager.do(http.MethodPost, "/api/admin/v1/establishments", map[string]any{"slug": "lido-c", "name": "Lido C"}, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = owner.do(http.MethodPost, "/api/admin/v1/establishments", map[string]any{"slug": "assets", "name": ""}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		if errs, _ := body["errors"].([]any); len(errs) != 2 {
			t.Fatalf("field errors %v", body)
		}
		resp, body = owner.do(http.MethodPost, "/api/admin/v1/establishments", map[string]any{"slug": "lido-b", "name": "Taken"}, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodPost, "/api/admin/v1/establishments",
			map[string]any{"slug": "lido-c", "name": "Lido C", "enabled_locales": []string{"it", "en"}}, map[string]string{"Idempotency-Key": "k-create-1"})
		stExpect(t, resp, body, 201, "")
		if resp.Header.Get("ETag") != `"1"` || body["slug"] != "lido-c" || body["timezone"] != "Europe/Rome" || body["currency"] != "EUR" {
			t.Fatalf("created %v %v", resp.Header, body)
		}
		newID := body["id"].(string)
		// Org-wide members see it in their list immediately.
		_, body = manager.do(http.MethodGet, "/api/admin/v1/establishments", nil, nil)
		if !stListHas(body, newID) {
			t.Fatalf("manager list %v", body)
		}
		_, body = ownerB.do(http.MethodGet, "/api/admin/v1/establishments", nil, nil)
		if stListHas(body, newID) {
			t.Fatal("establishment leaked to org B")
		}

		// Update with If-Match.
		path := "/api/admin/v1/establishments/" + e.estA.ID
		patch := map[string]any{
			"name": "Lido A Mare", "allowed_embed_origins": []string{"https://www.lido-a.it/"},
			"theme":    map[string]any{"primary_color": "#0E6BA8", "font_family": "Inter", "radius": "md"},
			"settings": map[string]any{"pricing": map[string]any{"half_day_factor": "0.6", "rounding": "euro", "default_vat_rate_bp": 2200, "local_holidays": []string{"2027-06-24"}, "recurring_local_holidays": []string{"08-16"}}, "default_policy": map[string]any{"max_lead_time_days": 90}},
		}
		resp, body = manager.do(http.MethodPatch, path, patch, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"2"` || body["name"] != "Lido A Mare" {
			t.Fatalf("patched %v %v", resp.Header, body)
		}
		pricingOut := body["settings"].(map[string]any)["pricing"].(map[string]any)
		if pricingOut["half_day_factor"] != "0.6" || pricingOut["rounding"] != "euro" || len(pricingOut["recurring_local_holidays"].([]any)) != 1 {
			t.Fatalf("settings %v", body["settings"])
		}
		if origins := body["allowed_embed_origins"].([]any); origins[0] != "https://www.lido-a.it" {
			t.Fatalf("origins %v", origins)
		}
		resp, body = manager.do(http.MethodPatch, path, map[string]any{"name": "X"}, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 412, "version_conflict")
		if body["current_version"] != float64(2) {
			t.Fatalf("current version %v", body)
		}
		resp, body = manager.do(http.MethodPatch, path, map[string]any{"name": "X"}, map[string]string{"If-Match": `"abc"`})
		stExpect(t, resp, body, 400, "validation_failed")
		bad := map[string]any{
			"timezone": "Nowhere/City", "default_locale": "es", "allowed_embed_origins": []string{"http://x.it"},
			"theme": map[string]any{"primary_color": "blue"},
		}
		resp, body = manager.do(http.MethodPatch, path, bad, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		if errs, _ := body["errors"].([]any); len(errs) < 4 {
			t.Fatalf("field errors %v", body)
		}
		resp, body = manager.do(http.MethodPatch, path, map[string]any{"settings": map[string]any{"pricing": map[string]any{"half_day_factor": "abc", "rounding": "none", "default_vat_rate_bp": 0}, "default_policy": map[string]any{}}}, nil)
		stExpect(t, resp, body, 400, "validation_failed")

		// Permissions and tenancy.
		resp, body = viewer.do(http.MethodPatch, path, map[string]any{"name": "X"}, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = ownerB.do(http.MethodPatch, path, map[string]any{"name": "X"}, nil)
		stExpect(t, resp, body, 404, "not_found")
		resp, body = manager.do(http.MethodPatch, path, map[string]any{"archived": true}, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = owner.do(http.MethodPatch, path, map[string]any{"name": "X"}, map[string]string{"X-CSRF-Token": ""})
		stExpect(t, resp, body, 403, "csrf_failed")

		// Archive and restore (owner).
		resp, body = owner.do(http.MethodPatch, path, map[string]any{"archived": true}, nil)
		stExpect(t, resp, body, 200, "")
		if body["archived_at"] == nil {
			t.Fatalf("archived %v", body)
		}
		_, body = owner.do(http.MethodGet, "/api/admin/v1/establishments", nil, nil)
		if stListHas(body, e.estA.ID) {
			t.Fatal("archived establishment listed")
		}
		resp, body = owner.do(http.MethodPatch, path, map[string]any{"archived": false}, nil)
		stExpect(t, resp, body, 200, "")
		if body["archived_at"] != nil {
			t.Fatalf("restored %v", body)
		}
		resp, body = viewer.do(http.MethodGet, path, nil, nil)
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"4"` {
			t.Fatalf("etag %q", resp.Header.Get("ETag"))
		}
	})
}

func stListHas(body map[string]any, id string) bool {
	items, _ := body["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["id"] == id {
			return true
		}
	}
	return false
}

func newStJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}
