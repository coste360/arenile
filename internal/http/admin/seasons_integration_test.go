//go:build integration

package admin_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestSeasonHTTP_SeasonsRatePeriodsClosures(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newStEnv(t, db)
		manager, cashier, viewer, ownerB := e.as("manager@lido-a.it"), e.as("cashier@lido-a.it"), e.as("viewer@lido-a.it"), e.as("owner@lido-b.it")
		base := "/api/admin/v1/establishments/" + e.estA.ID

		// Seasons.
		season := map[string]any{"year": 2027, "opens_on": "2027-05-15", "closes_on": "2027-09-30"}
		resp, body := cashier.do(http.MethodPost, base+"/seasons", season, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = manager.do(http.MethodPost, base+"/seasons", season, map[string]string{"Idempotency-Key": "season-2027"})
		stExpect(t, resp, body, 201, "")
		if resp.Header.Get("ETag") != `"1"` || body["status"] != "draft" || body["opens_on"] != "2027-05-15" {
			t.Fatalf("season %v %v", resp.Header, body)
		}
		seasonID := body["id"].(string)
		resp, body = manager.do(http.MethodPost, base+"/seasons", season, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = manager.do(http.MethodPost, base+"/seasons", map[string]any{"year": 2028, "opens_on": "2028-09-30", "closes_on": "2028-05-01"}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = manager.do(http.MethodPost, base+"/seasons", map[string]any{"year": 2028, "opens_on": "30/09/2028", "closes_on": "2028-05-01"}, nil)
		stExpect(t, resp, body, 400, "bad_request")

		resp, body = viewer.do(http.MethodGet, base+"/seasons", nil, nil)
		stExpect(t, resp, body, 200, "")
		if items := body["items"].([]any); len(items) != 1 {
			t.Fatalf("list %v", body)
		}
		resp, body = ownerB.do(http.MethodGet, base+"/seasons", nil, nil)
		stExpect(t, resp, body, 404, "not_found")
		resp, body = ownerB.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB.ID+"/seasons/"+seasonID, nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		active := map[string]any{"year": 2027, "opens_on": "2027-05-15", "closes_on": "2027-09-30", "status": "active"}
		resp, body = manager.do(http.MethodPut, base+"/seasons/"+seasonID, active, map[string]string{"If-Match": `W/"1"`})
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"2"` || body["status"] != "active" {
			t.Fatalf("activate %v %v", resp.Header, body)
		}
		resp, body = manager.do(http.MethodPut, base+"/seasons/"+seasonID, active, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 412, "version_conflict")
		resp, body = manager.do(http.MethodDelete, base+"/seasons/"+seasonID, nil, nil)
		stExpect(t, resp, body, 409, "conflict")

		// Rate periods.
		low := map[string]any{
			"season_id": seasonID, "code": "low", "name": map[string]string{"it": "Bassa", "en": "Low"}, "color": "#00AA00",
			"ranges": []map[string]string{{"from": "2027-05-15", "to": "2027-06-30"}, {"from": "2027-09-01", "to": "2027-09-30"}},
		}
		resp, body = manager.do(http.MethodPost, base+"/rate-periods", low, nil)
		stExpect(t, resp, body, 201, "")
		lowID := body["id"].(string)
		if body["color"] != "#00aa00" || len(body["ranges"].([]any)) != 2 || body["name"].(map[string]any)["en"] != "Low" {
			t.Fatalf("rate period %v", body)
		}
		overlap := map[string]any{
			"season_id": seasonID, "code": "high", "name": map[string]string{"it": "Alta"},
			"ranges": []map[string]string{{"from": "2027-06-20", "to": "2027-08-31"}},
		}
		resp, body = manager.do(http.MethodPost, base+"/rate-periods", overlap, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		overlap["ranges"] = []map[string]string{{"from": "2027-07-01", "to": "2027-08-31"}}
		resp, body = manager.do(http.MethodPost, base+"/rate-periods", overlap, nil)
		stExpect(t, resp, body, 201, "")
		resp, body = cashier.do(http.MethodPost, base+"/rate-periods", overlap, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = viewer.do(http.MethodGet, base+"/rate-periods?season_id="+seasonID+"&limit=1", nil, nil)
		stExpect(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 1 || body["next_cursor"] == nil {
			t.Fatalf("paginated %v", body)
		}
		resp, body = viewer.do(http.MethodGet, base+"/rate-periods?limit=500", nil, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		low["code"] = "high"
		resp, body = manager.do(http.MethodPut, base+"/rate-periods/"+lowID, low, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 409, "conflict")
		resp, body = manager.do(http.MethodGet, base+"/rate-periods/"+lowID, nil, nil)
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"1"` {
			t.Fatalf("etag %q", resp.Header.Get("ETag"))
		}
		resp, body = manager.do(http.MethodDelete, base+"/rate-periods/"+lowID, nil, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 204, "")

		// Closure days.
		closure := map[string]any{"date": "2027-07-10", "slots": []string{"PM", "AM"}, "reason": "Maltempo"}
		resp, body = manager.do(http.MethodPost, base+"/closure-days", closure, nil)
		stExpect(t, resp, body, 201, "")
		closureID := body["id"].(string)
		if s := body["slots"].([]any); len(s) != 2 || s[0] != "AM" {
			t.Fatalf("slots %v", body)
		}
		resp, body = manager.do(http.MethodPost, base+"/closure-days", map[string]any{"date": "2027-12-25", "slots": []string{"AM"}}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = manager.do(http.MethodPost, base+"/closure-days", map[string]any{"date": "2027-07-11", "slots": []string{"EVENING"}}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = cashier.do(http.MethodGet, base+"/closure-days?from=2027-07-01&to=2027-07-31", nil, nil)
		stExpect(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 1 {
			t.Fatalf("closures %v", body)
		}
		resp, body = cashier.do(http.MethodGet, base+"/closure-days?from=luglio", nil, nil)
		if resp.StatusCode != 400 {
			t.Fatalf("bad date filter %d %v", resp.StatusCode, body)
		}
		resp, body = viewer.do(http.MethodDelete, base+"/closure-days/"+closureID, nil, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = manager.do(http.MethodPut, base+"/closure-days/"+closureID, map[string]any{"date": "2027-07-10", "slots": []string{"AM"}}, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("ETag") != `"2"` || body["reason"] != "" {
			t.Fatalf("full replacement %v", body)
		}
		resp, body = manager.do(http.MethodDelete, base+"/closure-days/"+closureID, nil, nil)
		stExpect(t, resp, body, 204, "")
		resp, body = manager.do(http.MethodGet, base+"/closure-days/"+closureID, nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		// A draft season can be deleted (with its periods).
		resp, body = manager.do(http.MethodPost, base+"/seasons", map[string]any{"year": 2028, "opens_on": "2028-05-15", "closes_on": "2028-09-30"}, nil)
		stExpect(t, resp, body, 201, "")
		resp, body = manager.do(http.MethodDelete, base+"/seasons/"+body["id"].(string), nil, map[string]string{"If-Match": `"1"`})
		stExpect(t, resp, body, 204, "")
	})
}
