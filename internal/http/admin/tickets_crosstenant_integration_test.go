//go:build integration

package admin_test

import (
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestDailySaleHTTP_CrossTenantAndIfMatch (INV-26, INV-80): every ticket
// route is scoped by the establishment of the path and the principal's
// organisation: the owner of another organisation gets 403 on A and 404 on
// A's tickets through B; a spot or zone of another establishment is refused
// by the sale and by the assignment; the gate never sees another lido's
// QR. Assign and move require If-Match like the map actions.
func TestDailySaleHTTP_CrossTenantAndIfMatch(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h, f := newDailySaleHandlers(t, db)
		a, b := f.Est.ID, f.Other.ID
		date := bookingtest.D("2027-07-10")
		if status, _, body := call(t, f.Cashier, http.MethodPost, api.CashSessionOpen{OpeningFloat: 0}, func(w http.ResponseWriter, r *http.Request) {
			h.OpenCashSession(w, r, a, api.OpenCashSessionParams{})
		}); status != 201 {
			t.Fatalf("open: %d %v", status, body)
		}
		// A spot-first sale in A, by card.
		status, _, body := call(t, f.Cashier, http.MethodPost, api.DailySaleCreate{
			Date: date, Slots: "FULL", SpotId: &f.Spots[0].ID, PackId: f.Base.ID,
			Payment: api.DailySalePayment{Method: api.TicketPaymentMethod("card")},
		}, func(w http.ResponseWriter, r *http.Request) { h.CreateDailySale(w, r, a, api.CreateDailySaleParams{}) })
		if status != 201 {
			t.Fatalf("create: %d %v", status, body)
		}
		ticket := body["ticket"].(map[string]any)
		id, code := ticket["id"].(string), ticket["booking_code"].(string)
		if ticket["state"] != "assigned" {
			t.Fatalf("ticket %v", ticket)
		}
		// Its QR token, read through the print data of A.
		status, _, body = call(t, f.Viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetTicketPrint(w, r, a, id, api.GetTicketPrintParams{})
		})
		token, _ := body["qr_payload"].(string)
		if status != 200 || token == "" {
			t.Fatalf("print: %d %v", status, body)
		}

		// A spot or zone of B never sells or receives a ticket of A.
		if status, _, body := call(t, f.Cashier, http.MethodPost, api.DailySaleCreate{
			Date: date, Slots: "FULL", SpotId: &f.OtherSpot.ID, PackId: f.Base.ID,
			Payment: api.DailySalePayment{Method: api.TicketPaymentMethod("card")},
		}, func(w http.ResponseWriter, r *http.Request) { h.CreateDailySale(w, r, a, api.CreateDailySaleParams{}) }); status != 400 {
			t.Fatalf("sale on another establishment's spot: %d %v", status, body)
		}
		if status, _, body := call(t, f.Cashier, http.MethodPost, api.TicketAssignRequest{SpotId: f.OtherSpot.ID}, func(w http.ResponseWriter, r *http.Request) {
			h.MoveTicket(w, r, a, id, api.MoveTicketParams{IfMatch: "*"})
		}); status != 400 {
			t.Fatalf("move onto another establishment's spot: %d %v", status, body)
		}

		// If-Match: mandatory on move and assign (428), stale version 412.
		move := api.TicketAssignRequest{SpotId: f.Spots[1].ID}
		if status, _, body := call(t, f.Cashier, http.MethodPost, move, func(w http.ResponseWriter, r *http.Request) {
			h.MoveTicket(w, r, a, id, api.MoveTicketParams{})
		}); status != 428 || body["type"] != "precondition_required" {
			t.Fatalf("move without If-Match: %d %v", status, body)
		}
		if status, _, body := call(t, f.Cashier, http.MethodPost, move, func(w http.ResponseWriter, r *http.Request) {
			h.MoveTicket(w, r, a, id, api.MoveTicketParams{IfMatch: `"7"`})
		}); status != 412 || body["type"] != "version_conflict" {
			t.Fatalf("move with a stale version: %d %v", status, body)
		}
		if status, _, body := call(t, f.Cashier, http.MethodPost, move, func(w http.ResponseWriter, r *http.Request) {
			h.AssignTicket(w, r, a, id, api.AssignTicketParams{})
		}); status != 428 {
			t.Fatalf("assign without If-Match: %d %v", status, body)
		}
		// With the right version the move goes through and bumps the ETag.
		status, hdr, body := call(t, f.Cashier, http.MethodPost, move, func(w http.ResponseWriter, r *http.Request) {
			h.MoveTicket(w, r, a, id, api.MoveTicketParams{IfMatch: `"1"`})
		})
		if status != 200 || hdr.Get("ETag") != `"2"` || body["spot_id"] != f.Spots[1].ID {
			t.Fatalf("move: %d %s %v", status, hdr.Get("ETag"), body)
		}

		// Owner of B: 404 on A's routes (another organisation never learns
		// that A exists) and on A's tickets through B.
		for name, fn := range map[string]func(w http.ResponseWriter, r *http.Request){
			"list tickets of A": func(w http.ResponseWriter, r *http.Request) {
				h.ListTickets(w, r, a, api.ListTicketsParams{Date: date})
			},
			"gate of A": func(w http.ResponseWriter, r *http.Request) {
				h.CheckInEntryTicket(w, r, a, api.CheckInEntryTicketParams{})
			},
		} {
			status, _, body := call(t, f.Outsider, http.MethodPost, api.GateCheckInRequest{Code: token}, fn)
			if status != 404 {
				t.Fatalf("%s as outsider: %d %v", name, status, body)
			}
		}
		for name, fn := range map[string]func(w http.ResponseWriter, r *http.Request){
			"get ticket":   func(w http.ResponseWriter, r *http.Request) { h.GetTicket(w, r, b, id) },
			"get by code":  func(w http.ResponseWriter, r *http.Request) { h.GetTicketByCode(w, r, b, code) },
			"get by token": func(w http.ResponseWriter, r *http.Request) { h.GetTicketByCode(w, r, b, token) },
			"print": func(w http.ResponseWriter, r *http.Request) {
				h.GetTicketPrint(w, r, b, id, api.GetTicketPrintParams{})
			},
			"void": func(w http.ResponseWriter, r *http.Request) {
				h.VoidTicket(w, r, b, id, api.VoidTicketParams{IfMatch: "*"})
			},
			"assign": func(w http.ResponseWriter, r *http.Request) {
				h.AssignTicket(w, r, b, id, api.AssignTicketParams{IfMatch: "*"})
			},
			"gate": func(w http.ResponseWriter, r *http.Request) {
				h.CheckInEntryTicket(w, r, b, api.CheckInEntryTicketParams{})
			},
		} {
			// One body serves every route: the handlers read only their fields.
			status, _, body := call(t, f.Outsider, http.MethodPost, map[string]any{
				"code": token, "manual": true, "reason": "x", "spot_id": f.OtherSpot.ID,
			}, fn)
			if status != 404 || body["type"] != "not_found" {
				t.Fatalf("%s via B: %d %v", name, status, body)
			}
		}
		// B has no tickets of its own.
		status, _, body = call(t, f.Outsider, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListTickets(w, r, b, api.ListTicketsParams{Date: date})
		})
		if status != 200 || len(body["items"].([]any)) != 0 {
			t.Fatalf("tickets of B: %d %v", status, body)
		}
		// Anonymous: 401.
		if status, _, _ := call(t, nil, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListTickets(w, r, a, api.ListTicketsParams{Date: date})
		}); status != 401 {
			t.Fatalf("anonymous: %d", status)
		}
	})
}
