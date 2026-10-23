package middleware

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/reqctx"
)

func TestRequestContext_RealIPAndScheme(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name       string
		remote     string
		xff        string
		proto      string
		reqID      string
		wantIP     string
		wantHTTPS  bool
		wantSameID bool
	}{
		{"direct client ignores xff", "203.0.113.5:1234", "1.2.3.4", "https", "", "203.0.113.5", false, false},
		{"trusted proxy", "10.0.0.2:80", "198.51.100.7", "https", "", "198.51.100.7", true, false},
		{"chain of trusted proxies", "10.0.0.2:80", "198.51.100.7, 10.1.1.1", "http", "", "198.51.100.7", false, false},
		{"spoofed leftmost entry ignored", "10.0.0.2:80", "6.6.6.6, 198.51.100.7", "", "", "198.51.100.7", false, false},
		{"ipv6", "[2001:db8::1]:443", "", "", "", "2001:db8::1", false, false},
		{"keeps sane request id", "203.0.113.5:1", "", "", "abc-123", "203.0.113.5", false, true},
		{"replaces bad request id", "203.0.113.5:1", "", "", "bad id\n", "203.0.113.5", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *reqctx.Info
			h := RequestContext(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = reqctx.From(r.Context())
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			if tt.reqID != "" {
				r.Header.Set("X-Request-ID", tt.reqID)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if got.ClientIP != tt.wantIP || got.HTTPS != tt.wantHTTPS {
				t.Fatalf("got ip=%s https=%v", got.ClientIP, got.HTTPS)
			}
			if got.RequestID == "" || w.Header().Get("X-Request-ID") != got.RequestID {
				t.Fatal("request id must be set and echoed")
			}
			if tt.wantSameID != (got.RequestID == tt.reqID) {
				t.Fatalf("request id %q", got.RequestID)
			}
		})
	}
}

func TestSecurityHeaders_ByArea(t *testing.T) {
	origins := func(_ context.Context, slug string) ([]string, error) {
		if slug == "lido-a" {
			return []string{"https://www.lido-a.it", "javascript:alert(1)", "https://bad.it; script-src *", "https://*.partner.it"}, nil
		}
		return nil, nil
	}
	h := RequestContext(nil)(SecurityHeaders(SecurityOptions{EmbedOrigins: origins, AdminFrameSources: []string{"https://plugin.example.com"}})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })))
	get := func(path string, https bool) http.Header {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if https {
			r.Header.Set("X-Forwarded-Proto", "https")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Header()
	}
	admin := get("/admin/", false)
	if !strings.Contains(admin.Get("Content-Security-Policy"), "frame-ancestors 'none'") || admin.Get("X-Frame-Options") != "DENY" ||
		!strings.Contains(admin.Get("Content-Security-Policy"), "script-src 'self';") ||
		!strings.Contains(admin.Get("Content-Security-Policy"), "frame-src 'self' https://plugin.example.com") {
		t.Fatalf("admin csp: %v", admin)
	}
	if admin.Get("X-Content-Type-Options") != "nosniff" || admin.Get("Referrer-Policy") == "" {
		t.Fatal("base headers")
	}
	if admin.Get("Strict-Transport-Security") != "" {
		t.Fatal("no HSTS over plain http")
	}
	for _, path := range []string{"/b/lido-a/m/" + strings.Repeat("t", 43), "/b/lido-a/m", "/b/lido-a/m/"} {
		m := get(path, false)
		if m.Get("Referrer-Policy") != "no-referrer" || m.Get("Cache-Control") != "no-store" {
			t.Fatalf("manage page %s: %v", path, m)
		}
	}
	if pg := get("/b/lido-a", false); pg.Get("Referrer-Policy") != "strict-origin-when-cross-origin" || pg.Get("Cache-Control") != "" {
		t.Fatalf("public page headers: %v", pg)
	}
	b := get("/b/lido-a/checkout", false).Get("Content-Security-Policy")
	if !strings.HasSuffix(b, "frame-ancestors 'self' https://www.lido-a.it https://*.partner.it") {
		t.Fatalf("booking csp: %s", b)
	}
	if strings.Contains(b, "javascript") || strings.Contains(b, "bad.it") {
		t.Fatalf("unsafe origins must be dropped: %s", b)
	}
	other := get("/b/lido-b", false).Get("Content-Security-Policy")
	if !strings.HasSuffix(other, "frame-ancestors 'self'") {
		t.Fatalf("no origins means same origin only: %s", other)
	}
	api := get("/api/public/v1/lido-a", false)
	if api.Get("Content-Security-Policy") != apiCSP || api.Get("Cache-Control") != "no-store" {
		t.Fatalf("api headers: %v", api)
	}
	// Direct requests cannot fake https; HSTS only when TLS terminates here.
	r := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	r.TLS = nil
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("untrusted X-Forwarded-Proto ignored")
	}
}

func TestRequireJSON_Rejects(t *testing.T) {
	h := RequestContext(nil)(RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
	tests := []struct {
		method, ct, body string
		want             int
	}{
		{http.MethodPost, "application/json", "{}", 204},
		{http.MethodPost, "application/json; charset=utf-8", "{}", 204},
		{http.MethodPost, "application/x-www-form-urlencoded", "a=b", 415},
		{http.MethodPost, "text/plain", "{}", 415},
		{http.MethodPost, "", "", 204},
		{http.MethodGet, "text/plain", "", 204},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(tt.method, "/api/x", strings.NewReader(tt.body))
		if tt.ct != "" {
			r.Header.Set("Content-Type", tt.ct)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatalf("%s %q: %d", tt.method, tt.ct, w.Code)
		}
	}
}

func TestCSRF_DoubleSubmit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	withAuth := func(method reqctx.AuthMethod) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqctx.From(r.Context()).AuthMethod = method
				next.ServeHTTP(w, r)
			})
		}
	}
	token := NewCSRFToken()
	tests := []struct {
		name   string
		method string
		auth   reqctx.AuthMethod
		cookie string
		header string
		want   int
	}{
		{"cookie auth, matching token", http.MethodPost, reqctx.AuthCookie, token, token, 204},
		{"cookie auth, missing header", http.MethodPost, reqctx.AuthCookie, token, "", 403},
		{"cookie auth, wrong header", http.MethodPost, reqctx.AuthCookie, token, NewCSRFToken(), 403},
		{"cookie auth, missing cookie", http.MethodPost, reqctx.AuthCookie, "", token, 403},
		{"cookie auth, safe method", http.MethodGet, reqctx.AuthCookie, "", "", 204},
		{"bearer exempt", http.MethodPost, reqctx.AuthBearer, "", "", 204},
		{"anonymous (login)", http.MethodPost, reqctx.AuthNone, "", "", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := RequestContext(nil)(withAuth(tt.auth)(CSRF(true, time.Hour)(ok)))
			r := httptest.NewRequest(tt.method, "/api/admin/v1/auth/logout", nil)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: tt.cookie})
			}
			if tt.header != "" {
				r.Header.Set(CSRFHeader, tt.header)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if tt.cookie == "" && !strings.Contains(w.Header().Get("Set-Cookie"), CSRFCookie+"=") {
				t.Fatal("csrf cookie must be issued when missing")
			}
			if tt.want == 403 && !strings.Contains(w.Body.String(), `"type":"csrf_failed"`) {
				t.Fatalf("problem type: %s", w.Body.String())
			}
		})
	}
}

// TestCookieSecure_FollowsTLS: with Secure cookies off in the configuration
// (plain-HTTP development, desktop loopback), a request that arrived over
// TLS (desktop LAN listener) still gets Secure cookies; a plain request
// cannot fake it with X-Forwarded-Proto from an untrusted peer.
func TestCookieSecure_FollowsTLS(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := RequestContext(nil)(CSRF(false, time.Hour)(ok))
	for _, tt := range []struct {
		name string
		tls  bool
		xfp  string
		want bool
	}{
		{"plain", false, "", false},
		{"tls", true, "", true},
		{"forged forwarded-proto", false, "https", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/me", nil)
		if tt.tls {
			r.TLS = &tls.ConnectionState{}
		}
		if tt.xfp != "" {
			r.Header.Set("X-Forwarded-Proto", tt.xfp)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		set := w.Header().Get("Set-Cookie")
		if got := strings.Contains(set, "Secure"); got != tt.want || !strings.Contains(set, CSRFCookie+"=") {
			t.Errorf("%s: %q", tt.name, set)
		}
		if got := CookieSecure(r.WithContext(reqctx.With(r.Context(), &reqctx.Info{HTTPS: tt.tls})), false); got != tt.want {
			t.Errorf("%s: CookieSecure = %v", tt.name, got)
		}
	}
	if !CookieSecure(httptest.NewRequest(http.MethodGet, "/", nil), true) {
		t.Fatal("configured Secure must stay on over plain HTTP")
	}
}

func TestPublicRateLimit_ReadsAndCreations(t *testing.T) {
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	h := RequestContext(nil)(PublicRateLimit(auth.NewRateLimiter(c, 60, time.Minute), auth.NewRateLimiter(c, 10, time.Minute))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })))
	do := func(method, ip string) *httptest.ResponseRecorder {
		path := "/api/public/v1/lido"
		if method == http.MethodPost {
			path += "/bookings"
		}
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.RemoteAddr = ip + ":1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i := range 60 {
		if w := do(http.MethodGet, "1.1.1.1"); w.Code != 200 {
			t.Fatalf("read %d limited", i)
		}
	}
	w := do(http.MethodGet, "1.1.1.1")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("61st read: %d", w.Code)
	}
	for i := range 10 {
		if w := do(http.MethodPost, "1.1.1.1"); w.Code != 200 {
			t.Fatalf("creation %d limited", i)
		}
	}
	if w := do(http.MethodPost, "1.1.1.1"); w.Code != 429 {
		t.Fatal("11th creation must be limited")
	}
	// Steps of an existing booking (session token, magic link) count as
	// reads: they still pass once the creations of the IP are exhausted.
	for range 10 {
		do(http.MethodPost, "4.4.4.4")
	}
	for i, step := range []struct{ method, path string }{
		{http.MethodPatch, "/api/public/v1/lido/bookings/b1"},
		{http.MethodPost, "/api/public/v1/lido/bookings/b1/pay"},
		{http.MethodPost, "/api/public/v1/lido/manage/tok/cancel"},
	} {
		r := httptest.NewRequest(step.method, step.path, strings.NewReader("{}"))
		r.RemoteAddr = "4.4.4.4:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("step %d limited (%d): steps of an existing booking are reads", i, w.Code)
		}
	}
	links := httptest.NewRequest(http.MethodPost, "/api/public/v1/lido/manage-links", strings.NewReader("{}"))
	links.RemoteAddr = "1.1.1.1:1"
	if w := httptest.NewRecorder(); func() int { h.ServeHTTP(w, links); return w.Code }() != 429 {
		t.Fatal("manage-links is a creation: limited after 10 creations")
	}
	join := httptest.NewRequest(http.MethodPost, "/api/public/v1/lido/waitlist", strings.NewReader("{}"))
	join.RemoteAddr = "1.1.1.1:1"
	if w := httptest.NewRecorder(); func() int { h.ServeHTTP(w, join); return w.Code }() != 429 {
		t.Fatal("waitlist join is a creation: limited after 10 creations")
	}
	for _, path := range []string{"/api/public/v1/lido/waitlist/tok/accept", "/api/public/v1/lido/waitlist/tok/decline"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		r.RemoteAddr = "4.4.4.4:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s limited (%d): steps of an existing waitlist request are reads", path, w.Code)
		}
	}
	quote := httptest.NewRequest(http.MethodPost, "/api/public/v1/lido/quote", strings.NewReader("{}"))
	quote.RemoteAddr = "3.3.3.3:1"
	for i := range 11 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, quote.Clone(quote.Context()))
		if w.Code != 200 {
			t.Fatalf("quote %d limited: quotes count as reads", i)
		}
	}
	if w := do(http.MethodGet, "2.2.2.2"); w.Code != 200 {
		t.Fatal("other IPs unaffected")
	}
	c.Advance(time.Minute)
	if w := do(http.MethodGet, "1.1.1.1"); w.Code != 200 {
		t.Fatal("window reset")
	}
}

func TestOnPrefix_Scoping(t *testing.T) {
	block := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(418) })
	}
	h := OnPrefix("/api/admin/", block)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	for path, want := range map[string]int{"/api/admin/v1/x": 418, "/api/public/v1/x": 200, "/admin/": 200} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != want {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
