package desktop

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/app"
)

// TestFirstRun_LoopbackEnforcement checks the first-run endpoint through the
// real router of the core (middleware chain included): only the local
// machine, addressed by a loopback Host, reaches it; a LAN peer (even with
// a forged X-Forwarded-For) or a DNS-rebinding Host is refused before the
// handler, so the one-time token is never consumed by them.
func TestFirstRun_LoopbackEnforcement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	cfg, err := DesktopConfig(dir, 18190, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(RuntimeDeps{DataDir: dir})
	tok, _ := NewSetupToken()
	rt.SetSetupToken(tok)
	a, err := app.New(cfg, app.WithDesktop(rt), app.WithLogger(slog.New(slog.DiscardHandler)), app.WithoutSignalHandling())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	h := a.Handler()

	token := tok.Value()
	body := func() *bytes.Reader {
		b, _ := json.Marshal(map[string]string{
			"setup_token": token, "email": "anna@lido-test.it", "name": "Anna",
			"password": "spiaggia-sicura-2027", "organization_name": "Lido Test",
		})
		return bytes.NewReader(b)
	}
	tests := []struct {
		name, method, remote, host, xff string
		want                            int
		wantType                        string
		tls                             bool
	}{
		{"lan peer status", http.MethodGet, "192.168.1.5:50000", "192.168.1.20:8443", "", 403, "loopback_required", false},
		{"lan peer create", http.MethodPost, "192.168.1.5:50000", "127.0.0.1:18190", "", 403, "loopback_required", false},
		{"forged forwarded-for", http.MethodPost, "10.0.0.7:50000", "127.0.0.1:18190", "127.0.0.1", 403, "loopback_required", false},
		{"dns rebinding host", http.MethodPost, "127.0.0.1:50000", "arenile.attacker.example:18190", "", 403, "loopback_required", false},
		{"ipv6 loopback status", http.MethodGet, "[::1]:50000", "[::1]:18190", "", 200, "", false},
		{"loopback status", http.MethodGet, "127.0.0.1:50000", "127.0.0.1:18190", "", 200, "", false},
		// Over the desktop LAN listener (TLS) the cookies carry Secure even
		// though the loopback profile of the configuration does not.
		{"loopback create over tls", http.MethodPost, "127.0.0.1:50000", "localhost:18190", "", 201, "", true},
		{"loopback replay", http.MethodPost, "127.0.0.1:50000", "127.0.0.1:18190", "", 409, "setup_completed", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r *http.Request
			if tt.method == http.MethodPost {
				r = httptest.NewRequest(tt.method, "/api/desktop/v1/first-run", body())
				r.Header.Set("Content-Type", "application/json")
			} else {
				r = httptest.NewRequest(tt.method, "/api/desktop/v1/first-run", nil)
			}
			r.RemoteAddr, r.Host = tt.remote, tt.host
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want || !strings.Contains(w.Body.String(), tt.wantType) {
				t.Fatalf("%d %s, want %d %s", w.Code, w.Body, tt.want, tt.wantType)
			}
			if tt.want == 201 {
				cookies := w.Header().Values("Set-Cookie")
				if !strings.Contains(strings.Join(cookies, "\n"), "arenile_session=") {
					t.Fatalf("session cookie: %v", cookies)
				}
				for _, c := range cookies {
					if strings.Contains(c, "Secure") != tt.tls {
						t.Fatalf("Secure must follow TLS (%v): %q", tt.tls, c)
					}
				}
			}
		})
	}
	// Settings need an owner session: anonymous loopback requests are 401.
	r := httptest.NewRequest(http.MethodGet, "/api/desktop/v1/settings", nil)
	r.RemoteAddr, r.Host = "127.0.0.1:1", "127.0.0.1:18190"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous settings: %d", w.Code)
	}
}

// TestFirstRun_ServerEditionHidden: without the desktop runtime the
// desktop API does not exist (404), loopback or not.
func TestFirstRun_ServerEditionHidden(t *testing.T) {
	cfg, err := DesktopConfig(filepath.Join(t.TempDir(), "data"), 18191, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(cfg, app.WithLogger(slog.New(slog.DiscardHandler)), app.WithoutSignalHandling())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	r := httptest.NewRequest(http.MethodGet, "/api/desktop/v1/first-run", nil)
	r.RemoteAddr, r.Host = "127.0.0.1:1", "127.0.0.1:18191"
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("server edition: %d %s", w.Code, w.Body)
	}
}
