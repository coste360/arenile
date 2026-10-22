//go:build integration

package admin_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestTeamHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newStEnv(t, db)
		owner, manager, cashier, viewer, ownerB := e.as("owner@lido-a.it"), e.as("manager@lido-a.it"), e.as("cashier@lido-a.it"), e.as("viewer@lido-a.it"), e.as("owner@lido-b.it")
		base := "/api/admin/v1/establishments/" + e.estA.ID + "/team"

		// Everybody can see the team; other tenants cannot.
		for _, c := range []*stClient{viewer, cashier, manager} {
			resp, body := c.do(http.MethodGet, base, nil, nil)
			stExpect(t, resp, body, 200, "")
			if len(body["items"].([]any)) != 4 {
				t.Fatalf("members %v", body)
			}
		}
		resp, body := ownerB.do(http.MethodGet, base, nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		// Only owners manage users.
		newMember := map[string]any{"email": "bagnino@lido-a.it", "name": "Bagnino", "role": "beach_staff"}
		for _, c := range []*stClient{manager, cashier, viewer} {
			resp, body := c.do(http.MethodPost, base, newMember, nil)
			stExpect(t, resp, body, 403, "forbidden")
		}
		resp, body = owner.do(http.MethodPost, base, newMember, map[string]string{"Idempotency-Key": "team-member-create-1"})
		stExpect(t, resp, body, 201, "")
		pw, _ := body["initial_password"].(string)
		member := body["member"].(map[string]any)
		if pw == "" || body["user_created"] != true || member["role"] != "beach_staff" || member["scope"] != "establishment" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("created %v %v", resp.Header, body)
		}
		userID := member["user_id"].(string)
		// The temporary password works.
		staff := &stClient{t: t, base: e.srv.URL, http: &http.Client{Jar: newStJar()}}
		if resp, body := staff.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": "bagnino@lido-a.it", "password": pw}, nil); resp.StatusCode != 200 {
			t.Fatalf("login with temporary password: %d %v", resp.StatusCode, body)
		}
		resp, body = owner.do(http.MethodPost, base, newMember, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodPost, base, map[string]any{"email": "x@lido-a.it", "name": "X", "role": "viewer", "password": "short"}, nil)
		stExpect(t, resp, body, 400, "validation_failed")

		// Role change, then deactivation.
		resp, body = manager.do(http.MethodPatch, base+"/"+userID, map[string]any{"role": "cashier"}, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = owner.do(http.MethodPatch, base+"/"+userID, map[string]any{"role": "cashier"}, nil)
		stExpect(t, resp, body, 200, "")
		if body["role"] != "cashier" {
			t.Fatalf("role %v", body)
		}
		resp, body = owner.do(http.MethodPatch, base+"/"+userID, map[string]any{"active": false}, nil)
		stExpect(t, resp, body, 200, "")
		if body["active"] != false {
			t.Fatalf("active %v", body)
		}
		// Its session is gone.
		if resp, _ := staff.do(http.MethodGet, "/api/admin/v1/auth/me", nil, nil); resp.StatusCode != 401 {
			t.Fatalf("deactivated session still valid: %d", resp.StatusCode)
		}
		// Last owner rule and self-deactivation.
		resp, body = owner.do(http.MethodPatch, base+"/"+e.users["owner"], map[string]any{"role": "manager"}, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodPatch, base+"/"+e.users["owner"], map[string]any{"active": false}, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodDelete, base+"/"+e.users["owner"], nil, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodPatch, base+"/"+userID, map[string]any{"role": "emperor"}, nil)
		stExpect(t, resp, body, 400, "validation_failed")

		// Cross tenant: owner B cannot touch members of A, even through B's id.
		resp, body = ownerB.do(http.MethodPatch, "/api/admin/v1/establishments/"+e.estB.ID+"/team/"+userID, map[string]any{"role": "viewer"}, nil)
		stExpect(t, resp, body, 404, "not_found")
		resp, body = ownerB.do(http.MethodDelete, base+"/"+userID, nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		// Remove access.
		resp, body = owner.do(http.MethodDelete, base+"/"+userID, nil, nil)
		stExpect(t, resp, body, 204, "")
		resp, body = owner.do(http.MethodPatch, base+"/"+userID, map[string]any{"role": "viewer"}, nil)
		stExpect(t, resp, body, 404, "not_found")
		resp, body = viewer.do(http.MethodGet, base+"?limit=2", nil, nil)
		stExpect(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 2 || body["next_cursor"] == nil {
			t.Fatalf("page %v", body)
		}
	})
}
