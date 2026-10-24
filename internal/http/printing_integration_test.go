//go:build integration

package httpserver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/printing"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/printing/printertest"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestHTTPIntegration_Printing drives the printing API through the real
// router on the desktop and server editions: configure a printer on the
// fake TCP printer, route a document, "Stampa di prova", idempotent job
// creation and the server-edition fallback.
func TestHTTPIntegration_Printing(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		desktop := printing.NewService(printing.Deps{DB: e.db, Clock: e.clock, Audit: audit.NewWriter(e.clock), Available: true})
		server := printing.NewService(printing.Deps{DB: e.db, Clock: e.clock, Audit: audit.NewWriter(e.clock)})
		mount := func(svc *printing.Service) *httptest.Server {
			srv := httptest.NewServer(httpserver.New(httpserver.Deps{
				DB: e.db, Clock: e.clock, Version: "test", Auth: e.auth, Establishments: establishment.NewService(e.db),
				Bus: e.broker, Readiness: readiness{e.db}, Printing: svc,
			}))
			t.Cleanup(srv.Close)
			return srv
		}
		e.srv.Close()
		e.srv = mount(desktop)
		fp := printertest.Start(t)

		c := e.client()
		if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
			t.Fatal(body)
		}
		base := "/api/admin/v1/establishments/" + e.estA.ID
		hdr := func(extra ...string) map[string]string {
			h := map[string]string{"X-CSRF-Token": c.csrf(), "Content-Type": "application/json"}
			for i := 0; i+1 < len(extra); i += 2 {
				h[extra[i]] = extra[i+1]
			}
			return h
		}

		resp, body := c.do(http.MethodPost, base+"/printers", map[string]any{"name": "Pubblica", "host": "8.8.8.8", "paper_width_mm": 80}, hdr())
		assertProblem(t, resp, body, 422, "printer_address_not_allowed")

		resp, body = c.do(http.MethodPost, base+"/printers", map[string]any{
			"name": "Cassa", "host": fp.Host(), "port": fp.Port(), "paper_width_mm": 58, "drawer_pin": 2,
		}, hdr("Idempotency-Key", "create-cassa"))
		if resp.StatusCode != 201 || resp.Header.Get("ETag") != `"1"` || body["chars_per_line"] != float64(32) || body["codepage"] != "cp858" || body["logo_cached"] != false {
			t.Fatalf("create: %d %v", resp.StatusCode, body)
		}
		printerID := body["id"].(string)

		resp, body = c.do(http.MethodGet, base+"/printers", nil, nil)
		if resp.StatusCode != 200 || body["escpos_available"] != true || len(body["items"].([]any)) != 1 || body["unavailable_reason"] != nil {
			t.Fatalf("list: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodGet, base+"/printers/not-a-uuid", nil, nil)
		assertProblem(t, resp, body, 404, "not_found")

		resp, body = c.do(http.MethodPost, base+"/printers/"+printerID+"/check", nil, hdr())
		if resp.StatusCode != 200 || body["status"] != "reachable" {
			t.Fatalf("check: %d %v", resp.StatusCode, body)
		}

		// Routing: ETag over the whole set, 412 on a stale one.
		resp, _ = c.do(http.MethodGet, base+"/print-routes", nil, nil)
		etag := resp.Header.Get("ETag")
		routes := map[string]any{"items": []any{map[string]any{"document_kind": "test_page", "printer_id": printerID, "copies": 2}}}
		resp, body = c.do(http.MethodPut, base+"/print-routes", routes, hdr("If-Match", `"12345"`))
		assertProblem(t, resp, body, 412, "version_conflict")
		resp, body = c.do(http.MethodPut, base+"/print-routes", routes, hdr("If-Match", etag))
		if resp.StatusCode != 200 || resp.Header.Get("ETag") == etag || len(body["items"].([]any)) != 1 {
			t.Fatalf("routes: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodGet, base+"/print-routes/resolve?document_kind=test_page", nil, nil)
		if resp.StatusCode != 200 || body["ready"] != true || body["copies"] != float64(2) || body["printer"].(map[string]any)["id"] != printerID {
			t.Fatalf("resolve: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodGet, base+"/print-routes/resolve?document_kind=cash_z", nil, nil)
		if body["ready"] != false || body["reason"] != "no_route" {
			t.Fatalf("resolve without route: %v", body)
		}
		resp, body = c.do(http.MethodPost, base+"/print-jobs", map[string]any{"document_kind": "cash_z", "document_id": printerID}, hdr())
		assertProblem(t, resp, body, 409, "print_route_missing")

		// "Stampa di prova" reaches the printer.
		resp, body = c.do(http.MethodPost, base+"/printers/"+printerID+"/test-print", nil, hdr("Idempotency-Key", "test-print-0001"))
		if resp.StatusCode != 202 || body["status"] != "queued" || body["printer_name"] != "Cassa" || body["document_kind"] != "test_page" {
			t.Fatalf("test print: %d %v", resp.StatusCode, body)
		}
		jobID := body["id"].(string)
		// The routed test page (2 copies) through createPrintJob, then the same key again: 200, same job.
		resp, body = c.do(http.MethodPost, base+"/print-jobs", map[string]any{"document_kind": "test_page"}, hdr("Idempotency-Key", "print-job-0001"))
		if resp.StatusCode != 202 || body["copies"] != float64(2) {
			t.Fatalf("create job: %d %v", resp.StatusCode, body)
		}
		routedID := body["id"].(string)
		if _, err := desktop.DeliverDue(t.Context(), 10); err != nil {
			t.Fatal(err)
		}
		jobs := fp.WaitJobs(2, 5*time.Second)
		if len(jobs) != 2 || !bytes.Contains(jobs[0], []byte("STAMPA DI PROVA")) || !bytes.Contains(jobs[0], []byte{0x1d, 'V', 1}) {
			t.Fatalf("printer got %d jobs", len(jobs))
		}
		resp, body = c.do(http.MethodGet, base+"/print-jobs/"+jobID, nil, nil)
		if resp.StatusCode != 200 || body["status"] != "printed" || body["printed_at"] == nil || body["attempts"] != float64(1) {
			t.Fatalf("job: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodPost, base+"/print-jobs/"+routedID+"/retry", nil, hdr())
		assertProblem(t, resp, body, 409, "invalid_transition")
		resp, body = c.do(http.MethodGet, base+"/print-jobs?status=printed&limit=1", nil, nil)
		if resp.StatusCode != 200 || len(body["items"].([]any)) != 1 || body["next_cursor"] == nil {
			t.Fatalf("list jobs: %d %v", resp.StatusCode, body)
		}

		// Server edition: configuration visible, printing refused.
		e.srv = mount(server)
		c = e.client()
		if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
			t.Fatal(body)
		}
		resp, body = c.do(http.MethodGet, base+"/printers", nil, nil)
		if resp.StatusCode != 200 || body["escpos_available"] != false || body["unavailable_reason"] != "server_edition" {
			t.Fatalf("server list: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodPost, base+"/printers/"+printerID+"/test-print", nil, hdr())
		assertProblem(t, resp, body, 409, "printing_unavailable")
		resp, body = c.do(http.MethodGet, base+"/print-routes/resolve?document_kind=test_page", nil, nil)
		if body["ready"] != false || body["reason"] != "server_edition" {
			t.Fatalf("server resolve: %v", body)
		}

		// Delete with a stale If-Match, then for real.
		resp, body = c.do(http.MethodDelete, base+"/printers/"+printerID, nil, hdr("If-Match", `"9"`))
		assertProblem(t, resp, body, 412, "version_conflict")
		resp, _ = c.do(http.MethodDelete, base+"/printers/"+printerID, nil, hdr("If-Match", `"1"`))
		if resp.StatusCode != 204 {
			t.Fatalf("delete: %d", resp.StatusCode)
		}
	})
}
