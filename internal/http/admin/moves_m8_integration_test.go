//go:build integration

package admin_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/findspot"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMovesHTTP_FindSpotSwapUndoDuplicate: the M8 endpoints of the moves
// builder end to end on the handlers (spec 30.6, 30.7, 30.9).
func TestMovesHTTP_FindSpotSwapUndoDuplicate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		h := &admin.Handlers{
			Bookings: f.Svc, Log: slog.New(slog.DiscardHandler),
			SpotFinder: findspot.NewService(findspot.Deps{DB: db, Clock: f.Clock, Quoter: f.Svc, Booker: f.Svc}),
		}
		est := f.Est.ID
		f.MustCreate(f.Input(f.Spots[0], "2027-07-12", "2027-07-12"))
		for _, s := range f.Spots[2:] {
			f.MustCreate(f.Input(s, "2027-07-10", "2027-07-12"))
		}
		// A1 busy on 12, A2 free: one whole-stay proposal (A2), and with a
		// change also A1 then A2.
		search := map[string]any{
			"starts_on": "2027-07-10", "ends_on": "2027-07-12", "slots": "FULL", "pack_id": f.Base.ID,
			"zone_ids": []string{f.SeatZone.ID}, "max_spot_changes": 1,
		}
		status, _, body := bkCall(t, f.Cashier, search, "", func(w http.ResponseWriter, r *http.Request) { h.FindSpot(w, r, est, api.FindSpotParams{}) })
		bkExpect(t, "find spot", status, body, 200, "")
		props := body["proposals"].([]any)
		if len(props) < 2 {
			t.Fatalf("proposals %v", body)
		}
		p1 := props[0].(map[string]any)
		if p1["spot_changes"].(float64) != 0 || p1["total"].(float64) != 3*bookingtest.BasePerDay {
			t.Fatalf("first proposal %v", p1)
		}
		split := props[1].(map[string]any)
		segs := split["lines"].([]any)[0].(map[string]any)["segments"].([]any)
		if split["spot_changes"].(float64) != 1 || len(segs) != 2 || segs[0].(map[string]any)["spot_label"] != "A1" {
			t.Fatalf("split proposal %v", split)
		}
		var segIn []map[string]any
		for _, s := range segs {
			m := s.(map[string]any)
			segIn = append(segIn, map[string]any{"spot_id": m["spot_id"], "starts_on": m["starts_on"], "ends_on": m["ends_on"]})
		}
		book := map[string]any{
			"proposal_id": split["id"], "expected_total": split["total"],
			"booking": map[string]any{
				"customer":  map[string]any{"first_name": "Anna", "last_name": "Bianchi"},
				"starts_on": "2027-07-10", "ends_on": "2027-07-12", "slots": "FULL", "formula": "multi_day",
				"lines": []map[string]any{{"pack_id": f.Base.ID, "segments": segIn}},
			},
		}
		bookFn := func(w http.ResponseWriter, r *http.Request) {
			h.BookFindSpotProposal(w, r, est, api.BookFindSpotProposalParams{})
		}
		status, _, body = bkCall(t, f.Cashier, book, "", bookFn)
		bkExpect(t, "book proposal", status, body, 201, "")
		line := body["lines"].([]any)[0].(map[string]any)
		if len(line["segments"].([]any)) != 2 {
			t.Fatalf("booked line %v", line)
		}
		splitID, splitLine := body["id"].(string), line["id"].(string)
		status, _, body = bkCall(t, f.Cashier, book, "", bookFn)
		bkExpect(t, "book again", status, body, 409, "spot_unavailable")

		// Swap the split booking with the A3 one from 07-11.
		rows := []store.BookingLine{}
		if err := db.NewSelect().Model(&rows).Where("spot_id = ?", f.Spots[2].ID).Scan(f.Ctx); err != nil || len(rows) != 1 {
			t.Fatalf("A3 line: %v", err)
		}
		a3 := &rows[0]
		swap := map[string]any{
			"a": map[string]any{"booking_id": splitID, "line_id": splitLine},
			"b": map[string]any{"booking_id": a3.BookingID, "line_id": a3.ID}, "from": "2027-07-11",
		}
		swapFn := func(w http.ResponseWriter, r *http.Request) { h.SwapBookings(w, r, est, api.SwapBookingsParams{}) }
		status, _, body = bkCall(t, f.Cashier, swap, "", swapFn)
		bkExpect(t, "swap", status, body, 200, "")
		undo := body["undo"].(map[string]any)
		if body["applied"] != true || body["to"] != "2027-07-12" || undo["kind"] != "swap" {
			t.Fatalf("swap %v", body)
		}
		status, _, body = bkCall(t, f.Cashier, nil, "", func(w http.ResponseWriter, r *http.Request) { h.GetLastMove(w, r, est) })
		bkExpect(t, "last move", status, body, 200, "")
		if body["move"].(map[string]any)["move_id"] != undo["move_id"] {
			t.Fatalf("last move %v", body)
		}
		undoFn := func(w http.ResponseWriter, r *http.Request) {
			h.UndoMove(w, r, est, undo["move_id"].(string), api.UndoMoveParams{})
		}
		status, _, body = bkCall(t, f.Cashier, nil, "", undoFn)
		bkExpect(t, "undo", status, body, 200, "")
		if body["undone"] != true || len(body["bookings"].([]any)) != 2 {
			t.Fatalf("undo %v", body)
		}
		status, _, body = bkCall(t, f.Cashier, nil, "", undoFn)
		bkExpect(t, "undo twice", status, body, 409, "move_undo_conflict")

		// Swap without a common interval.
		swap["from"] = "2027-07-20"
		status, _, body = bkCall(t, f.Cashier, swap, "", swapFn)
		bkExpect(t, "swap no interval", status, body, 409, "swap_conflict")

		// Duplicate: preview then copy.
		dupFn := func(w http.ResponseWriter, r *http.Request) {
			h.DuplicateBooking(w, r, est, a3.BookingID, api.DuplicateBookingParams{})
		}
		status, _, body = bkCall(t, f.Cashier, map[string]any{"starts_on": "2027-08-01", "ends_on": "2027-08-02", "dry_run": true}, "", dupFn)
		bkExpect(t, "duplicate preview", status, body, 200, "")
		if body["applied"] != false || body["total"].(float64) != 2*bookingtest.BasePerDay {
			t.Fatalf("preview %v", body)
		}
		status, hdr, body := bkCall(t, f.Cashier, map[string]any{"starts_on": "2027-08-01", "ends_on": "2027-08-02"}, "", dupFn)
		bkExpect(t, "duplicate", status, body, 201, "")
		if body["applied"] != true || hdr.Get("ETag") == "" || body["booking"].(map[string]any)["lines"].([]any)[0].(map[string]any)["spot_label"] != "A3" {
			t.Fatalf("duplicate %v", body)
		}

		// Group move (one line here; the domain tests cover several).
		group := map[string]any{"from": "2027-08-01", "moves": []map[string]any{
			{"booking_id": body["booking"].(map[string]any)["id"], "line_id": body["booking"].(map[string]any)["lines"].([]any)[0].(map[string]any)["id"], "spot_id": f.Spots[3].ID},
		}}
		status, _, body = bkCall(t, f.Cashier, group, "", func(w http.ResponseWriter, r *http.Request) {
			h.MoveBookingGroup(w, r, est, api.MoveBookingGroupParams{})
		})
		bkExpect(t, "group move", status, body, 200, "")
		if body["applied"] != true || len(body["results"].([]any)) != 1 || body["undo"].(map[string]any)["kind"] != "group" {
			t.Fatalf("group move %v", body)
		}
	})
}
