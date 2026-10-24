package httpserver_test

import (
	"bufio"
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

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/importer"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/refund"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/domain/testmode"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/domain/webhook"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var fastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

type readiness struct{ db *store.DB }

func (r readiness) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }
func (r readiness) MigrationsAligned(ctx context.Context) (bool, error) {
	m, err := r.db.Migrator()
	if err != nil {
		return false, err
	}
	return m.Aligned(ctx)
}

type env struct {
	t      *testing.T
	db     *store.DB
	h      http.Handler
	srv    *httptest.Server
	auth   *auth.Service
	broker *event.Broker
	clock  *clock.Fake
	orgA   *store.Organization
	estA   *store.Establishment
	estB   *store.Establishment
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Now().UTC())
	aw := audit.NewWriter(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &fastArgon})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner A", Password: "password-a-123", OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
	estA.BookingEnabled = true
	estA.EnabledLocales = []string{"it", "en"}
	estA.AllowedEmbedOrigins = []string{"https://www.lido-a.it"}
	estA.ContactEmail = "info@lido-a.it"
	estA.Theme = store.Theme{PrimaryColor: "#0E6BA8"}
	if err := store.InsertEstablishment(ctx, db, estA); err != nil {
		t.Fatal(err)
	}
	_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	broker := event.NewBroker(nil, 16)
	ob := event.NewOutbox(c)
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{7}, 32))
	h := httpserver.New(httpserver.Deps{
		DB: db, Clock: c, Version: "test", Auth: as, Establishments: establishment.NewService(db),
		Bus: broker, Readiness: readiness{db}, SecureCookies: false, SSEHeartbeat: 50 * time.Millisecond,
		// M5B services with the database only: enough for the routing and
		// the tenancy checks (INV-26) exercised here.
		Rollover: rollover.NewService(rollover.Deps{DB: db, Clock: c}),
		Renewals: renewal.NewService(renewal.Deps{DB: db, Clock: c}),
		Waitlist: waitlist.NewService(waitlist.Deps{DB: db, Clock: c}),
		Credits:  credit.NewService(credit.Deps{DB: db, Clock: c}),
		// Deferred features (ADR 0019), database only as above.
		Cash:     cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
		Passes:   pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
		Notify:   notify.NewService(notify.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob}),
		Reports:  report.NewService(report.Deps{DB: db, Clock: c}),
		Webhooks: webhook.NewService(webhook.Deps{DB: db, Clock: c, Audit: aw, Keys: kr}),
		TestMode: testmode.NewService(testmode.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Keys: kr}),
		Refunds:  refund.NewService(refund.Deps{DB: db, Clock: c, Audit: aw}),
		Imports:  importer.NewService(importer.Deps{DB: db, Clock: c}),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, db: db, h: h, srv: srv, auth: as, broker: broker, clock: c, orgA: res.Organization, estA: estA, estB: estB}
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: e.t, base: e.srv.URL, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *client) csrf() string {
	u, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(u) {
		if ck.Name == "arenile_csrf" {
			return ck.Value
		}
	}
	return ""
}

func (c *client) do(method, path string, body any, headers map[string]string) (*http.Response, map[string]any) {
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
	for k, v := range headers {
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

func (c *client) login(email, password string) (*http.Response, map[string]any) {
	return c.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": email, "password": password}, nil)
}

func assertProblem(t *testing.T, resp *http.Response, body map[string]any, status int, typ string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d (%v)", resp.StatusCode, status, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type %q", ct)
	}
	if body["type"] != typ || int(body["status"].(float64)) != status || body["title"] == "" || body["request_id"] == "" {
		t.Fatalf("problem body %v", body)
	}
}

// TestHTTP_LoginOverTLS_SecureCookies: SecureCookies is false in this
// environment (plain-HTTP development, desktop loopback profile), yet a
// login that arrives over a TLS listener (desktop LAN mode) sets the
// session and CSRF cookies with Secure, so a browser never sends them in
// clear to the same host.
func TestHTTP_LoginOverTLS_SecureCookies(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	srv := httptest.NewTLSServer(e.h)
	t.Cleanup(srv.Close)
	body, _ := json.Marshal(map[string]string{"email": "owner@lido-a.it", "password": "password-a-123"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/admin/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login over tls: %d", resp.StatusCode)
	}
	names := map[string]bool{}
	for _, c := range resp.Cookies() {
		names[c.Name] = true
		if !c.Secure {
			t.Errorf("cookie %s without Secure over TLS", c.Name)
		}
	}
	if !names["arenile_session"] || !names["arenile_csrf"] {
		t.Fatalf("cookies: %v", names)
	}
	// Plain HTTP keeps the configured behaviour (no Secure).
	c := e.client()
	plain, _ := c.login("owner@lido-a.it", "password-a-123")
	for _, ck := range plain.Cookies() {
		if ck.Secure {
			t.Errorf("cookie %s Secure over plain http with SecureCookies=false", ck.Name)
		}
	}
}

func TestHTTP_LoginMeEstablishmentsLogout(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()

	resp, body := c.do(http.MethodGet, "/api/admin/v1/auth/me", nil, nil)
	assertProblem(t, resp, body, 401, "unauthorized")

	resp, body = c.login("owner@lido-a.it", "wrong-password")
	assertProblem(t, resp, body, 401, "invalid_credentials")

	resp, body = c.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": "not-an-email", "password": "x"}, nil)
	assertProblem(t, resp, body, 400, "bad_request")

	resp, body = c.login("OWNER@lido-a.it", "password-a-123")
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d %v", resp.StatusCode, body)
	}
	var sessionCookie *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "arenile_session" {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.MaxAge <= 0 {
		t.Fatalf("session cookie attributes: %+v", sessionCookie)
	}
	if body["principal_type"] != "user" || body["user"].(map[string]any)["email"] != "owner@lido-a.it" {
		t.Fatalf("me: %v", body)
	}
	ms := body["memberships"].([]any)
	if len(ms) != 1 || ms[0].(map[string]any)["role"] != "owner" || len(ms[0].(map[string]any)["permissions"].([]any)) != 9 {
		t.Fatalf("memberships: %v", ms)
	}

	resp, body = c.do(http.MethodGet, "/api/admin/v1/auth/me", nil, nil)
	if resp.StatusCode != 200 || body["user"] == nil {
		t.Fatalf("me after login: %d %v", resp.StatusCode, body)
	}

	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments", nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d %v", resp.StatusCode, body)
	}
	items := body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["slug"] != "lido-a" || body["next_cursor"] != nil {
		t.Fatalf("establishments must be scoped to org A: %v", body)
	}
	est := items[0].(map[string]any)
	for _, k := range []string{"id", "organization_id", "timezone", "currency", "default_locale", "enabled_locales", "theme", "address", "allowed_embed_origins", "created_at"} {
		if _, ok := est[k]; !ok {
			t.Fatalf("missing field %s in %v", k, est)
		}
	}
	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments?limit=0", nil, nil)
	assertProblem(t, resp, body, 400, "validation_failed")
	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments?limit=abc", nil, nil)
	assertProblem(t, resp, body, 400, "bad_request")
	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments?cursor=%21%21", nil, nil)
	assertProblem(t, resp, body, 400, "bad_request")

	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA.ID, nil, nil)
	if resp.StatusCode != 200 || body["name"] != "Lido A" {
		t.Fatalf("get: %d %v", resp.StatusCode, body)
	}

	// Logout requires the CSRF token.
	resp, body = c.do(http.MethodPost, "/api/admin/v1/auth/logout", nil, nil)
	assertProblem(t, resp, body, 403, "csrf_failed")
	resp, body = c.do(http.MethodPost, "/api/admin/v1/auth/logout", nil, map[string]string{"X-CSRF-Token": "forged-token-forged-token-forged-token"})
	assertProblem(t, resp, body, 403, "csrf_failed")
	resp, _ = c.do(http.MethodPost, "/api/admin/v1/auth/logout", nil, map[string]string{"X-CSRF-Token": c.csrf()})
	if resp.StatusCode != 204 {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, body = c.do(http.MethodGet, "/api/admin/v1/auth/me", nil, nil)
	assertProblem(t, resp, body, 401, "unauthorized")
}

func TestHTTP_CrossTenantDenied(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
		t.Fatal(body)
	}
	resp, body := c.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID, nil, nil)
	assertProblem(t, resp, body, 404, "not_found")
	resp, body = c.do(http.MethodGet, "/api/admin/v1/stream?establishment="+e.estB.ID, nil, nil)
	assertProblem(t, resp, body, 404, "not_found")
}

func TestHTTP_APIKeyBearer(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	ctx := context.Background()
	login, err := e.auth.Login(ctx, auth.LoginInput{Email: "owner@lido-a.it", Password: "password-a-123", IP: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := e.auth.CreateAPIKey(ctx, login.Principal, auth.CreateAPIKeyInput{EstablishmentID: e.estA.ID, Name: "pos", Scopes: []auth.Permission{auth.PermBookingsRead}})
	if err != nil {
		t.Fatal(err)
	}
	c := e.client()
	resp, body := c.do(http.MethodGet, "/api/admin/v1/auth/me", nil, map[string]string{"Authorization": "Bearer " + key})
	if resp.StatusCode != 200 || body["principal_type"] != "api_key" {
		t.Fatalf("me with api key: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments", nil, map[string]string{"Authorization": "Bearer " + key})
	if resp.StatusCode != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("list with api key: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID, nil, map[string]string{"Authorization": "Bearer " + key})
	assertProblem(t, resp, body, 404, "not_found")
	resp, body = c.do(http.MethodGet, "/api/admin/v1/auth/me", nil, map[string]string{"Authorization": "Bearer ar_live_aaaaaaaa_" + strings.Repeat("a", 52)})
	assertProblem(t, resp, body, 401, "unauthorized")
	// Bearer requests are exempt from CSRF.
	resp, body = c.do(http.MethodPost, "/api/admin/v1/auth/totp/enroll", nil, map[string]string{"Authorization": "Bearer " + key})
	assertProblem(t, resp, body, 403, "forbidden")
}

func TestHTTP_PublicEstablishmentAndHeaders(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	resp, body := c.do(http.MethodGet, "/api/public/v1/lido-a", nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("public: %d %v", resp.StatusCode, body)
	}
	if body["name"] != "Lido A" || body["booking_enabled"] != true || body["theme"].(map[string]any)["primary_color"] != "#0E6BA8" {
		t.Fatalf("public body: %v", body)
	}
	for _, forbidden := range []string{"id", "organization_id", "allowed_embed_origins", "settings"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("public endpoint leaks %s", forbidden)
		}
	}
	if _, ok := body["season"]; ok {
		t.Fatalf("public body has a season without an active one: %v", body)
	}
	// The active season (and only that one) is exposed for the booking calendar.
	ctx := context.Background()
	es, err := store.SystemEstablishmentScope(ctx, e.db, e.estA.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := e.clock.Now()
	for _, s := range []*store.Season{
		{ID: store.NewID(), Year: 2026, OpensOn: calendar.MustParse("2026-05-01"), ClosesOn: calendar.MustParse("2026-09-30"), Status: "closed", CreatedAt: now, UpdatedAt: now},
		{ID: store.NewID(), Year: 2027, OpensOn: calendar.MustParse("2027-05-15"), ClosesOn: calendar.MustParse("2027-09-20"), Status: "active", CreatedAt: now, UpdatedAt: now},
		{ID: store.NewID(), Year: 2028, OpensOn: calendar.MustParse("2028-05-01"), ClosesOn: calendar.MustParse("2028-09-30"), Status: "draft", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.InsertSeason(ctx, e.db, es, s); err != nil {
			t.Fatal(err)
		}
	}
	resp, body = c.do(http.MethodGet, "/api/public/v1/lido-a", nil, nil)
	season, _ := body["season"].(map[string]any)
	if resp.StatusCode != 200 || season["year"] != float64(2027) || season["opens_on"] != "2027-05-15" || season["closes_on"] != "2027-09-20" {
		t.Fatalf("public season: %d %v", resp.StatusCode, body["season"])
	}
	resp, body = c.do(http.MethodGet, "/api/public/v1/nope", nil, nil)
	assertProblem(t, resp, body, 404, "not_found")
	resp, body = c.do(http.MethodGet, "/api/public/v1/BAD_SLUG!", nil, nil)
	if resp.StatusCode != 404 && resp.StatusCode != 400 {
		t.Fatalf("bad slug: %d %v", resp.StatusCode, body)
	}

	resp, _ = c.do(http.MethodGet, "/b/lido-a", nil, nil)
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self' https://www.lido-a.it") {
		t.Fatalf("booking frame-ancestors: %s", csp)
	}
	resp, _ = c.do(http.MethodGet, "/admin/", nil, nil)
	if resp.StatusCode != 200 || resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("admin: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = c.do(http.MethodGet, "/", nil, nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/admin/" {
		t.Fatalf("root redirect: %d", resp.StatusCode)
	}
	resp, body = c.do(http.MethodGet, "/api/admin/v1/does-not-exist", nil, nil)
	assertProblem(t, resp, body, 401, "unauthorized")
	resp, body = c.do(http.MethodGet, "/api/public/v1/lido-a/unknown/path", nil, nil)
	assertProblem(t, resp, body, 404, "not_found")
	resp, body = c.do(http.MethodDelete, "/api/public/v1/lido-a", nil, nil)
	assertProblem(t, resp, body, 405, "method_not_allowed")
}

func TestHTTP_HealthAndReadiness(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	resp, body := c.do(http.MethodGet, "/healthz", nil, nil)
	if resp.StatusCode != 200 || body["status"] != "ok" || body["version"] != "test" {
		t.Fatalf("healthz: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do(http.MethodGet, "/readyz", nil, nil)
	if resp.StatusCode != 200 || body["status"] != "ok" || body["checks"].(map[string]any)["migrations"] != "ok" {
		t.Fatalf("readyz: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do(http.MethodGet, "/metrics", nil, nil); resp.StatusCode != 404 {
		t.Fatalf("metrics must be disabled by default: %d", resp.StatusCode)
	}
}

func TestHTTP_ReadinessFailsWithoutMigrations(t *testing.T) {
	db, err := store.Open(context.Background(), store.Options{SQLiteMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := httpserver.New(httpserver.Deps{DB: db, Establishments: establishment.NewService(db), Readiness: readiness{db}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"pending"`) {
		t.Fatalf("readyz on empty db: %d %s", w.Code, w.Body.String())
	}
}

func TestHTTP_LoginRateLimited(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	for range 8 {
		c.login("owner@lido-a.it", "bad-password")
	}
	resp, body := c.login("owner@lido-a.it", "password-a-123")
	assertProblem(t, resp, body, 429, "rate_limited")
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing")
	}
}

func TestHTTP_SSEStream(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
		t.Fatal(body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.srv.URL+"/api/admin/v1/stream?establishment="+e.estA.ID, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Publish once the subscription is registered.
	go func() {
		for e.broker.Subscribers(e.estA.ID) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		_ = store.RunInTx(ctx, e.db, func(ctx context.Context, tx bun.Tx) error {
			_, err := event.NewOutbox(e.clock).Write(ctx, tx, event.BookingConfirmed, e.estA.ID, 1, map[string]any{"spot_ids": []string{"s1"}, "dates": []string{"2027-07-01"}})
			return err
		})
		_, _ = event.NewDispatcher(e.db, e.clock, nil, e.broker, nil).DispatchPending(ctx)
	}()
	sc := bufio.NewScanner(resp.Body)
	var sawPing, sawEvent bool
	for (!sawPing || !sawEvent) && sc.Scan() {
		line := sc.Text()
		switch {
		case line == ": ping":
			sawPing = true
		case strings.HasPrefix(line, "event: booking.changed"):
			sawEvent = true
		}
	}
	if !sawEvent || !sawPing {
		t.Fatalf("event=%v ping=%v", sawEvent, sawPing)
	}
}

// TestHTTP_M4Contract checks the M4 scaffolding wired in the router: the
// public profile carries a content ETag honoured by If-None-Match, the M4
// operations answer 501 not_implemented until built, and the payment
// webhook route lives outside /api/ (no JSON content-type requirement).
func TestHTTP_M4Contract(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	resp, _ := c.do(http.MethodGet, "/api/public/v1/lido-a", nil, nil)
	tag := resp.Header.Get("ETag")
	if resp.StatusCode != 200 || !strings.HasPrefix(tag, `"`) {
		t.Fatalf("profile etag: %d %q", resp.StatusCode, tag)
	}
	resp, _ = c.do(http.MethodGet, "/api/public/v1/lido-a", nil, map[string]string{"If-None-Match": "W/" + tag})
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match: %d", resp.StatusCode)
	}
	resp, body := c.do(http.MethodGet, "/api/public/v1/lido-a/calendar?month=2027-07", nil, nil)
	assertProblem(t, resp, body, 501, "not_implemented")
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("public M4 responses must not be cached: %q", resp.Header.Get("Cache-Control"))
	}
	resp, body = c.do(http.MethodPost, "/webhooks/payments/stripe/"+e.estA.ID, map[string]any{"id": "evt_1"}, nil)
	assertProblem(t, resp, body, 501, "not_implemented")
}
