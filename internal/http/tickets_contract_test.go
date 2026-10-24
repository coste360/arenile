package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store/storetest"
)

// TestHTTP_DailySaleContract checks that the daily sale, ticket and entry
// operations (ADR 0020) are routed, authenticated and wired to their
// services (internal/http/dailysale_wiring.go): reads answer, writes with
// empty bodies are validated, unknown ids are not found; no 501 is left.
func TestHTTP_DailySaleContract(t *testing.T) {
	e := newEnv(t, storetest.SQLite(t))
	c := e.client()
	if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
		t.Fatal(body)
	}
	base := "/api/admin/v1/establishments/" + e.estA.ID
	id := "0190f5b4-0000-7000-8000-000000000001"
	csrf := map[string]string{"X-CSRF-Token": c.csrf()}
	tests := []struct {
		method, path string
		body         any
		status       int
		problem      string
	}{
		{http.MethodPost, "/daily-sales/quote", map[string]any{}, 400, "validation_failed"},
		{http.MethodPost, "/daily-sales", map[string]any{}, 400, "validation_failed"},
		{http.MethodGet, "/daily-sale-settings", nil, 200, ""},
		{http.MethodPut, "/daily-sale-settings", map[string]any{}, 400, "validation_failed"},
		{http.MethodGet, "/tickets?date=2027-07-03", nil, 200, ""},
		{http.MethodGet, "/tickets?date=2027-07-03&format=csv", nil, 200, ""},
		{http.MethodGet, "/tickets/by-code/LD-2027-000001", nil, 404, "not_found"},
		{http.MethodGet, "/tickets/" + id, nil, 404, "not_found"},
		{http.MethodGet, "/tickets/" + id + "/print", nil, 404, "not_found"},
		{http.MethodPost, "/tickets/" + id + "/reprint", map[string]any{}, 404, "not_found"},
		{http.MethodPost, "/tickets/" + id + "/void", map[string]any{"reason": "x"}, 428, "precondition_required"},
		{http.MethodPost, "/tickets/" + id + "/assign", map[string]any{"spot_id": id}, 428, "precondition_required"},
		{http.MethodPost, "/entry-gate/check-in", map[string]any{"code": "LD-2027-000001"}, 404, "not_found"},
		{http.MethodGet, "/entry-ticket-types", nil, 200, ""},
		{http.MethodPost, "/entry-ticket-types", map[string]any{}, 400, "validation_failed"},
		{http.MethodGet, "/entry-ticket-types/" + id, nil, 404, "not_found"},
		{http.MethodPatch, "/entry-ticket-types/" + id, map[string]any{}, 428, "precondition_required"},
		{http.MethodDelete, "/entry-ticket-types/" + id, nil, 404, "not_found"},
		{http.MethodPost, "/entry-sales/quote", map[string]any{}, 400, "validation_failed"},
		{http.MethodPost, "/entry-sales", map[string]any{}, 400, "validation_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			resp, body := c.do(tt.method, base+tt.path, tt.body, csrf)
			if tt.problem == "" {
				if resp.StatusCode != tt.status {
					t.Fatalf("status %d, want %d: %v", resp.StatusCode, tt.status, body)
				}
				return
			}
			assertProblem(t, resp, body, tt.status, tt.problem)
		})
	}
}
