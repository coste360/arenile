//go:build integration

package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/establishment"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type pairingTarget struct{}

func (pairingTarget) PairingTarget(context.Context) (device.PairingTarget, error) {
	return device.PairingTarget{URL: "https://lido-a.example.it"}, nil
}

// withDevices replaces the env server with one that wires the device
// service (pairing limit lowered to 3 per minute).
func withDevices(t *testing.T, e *env) {
	t.Helper()
	devices := device.NewService(device.Deps{DB: e.db, Clock: e.clock, Audit: audit.NewWriter(e.clock), Principals: e.auth, Instance: pairingTarget{}})
	h := httpserver.New(httpserver.Deps{
		DB: e.db, Clock: e.clock, Version: "test", Auth: e.auth, Establishments: establishment.NewService(e.db),
		Bus: e.broker, Readiness: readiness{e.db}, Devices: devices, PairingsPerMinute: 3,
	})
	e.srv.Close()
	e.srv = httptest.NewServer(h)
	t.Cleanup(e.srv.Close)
}

// TestHTTPIntegration_DevicePairing runs the pairing flow end to end (spec
// 29.2): a panel session creates a code (cookie + CSRF), an app exchanges
// it for a token, the token authenticates on /api/admin (no CSRF) and on
// /api/device, revocation from the panel is immediate, the code is single
// use and pairing is rate limited per IP.
func TestHTTPIntegration_DevicePairing(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		withDevices(t, e)
		panel := e.client()
		if resp, body := panel.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
			t.Fatal(body)
		}
		codesPath := "/api/admin/v1/establishments/" + e.estA.ID + "/devices/pairing-codes"

		// Cookie authentication needs the CSRF token.
		resp, body := panel.do(http.MethodPost, codesPath, map[string]any{}, nil)
		assertProblem(t, resp, body, 403, "csrf_failed")
		resp, body = panel.do(http.MethodPost, codesPath, nil, map[string]string{"X-CSRF-Token": panel.csrf(), "Content-Type": "application/json"})
		if resp.StatusCode != 201 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("create code: %d %v", resp.StatusCode, body)
		}
		code := body["code"].(string)
		if !strings.HasPrefix(body["qr_text"].(string), "arenile://pair?d=") || !strings.HasPrefix(body["qr_svg"].(string), "<svg") ||
			body["qr_payload"].(map[string]any)["url"] != "https://lido-a.example.it" {
			t.Fatalf("code body %v", body)
		}
		resp, body = panel.do(http.MethodPost, codesPath, map[string]any{"ttl_seconds": 10}, map[string]string{"X-CSRF-Token": panel.csrf()})
		assertProblem(t, resp, body, 400, "validation_failed")

		// The app pairs (no cookies at all).
		app := e.client()
		pair := map[string]any{"code": code, "name": "iPhone di Anna", "platform": "ios", "kind": "mobile", "app_version": "1.0.0"}
		resp, body = app.do(http.MethodPost, "/api/device/v1/pair", pair, nil)
		if resp.StatusCode != 201 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("pair: %d %v", resp.StatusCode, body)
		}
		token := body["token"].(string)
		dev := body["device"].(map[string]any)
		if dev["role"] != "owner" || dev["current"] != true || body["establishment_id"] != e.estA.ID ||
			!strings.HasPrefix(dev["token_prefix"].(string), "ar_dev_") || strings.Contains(token, dev["token_prefix"].(string)+"_") == false {
			t.Fatalf("pair body %v", body)
		}
		resp, body = app.do(http.MethodPost, "/api/device/v1/pair", pair, nil)
		assertProblem(t, resp, body, 400, "pairing_code_invalid")

		bearer := map[string]string{"Authorization": "Bearer " + token}
		resp, body = app.do(http.MethodGet, "/api/admin/v1/auth/me", nil, bearer)
		if resp.StatusCode != 200 || body["device"].(map[string]any)["id"] != dev["id"] {
			t.Fatalf("me with device token: %d %v", resp.StatusCode, body)
		}
		resp, body = app.do(http.MethodGet, "/api/device/v1/self", nil, bearer)
		if resp.StatusCode != 200 || body["id"] != dev["id"] || body["current"] != true {
			t.Fatalf("self: %d %v", resp.StatusCode, body)
		}
		// Unsafe admin request with the token and no CSRF header: it reaches
		// the domain, which refuses devices minting codes (403 forbidden, not
		// csrf_failed).
		resp, body = app.do(http.MethodPost, codesPath, map[string]any{}, bearer)
		assertProblem(t, resp, body, 403, "forbidden")
		// The token is narrowed to its establishment.
		resp, body = app.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID, nil, bearer)
		assertProblem(t, resp, body, 404, "not_found")

		// The panel lists and revokes it; the token stops working at once.
		resp, body = panel.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA.ID+"/devices", nil, nil)
		items := body["items"].([]any)
		if resp.StatusCode != 200 || len(items) != 1 || items[0].(map[string]any)["current"] != false {
			t.Fatalf("list: %d %v", resp.StatusCode, body)
		}
		devPath := "/api/admin/v1/establishments/" + e.estA.ID + "/devices/" + dev["id"].(string)
		for range 2 { // idempotent
			resp, body = panel.do(http.MethodDelete, devPath, nil, map[string]string{"X-CSRF-Token": panel.csrf()})
			if resp.StatusCode != 204 {
				t.Fatalf("revoke: %d %v", resp.StatusCode, body)
			}
		}
		resp, body = app.do(http.MethodGet, "/api/device/v1/self", nil, bearer)
		assertProblem(t, resp, body, 401, "unauthorized")
		resp, body = app.do(http.MethodGet, "/api/admin/v1/auth/me", nil, bearer)
		assertProblem(t, resp, body, 401, "unauthorized")

		// Pairing is limited per IP (3 per minute in this server).
		bad := map[string]any{"code": "ZZZZ-ZZZZ", "name": "x", "platform": "ios", "kind": "mobile"}
		var last int
		for range 4 {
			resp, _ = app.do(http.MethodPost, "/api/device/v1/pair", bad, nil)
			last = resp.StatusCode
		}
		if last != 429 {
			t.Fatalf("pair rate limit: %d", last)
		}
	})
}
