package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/meta"
)

// fakeDesktop is a control.Control in LAN mode with no behaviour.
type fakeDesktop struct{ control.Control }

func (fakeDesktop) Runtime() meta.Runtime {
	return meta.Runtime{Edition: meta.EditionDesktop, Desktop: &meta.Desktop{Mode: meta.DesktopLAN, LANURL: "https://192.168.1.20:8443", LANFingerprint: "AA"}}
}

// TestM6_ContractRouting checks the routing decisions of the M6/M7
// contract on both editions: public /meta, device API authentication,
// desktop API hidden on the server edition and loopback-only on desktop.
func TestM6_ContractRouting(t *testing.T) {
	newApp := func(t *testing.T, opts ...Option) http.Handler {
		t.Helper()
		a, err := New(devConfig(t), append(opts, WithVersion("m6"), WithoutSignalHandling(), WithLogger(slog.New(slog.DiscardHandler)))...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = a.Close() })
		return a.Handler()
	}
	do := func(h http.Handler, method, path, remote, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		if remote != "" {
			r.RemoteAddr = remote
			r.Host = "127.0.0.1:18190"
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	devToken := "ar_dev_abcdefgh_" + strings.Repeat("a", 52)

	server := newApp(t)
	w := do(server, http.MethodGet, "/api/admin/v1/meta", "", "")
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || m["edition"] != "server" || m["version"] != "m6" {
		t.Fatalf("server meta: %d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		name, method, path, remote, bearer string
		want                               int
	}{
		{"device api needs a token", http.MethodGet, "/api/device/v1/self", "", "", 401},
		{"api keys are not device tokens", http.MethodGet, "/api/device/v1/self", "", "ar_live_abcdefgh_" + strings.Repeat("a", 52), 401},
		{"unknown device token on admin", http.MethodGet, "/api/admin/v1/auth/me", "", devToken, 401},
		{"pair is public", http.MethodPost, "/api/device/v1/pair", "", "", 400},
		{"desktop api hidden on server", http.MethodGet, "/api/desktop/v1/first-run", "192.168.1.5:4000", "", 404},
	} {
		if got := do(server, tc.method, tc.path, tc.remote, tc.bearer).Code; got != tc.want {
			t.Errorf("server %s: %d, want %d", tc.name, got, tc.want)
		}
	}

	desktop := newApp(t, WithDesktop(fakeDesktop{}))
	// LAN mode guard: peers outside the local network are refused on the
	// desktop edition (httptest's default peer 192.0.2.1 is public).
	if got := do(desktop, http.MethodGet, "/api/admin/v1/meta", "", "").Code; got != 403 {
		t.Errorf("desktop from a public peer: %d, want 403", got)
	}
	w = do(desktop, http.MethodGet, "/api/admin/v1/meta", "192.168.1.5:4000", "")
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	caps, _ := m["capabilities"].(map[string]any)
	if w.Code != 200 || m["edition"] != "desktop" || caps["online_booking"] != false || caps["device_pairing"] != true {
		t.Fatalf("desktop meta: %d %s", w.Code, w.Body)
	}
	if got := do(desktop, http.MethodGet, "/api/desktop/v1/first-run", "192.168.1.5:4000", "").Code; got != 403 {
		t.Errorf("desktop api from the LAN: %d, want 403", got)
	}
	if got := do(desktop, http.MethodGet, "/api/desktop/v1/first-run", "127.0.0.1:4000", "").Code; got != 200 {
		t.Errorf("desktop first-run status from loopback: %d, want 200", got)
	}
	if got := do(desktop, http.MethodGet, "/api/desktop/v1/settings", "127.0.0.1:4000", "").Code; got != 401 {
		t.Errorf("desktop settings without session: %d, want 401", got)
	}
}
