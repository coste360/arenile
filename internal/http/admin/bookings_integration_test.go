//go:build integration

package admin_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// bkCall runs a handler as p with an optional If-Match header.
func bkCall(t *testing.T, p *auth.Principal, body any, ifMatch string, fn func(w http.ResponseWriter, r *http.Request)) (int, http.Header, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	r = r.WithContext(auth.WithPrincipal(r.Context(), p))
	w := httptest.NewRecorder()
	fn(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, w.Header(), out
}

func bkExpect(t *testing.T, what string, status int, body map[string]any, wantStatus int, wantType string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("%s: status %d, want %d: %v", what, status, wantStatus, body)
	}
	if wantType != "" && body["type"] != wantType {
		t.Fatalf("%s: problem type %v, want %s (%v)", what, body["type"], wantType, body)
	}
}

func TestBookingsHTTP_FlowAndErrors(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		h := &admin.Handlers{Bookings: f.Svc, Log: slog.New(slog.DiscardHandler)}
		est := f.Est.ID
		create := map[string]any{
			"customer":  map[string]any{"first_name": "Mario", "last_name": "Rossi", "phone": "+393331234567"},
			"starts_on": "2027-07-10", "ends_on": "2027-07-11", "slots": "FULL", "formula": "multi_day",
			"lines": []map[string]any{{"pack_id": f.Base.ID, "spot_id": f.Spots[0].ID, "extras": []map[string]any{{"extra_id": f.Sunbed.ID, "quantity": 1}}}},
		}
		createFn := func(w http.ResponseWriter, r *http.Request) { h.CreateBooking(w, r, est, api.CreateBookingParams{}) }

		// Quote first: available, priced, no writes.
		status, _, body := bkCall(t, f.Cashier, create, "", func(w http.ResponseWriter, r *http.Request) { h.QuoteBooking(w, r, est, api.QuoteBookingParams{}) })
		bkExpect(t, "quote", status, body, 200, "")
		if body["available"] != true || body["breakdown"].(map[string]any)["total"].(float64) != 2*(bookingtest.BasePerDay+bookingtest.SunbedPerDay) {
			t.Fatalf("quote %v", body)
		}

		status, hdr, body := bkCall(t, f.Cashier, create, "", createFn)
		bkExpect(t, "create", status, body, 201, "")
		if hdr.Get("ETag") != `"1"` || body["code"] != "LD-2027-000001" || body["status"] != "confirmed" || body["balance_due"].(float64) != 5600 {
			t.Fatalf("create: %v %v", hdr, body)
		}
		id := body["id"].(string)
		lines := body["lines"].([]any)
		line := lines[0].(map[string]any)
		if line["spot_label"] != "A1" || line["pack_code"] != "base" || len(line["extras"].([]any)) != 1 || len(body["allocations"].([]any)) != 4 {
			t.Fatalf("detail lines/allocations: %v", body)
		}
		if ns := body["next_statuses"].([]any); len(ns) != 4 {
			t.Fatalf("next statuses %v", ns)
		}

		// Same spot again: 409 spot_unavailable with the conflicts.
		status, _, body = bkCall(t, f.Cashier, create, "", createFn)
		bkExpect(t, "conflict", status, body, 409, "spot_unavailable")
		if cs, _ := body["conflicts"].([]any); len(cs) != 4 {
			t.Fatalf("conflicts %v", body)
		}
		// Viewer: 403; other organization: 404.
		status, _, body = bkCall(t, f.Viewer, create, "", createFn)
		bkExpect(t, "viewer create", status, body, 403, "forbidden")
		status, _, body = bkCall(t, f.Outsider, nil, "", func(w http.ResponseWriter, r *http.Request) { h.GetBooking(w, r, est, id) })
		bkExpect(t, "outsider get", status, body, 404, "not_found")

		get := func(w http.ResponseWriter, r *http.Request) { h.GetBooking(w, r, est, id) }
		status, hdr, body = bkCall(t, f.Viewer, nil, "", get)
		bkExpect(t, "get", status, body, 200, "")
		if hdr.Get("ETag") != `"1"` || body["customer"].(map[string]any)["display_name"] != "Rossi Mario" {
			t.Fatalf("get %v", body)
		}

		// PATCH: If-Match required (428), stale (412), then applied with ETag 2.
		patch := map[string]any{"ends_on": "2027-07-12", "extras": []any{}}
		patchFn := func(ifm string) func(w http.ResponseWriter, r *http.Request) {
			return func(w http.ResponseWriter, r *http.Request) {
				h.UpdateBooking(w, r, est, id, api.UpdateBookingParams{IfMatch: ifm})
			}
		}
		status, _, body = bkCall(t, f.Cashier, patch, "", patchFn(""))
		bkExpect(t, "patch without if-match", status, body, 428, "precondition_required")
		status, _, body = bkCall(t, f.Cashier, patch, "", patchFn(`"7"`))
		bkExpect(t, "patch stale", status, body, 412, "version_conflict")
		status, hdr, body = bkCall(t, f.Cashier, patch, "", patchFn(`W/"1"`))
		bkExpect(t, "patch", status, body, 200, "")
		if hdr.Get("ETag") != `"2"` || body["applied"] != true || body["new_total"].(float64) != 3*bookingtest.BasePerDay+3*bookingtest.SunbedPerDay {
			t.Fatalf("patch %v", body)
		}

		// Confirm a confirmed booking: 409 invalid_transition with current_status.
		status, _, body = bkCall(t, f.Cashier, nil, "", func(w http.ResponseWriter, r *http.Request) {
			h.ConfirmBooking(w, r, est, id, api.ConfirmBookingParams{})
		})
		bkExpect(t, "confirm", status, body, 409, "invalid_transition")
		if body["current_status"] != "confirmed" {
			t.Fatalf("current_status %v", body)
		}
		// Check-in with a stale If-Match: 412; with the right one: 200.
		stale, cur := `"1"`, `"2"`
		status, _, body = bkCall(t, f.Staff, nil, "", func(w http.ResponseWriter, r *http.Request) {
			h.CheckInBooking(w, r, est, id, api.CheckInBookingParams{IfMatch: &stale})
		})
		bkExpect(t, "check-in stale", status, body, 412, "version_conflict")
		status, _, body = bkCall(t, f.Staff, map[string]any{"note": "arrivato"}, "", func(w http.ResponseWriter, r *http.Request) {
			h.CheckInBooking(w, r, est, id, api.CheckInBookingParams{IfMatch: &cur})
		})
		bkExpect(t, "check-in", status, body, 200, "")
		if body["status"] != "checked_in" {
			t.Fatalf("check-in %v", body)
		}

		// Note and list.
		status, _, body = bkCall(t, f.Cashier, map[string]any{"text": "Ombrellone vicino alla doccia"}, "", func(w http.ResponseWriter, r *http.Request) {
			h.AddBookingNote(w, r, est, id, api.AddBookingNoteParams{})
		})
		bkExpect(t, "note", status, body, 200, "")
		status, _, body = bkCall(t, f.Viewer, nil, "", func(w http.ResponseWriter, r *http.Request) {
			h.ListBookings(w, r, est, api.ListBookingsParams{Status: &[]api.BookingStatus{"checked_in"}})
		})
		bkExpect(t, "list", status, body, 200, "")
		if items := body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["spot_labels"].([]any)[0] != "A1" {
			t.Fatalf("list %v", body)
		}

		// Cancel preview on a second booking (100 % refund tier, nothing paid).
		create["lines"] = []map[string]any{{"pack_id": f.Base.ID, "spot_id": f.Spots[1].ID}}
		_, _, body = bkCall(t, f.Cashier, create, "", createFn)
		id2 := body["id"].(string)
		status, _, body = bkCall(t, f.Cashier, map[string]any{"dry_run": true}, "", func(w http.ResponseWriter, r *http.Request) {
			h.CancelBooking(w, r, est, id2, api.CancelBookingParams{})
		})
		bkExpect(t, "cancel preview", status, body, 200, "")
		if body["applied"] != false || body["refund"].(map[string]any)["refund_percent"].(float64) != 100 {
			t.Fatalf("cancel preview %v", body)
		}
		status, _, body = bkCall(t, f.Cashier, map[string]any{"reason": "cliente"}, "", func(w http.ResponseWriter, r *http.Request) {
			h.CancelBooking(w, r, est, id2, api.CancelBookingParams{})
		})
		bkExpect(t, "cancel", status, body, 200, "")
		if body["booking"].(map[string]any)["status"] != "cancelled" || len(body["booking"].(map[string]any)["allocations"].([]any)) != 0 {
			t.Fatalf("cancel %v", body)
		}
		// Move validation: unknown line is 404, missing spot is 400.
		status, _, body = bkCall(t, f.Cashier, map[string]any{"line_id": store.NewID(), "spot_id": f.Spots[3].ID}, "", func(w http.ResponseWriter, r *http.Request) {
			h.MoveBooking(w, r, est, id, api.MoveBookingParams{})
		})
		bkExpect(t, "move unknown line", status, body, 404, "not_found")
		// Auto-assign with nothing pending.
		status, _, body = bkCall(t, f.Cashier, map[string]any{"from": "2027-07-01", "to": "2027-07-31", "dry_run": true}, "", func(w http.ResponseWriter, r *http.Request) {
			h.AutoAssign(w, r, est, api.AutoAssignParams{})
		})
		bkExpect(t, "auto-assign", status, body, 200, "")
	})
}
