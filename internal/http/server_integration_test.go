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
	})
}
