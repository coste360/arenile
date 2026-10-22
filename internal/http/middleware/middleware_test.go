package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/httpctx"
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
			var got *httpctx.Info
			h := RequestContext(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = httpctx.From(r.Context())
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
	withAuth := func(method httpctx.AuthMethod) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpctx.From(r.Context()).AuthMethod = method
				next.ServeHTTP(w, r)
			})
		}
	}
	token := NewCSRFToken()
	tests := []struct {
		name   string
		method string
		auth   httpctx.AuthMethod
		cookie string
		header string
		want   int
	}{
		{"cookie auth, matching token", http.MethodPost, httpctx.AuthCookie, token, token, 204},
		{"cookie auth, missing header", http.MethodPost, httpctx.AuthCookie, token, "", 403},
		{"cookie auth, wrong header", http.MethodPost, httpctx.AuthCookie, token, NewCSRFToken(), 403},
		{"cookie auth, missing cookie", http.MethodPost, httpctx.AuthCookie, "", token, 403},
		{"cookie auth, safe method", http.MethodGet, httpctx.AuthCookie, "", "", 204},
		{"bearer exempt", http.MethodPost, httpctx.AuthBearer, "", "", 204},
		{"anonymous (login)", http.MethodPost, httpctx.AuthNone, "", "", 204},
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

func TestPublicRateLimit_ReadsAndCreations(t *testing.T) {
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	h := RequestContext(nil)(PublicRateLimit(auth.NewRateLimiter(c, 60, time.Minute), auth.NewRateLimiter(c, 10, time.Minute))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })))
	do := func(method, ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/public/v1/lido", strings.NewReader("{}"))
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
