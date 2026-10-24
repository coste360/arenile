package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store/storetest"
)

// TestHTTP_TicketAssignRoutes checks that the assignment operations of the
// daily sale (ADR 0020) are wired to internal/domain/ticketassign: list,
// validation and not-found answers instead of 501.
func TestHTTP_TicketAssignRoutes(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
		t.Fatal(body)
	}
	base := "/api/admin/v1/establishments/" + e.estA.ID
	csrf := map[string]string{"X-CSRF-Token": c.csrf()}
	missing := "0190f5b4-0000-7000-8000-000000000001"

	resp, body := c.do(http.MethodGet, base+"/tickets/unassigned?date=2027-07-03", nil, csrf)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unassigned: %d %v", resp.StatusCode, body)
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("unassigned body: %v", body)
	}
	resp, body = c.do(http.MethodPost, base+"/tickets/auto-assign", map[string]any{"date": "2027-07-03", "dry_run": true}, csrf)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auto-assign: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do(http.MethodPost, base+"/tickets/"+missing+"/assign", map[string]any{"spot_id": missing}, csrf)
	assertProblem(t, resp, body, http.StatusNotFound, "not_found")
	resp, body = c.do(http.MethodPost, base+"/tickets/"+missing+"/move", map[string]any{"spot_id": "x"}, csrf)
	assertProblem(t, resp, body, http.StatusBadRequest, "validation_failed")
}
