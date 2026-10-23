//go:build integration

package httpserver_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestHTTPIntegration_CrossTenant runs the HTTP isolation checks on every
// configured dialect: a user with a membership on establishment A must never
// read establishment B.
func TestHTTPIntegration_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		c := e.client()
		if resp, body := c.login("owner@lido-a.it", "password-a-123"); resp.StatusCode != 200 {
			t.Fatal(body)
		}
		resp, body := c.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA.ID, nil, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("own establishment: %d %v", resp.StatusCode, body)
		}
		resp, body = c.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID, nil, nil)
		assertProblem(t, resp, body, 404, "not_found")
		_, body = c.do(http.MethodGet, "/api/admin/v1/establishments", nil, nil)
		for _, it := range body["items"].([]any) {
			if it.(map[string]any)["id"] == e.estB.ID {
				t.Fatal("establishment B leaked in list")
			}
		}
		resp, body = c.do(http.MethodGet, "/api/admin/v1/stream?establishment="+e.estB.ID, nil, nil)
		assertProblem(t, resp, body, 404, "not_found")
		// M5B routes (INV-26): campaigns, proposals, waitlist entries and
		// offers, customer credits and rollovers of B are 404 for A's owner,
		// reads and writes alike.
		someID := store.NewID()
		for _, step := range []struct{ method, path string }{
			{http.MethodGet, "/renewal-campaigns"},
			{http.MethodGet, "/renewal-campaigns/" + someID},
			{http.MethodGet, "/renewal-campaigns/" + someID + "/proposals"},
			{http.MethodPost, "/renewal-campaigns/" + someID + "/proposals/bulk"},
			{http.MethodGet, "/renewal-proposals/" + someID},
			{http.MethodPost, "/renewal-proposals/" + someID + "/send"},
			{http.MethodGet, "/waitlist"},
			{http.MethodGet, "/waitlist/" + someID},
			{http.MethodPost, "/waitlist/" + someID + "/offer"},
			{http.MethodPost, "/waitlist/" + someID + "/offers/" + someID + "/revoke"},
			{http.MethodGet, "/waitlist-settings"},
			{http.MethodGet, "/customers/" + someID + "/credits"},
			{http.MethodPost, "/customers/" + someID + "/credits"},
			{http.MethodGet, "/season-rollovers"},
			{http.MethodPost, "/season-rollovers"},
		} {
			var payload any
			if step.method == http.MethodPost {
				payload = map[string]any{}
			}
			resp, body = c.do(step.method, "/api/admin/v1/establishments/"+e.estB.ID+step.path, payload, nil)
			if resp.StatusCode != 404 {
				t.Fatalf("%s %s on establishment B: %d %v", step.method, step.path, resp.StatusCode, body)
			}
		}
	})
}
