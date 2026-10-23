package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store/storetest"
)

// TestHTTP_DailySaleContract checks the daily sale scaffolding wired in the
// router (ADR 0020): every new operation is routed, authenticated and
// answers 501 not_implemented until its domain service is built.
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
	}{
		{http.MethodPost, "/daily-sales/quote", map[string]any{}},
		{http.MethodPost, "/daily-sales", map[string]any{}},
		{http.MethodGet, "/daily-sale-settings", nil},
		{http.MethodPut, "/daily-sale-settings", map[string]any{}},
		{http.MethodGet, "/tickets?date=2027-07-03", nil},
		{http.MethodGet, "/tickets/unassigned?date=2027-07-03", nil},
		{http.MethodPost, "/tickets/auto-assign", map[string]any{}},
		{http.MethodGet, "/tickets/by-code/LD-2027-000001", nil},
		{http.MethodGet, "/tickets/" + id, nil},
		{http.MethodGet, "/tickets/" + id + "/print", nil},
		{http.MethodPost, "/tickets/" + id + "/reprint", map[string]any{}},
		{http.MethodPost, "/tickets/" + id + "/void", map[string]any{}},
		{http.MethodPost, "/tickets/" + id + "/assign", map[string]any{}},
		{http.MethodPost, "/tickets/" + id + "/move", map[string]any{}},
		{http.MethodPost, "/entry-gate/check-in", map[string]any{}},
		{http.MethodGet, "/entry-ticket-types", nil},
		{http.MethodPost, "/entry-ticket-types", map[string]any{}},
		{http.MethodGet, "/entry-ticket-types/" + id, nil},
		{http.MethodPatch, "/entry-ticket-types/" + id, map[string]any{}},
		{http.MethodDelete, "/entry-ticket-types/" + id, nil},
		{http.MethodPost, "/entry-sales/quote", map[string]any{}},
		{http.MethodPost, "/entry-sales", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			resp, body := c.do(tt.method, base+tt.path, tt.body, csrf)
			assertProblem(t, resp, body, http.StatusNotImplemented, "not_implemented")
		})
	}
}
