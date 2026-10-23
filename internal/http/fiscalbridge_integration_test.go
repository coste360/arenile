//go:build integration

package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/fiscalbridge"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// TestHTTPIntegration_FiscalBridge runs the bridge API through the real
// router (spec 29.1): a desktop app paired with a device token registers
// as the bridge, long-polls a job, submits the result idempotently; the
// panel sees the status; revoking the device deactivates the bridge.
func TestHTTPIntegration_FiscalBridge(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		bridge := fiscalbridge.NewService(fiscalbridge.Deps{DB: e.db, Clock: e.clock, Audit: audit.NewWriter(e.clock), Recheck: 10 * time.Millisecond})
		devices := device.NewService(device.Deps{
			DB: e.db, Clock: e.clock, Audit: audit.NewWriter(e.clock), Principals: e.auth, Instance: pairingTarget{}, Bridge: bridge,
		})
		h := httpserver.New(httpserver.Deps{
			DB: e.db, Clock: e.clock, Version: "test", Auth: e.auth, Establishments: establishment.NewService(e.db),
			Bus: e.broker, Readiness: readiness{e.db}, Devices: devices, FiscalBridge: bridge,
		})
		e.srv.Close()
		e.srv = httptest.NewServer(h)
		t.Cleanup(e.srv.Close)

		panel := e.client()
		if resp, body := panel.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
			t.Fatal(body)
		}
		estPath := "/api/admin/v1/establishments/" + e.estA.ID
		resp, body := panel.do(http.MethodPost, estPath+"/devices/pairing-codes", nil, map[string]string{"X-CSRF-Token": panel.csrf(), "Content-Type": "application/json"})
		if resp.StatusCode != 201 {
			t.Fatalf("code: %d %v", resp.StatusCode, body)
		}
		app := e.client()
		resp, body = app.do(http.MethodPost, "/api/device/v1/pair", map[string]any{
			"code": body["code"], "name": "PC cassa", "platform": "windows", "kind": "desktop", "app_version": "1.0.0",
		}, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("pair: %d %v", resp.StatusCode, body)
		}
		bearer := map[string]string{"Authorization": "Bearer " + body["token"].(string)}
		deviceID := body["device"].(map[string]any)["id"].(string)

		// Session cookies are not accepted on the device API.
		resp, body = panel.do(http.MethodPut, "/api/device/v1/fiscal-bridge", map[string]any{"driver": "noop"}, map[string]string{"X-CSRF-Token": panel.csrf()})
		assertProblem(t, resp, body, 401, "unauthorized")

		resp, body = app.do(http.MethodPut, "/api/device/v1/fiscal-bridge", map[string]any{
			"driver": "noop", "device_status": map[string]any{"online": true, "message": "pronto"},
		}, bearer)
		if resp.StatusCode != 200 || body["registered"] != true || body["device_id"] != deviceID || body["device_name"] != "PC cassa" {
			t.Fatalf("register: %d %v", resp.StatusCode, body)
		}
		resp, body = app.do(http.MethodGet, "/api/device/v1/fiscal-bridge/jobs?wait=99", nil, bearer)
		assertProblem(t, resp, body, 400, "validation_failed")
		resp, body = app.do(http.MethodGet, "/api/device/v1/fiscal-bridge/jobs?wait=0", nil, bearer)
		if resp.StatusCode != 200 || len(body["jobs"].([]any)) != 0 {
			t.Fatalf("empty poll: %d %v", resp.StatusCode, body)
		}

		// A job enqueued while the agent waits wakes the long-poll.
		go func() {
			time.Sleep(100 * time.Millisecond)
			_, _, _, _ = bridge.Enqueue(context.Background(), e.estA.ID, fiscalbridge.KindIssue, "fiscal_p1", fiscalbridge.DocumentFromRequest(ext.FiscalDocumentRequest{
				EstablishmentID: e.estA.ID, PaymentID: "p1", Currency: "EUR", Total: 1220, IdempotencyKey: "fiscal_p1",
				Lines:    []ext.FiscalLine{{Description: "Ombrellone", Quantity: 1, UnitGross: 1220, Gross: 1220, VATRateBP: 2200}},
				Payments: []ext.FiscalPayment{{Method: "cash", Amount: 1220}},
			}))
		}()
		resp, body = app.do(http.MethodGet, "/api/device/v1/fiscal-bridge/jobs?wait=10&max=5", nil, bearer)
		jobs, _ := body["jobs"].([]any)
		if resp.StatusCode != 200 || len(jobs) != 1 {
			t.Fatalf("long poll: %d %v", resp.StatusCode, body)
		}
		job := jobs[0].(map[string]any)
		doc := job["document"].(map[string]any)
		if job["kind"] != "issue" || doc["total"] != float64(1220) || doc["idempotency_key"] != "fiscal_p1" {
			t.Fatalf("job %v", job)
		}
		resultPath := "/api/device/v1/fiscal-bridge/jobs/" + job["id"].(string) + "/result"
		result := map[string]any{"outcome": "succeeded", "ref": "rt-1", "number": "0042-0001", "raw": map[string]any{"z": 1}}
		for i := range 2 { // the second submission is a retry after a lost response
			resp, body = app.do(http.MethodPost, resultPath, result, bearer)
			if resp.StatusCode != 204 {
				t.Fatalf("submit %d: %d %v", i, resp.StatusCode, body)
			}
		}
		resp, body = app.do(http.MethodPost, resultPath, map[string]any{"outcome": "failed", "error": "altro"}, bearer)
		assertProblem(t, resp, body, 409, "conflict")

		resp, body = panel.do(http.MethodGet, estPath+"/fiscal-bridge", nil, nil)
		if resp.StatusCode != 200 || body["registered"] != true || body["online"] != true || body["pending_jobs"] != float64(0) || body["driver"] != "noop" {
			t.Fatalf("panel status: %d %v", resp.StatusCode, body)
		}
		resp, body = panel.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID+"/fiscal-bridge", nil, nil)
		assertProblem(t, resp, body, 404, "not_found")

		// Revoking the device from the panel deactivates the bridge.
		resp, body = panel.do(http.MethodDelete, estPath+"/devices/"+deviceID, nil, map[string]string{"X-CSRF-Token": panel.csrf()})
		if resp.StatusCode != 204 {
			t.Fatalf("revoke: %d %v", resp.StatusCode, body)
		}
		resp, body = panel.do(http.MethodGet, estPath+"/fiscal-bridge", nil, nil)
		if resp.StatusCode != 200 || body["registered"] != false {
			t.Fatalf("status after revoke: %v", body)
		}
		resp, body = app.do(http.MethodGet, "/api/device/v1/fiscal-bridge/jobs?wait=0", nil, bearer)
		assertProblem(t, resp, body, 401, "unauthorized")
	})
}
