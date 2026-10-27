//go:build integration

package admin_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/findspot"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestM8HTTP_CrossTenant (INV-26): the M8 endpoints never read or write
// across establishments. A booking, line, spot, partner, payment, move or
// agreement of establishment B used from A is 404 (or 400 for a spot named
// in a body); A's own ids used from B's owner are 404 too.
func TestM8HTTP_CrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		aw, ob := audit.NewWriter(f.Clock), event.NewOutbox(f.Clock)
		partners := partner.NewService(partner.Deps{DB: db, Clock: f.Clock, Audit: aw, Outbox: ob})
		h := &admin.Handlers{
			Bookings: f.Svc, Partners: partners, Pricing: f.Pricing, Log: slog.New(slog.DiscardHandler),
			SpotFinder: findspot.NewService(findspot.Deps{DB: db, Clock: f.Clock, Quoter: f.Svc, Booker: f.Svc}),
			Inventory:  inventory.NewService(inventory.Deps{DB: db, Clock: f.Clock, Audit: aw, Outbox: ob}),
		}
		est := f.Est.ID
		a := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-12"))
		b := f.MustCreate(f.Input(f.Spots[1], "2027-07-10", "2027-07-12"))
		from := "2027-07-11"
		anyVersion := "*"
		// A move of A's own booking: its undo entry belongs to A.
		status, _, body := bkCall(t, f.Cashier, map[string]any{"line_id": a.Lines[0].ID, "spot_id": f.Spots[2].ID, "from": from}, `"1"`,
			func(w http.ResponseWriter, r *http.Request) { h.MoveBooking(w, r, est, a.Booking.ID, api.MoveBookingParams{IfMatch: &anyVersion}) })
		bkExpect(t, "move", status, body, 200, "")
		moveID := body["undo"].(map[string]any)["move_id"].(string)

		// B's owner (another organization) against A's ids: not found.
		other := f.Outsider
		otherEst := f.OtherSpot.EstablishmentID
		for _, step := range []struct {
			name string
			fn   func(w http.ResponseWriter, r *http.Request)
			body any
			want int
		}{
			{"swap A's bookings from B", func(w http.ResponseWriter, r *http.Request) { h.SwapBookings(w, r, otherEst, api.SwapBookingsParams{}) },
				map[string]any{"a": map[string]any{"booking_id": a.Booking.ID, "line_id": a.Lines[0].ID}, "b": map[string]any{"booking_id": b.Booking.ID, "line_id": b.Lines[0].ID}, "from": from}, 404},
			{"group move A's line from B", func(w http.ResponseWriter, r *http.Request) { h.MoveBookingGroup(w, r, otherEst, api.MoveBookingGroupParams{}) },
				map[string]any{"moves": []map[string]any{{"booking_id": a.Booking.ID, "line_id": a.Lines[0].ID, "spot_id": f.OtherSpot.ID}}, "from": from}, 404},
			{"undo A's move from B", func(w http.ResponseWriter, r *http.Request) { h.UndoMove(w, r, otherEst, moveID, api.UndoMoveParams{}) }, nil, 404},
			{"duplicate A's booking from B", func(w http.ResponseWriter, r *http.Request) { h.DuplicateBooking(w, r, otherEst, a.Booking.ID, api.DuplicateBookingParams{}) },
				map[string]any{"starts_on": "2027-07-20", "ends_on": "2027-07-21"}, 404},
			{"calendar of A's spot from B", func(w http.ResponseWriter, r *http.Request) {
				h.GetSpotCalendar(w, r, otherEst, f.Spots[0].ID, api.GetSpotCalendarParams{})
			}, nil, 404},
			{"find spot in A's zone from B", func(w http.ResponseWriter, r *http.Request) { h.FindSpot(w, r, otherEst, api.FindSpotParams{}) },
				map[string]any{"starts_on": "2027-07-10", "ends_on": "2027-07-12", "slots": "FULL", "pack_id": f.Base.ID, "zone_ids": []string{f.SeatZone.ID}}, 400},
		} {
			status, _, body := bkCall(t, other, step.body, "", step.fn)
			if status != step.want {
				t.Fatalf("%s: %d %v (want %d)", step.name, status, body, step.want)
			}
		}

		// A's cashier against B's spot and ids: a spot of B in a body is a
		// validation error, B's ids are not found.
		status, _, body = bkCall(t, f.Cashier, map[string]any{"moves": []map[string]any{{"booking_id": a.Booking.ID, "line_id": a.Lines[0].ID, "spot_id": f.OtherSpot.ID}}, "from": from}, "",
			func(w http.ResponseWriter, r *http.Request) { h.MoveBookingGroup(w, r, est, api.MoveBookingGroupParams{}) })
		if status != 400 {
			t.Fatalf("group move onto B's spot: %d %v", status, body)
		}
		status, _, body = bkCall(t, f.Cashier, map[string]any{"line_id": a.Lines[0].ID, "spot_id": f.OtherSpot.ID}, "",
			func(w http.ResponseWriter, r *http.Request) { h.MoveBooking(w, r, est, a.Booking.ID, api.MoveBookingParams{IfMatch: &anyVersion}) })
		if status != 400 {
			t.Fatalf("move onto B's spot: %d %v", status, body)
		}
		status, _, body = bkCall(t, f.Cashier, nil, "", func(w http.ResponseWriter, r *http.Request) {
			h.GetSpotCalendar(w, r, est, f.OtherSpot.ID, api.GetSpotCalendarParams{})
		})
		if status != 404 {
			t.Fatalf("calendar of B's spot: %d %v", status, body)
		}

		// Partners, payments and agreement quota: B's ids from A are 404.
		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{"name": "Hotel Miramare", "kind": "struttura_ricettiva"},
			func(w http.ResponseWriter, r *http.Request) { h.CreatePartner(w, r, est, api.CreatePartnerParams{}) })
		if status != 201 {
			t.Fatalf("create partner: %d %v", status, body)
		}
		pid := body["id"].(string)
		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{"amount": 1000, "method": "bank_transfer", "received_on": "2027-07-01"},
			func(w http.ResponseWriter, r *http.Request) { h.RecordPartnerPayment(w, r, est, pid, api.RecordPartnerPaymentParams{}) })
		if status != 201 {
			t.Fatalf("record payment: %d %v", status, body)
		}
		payID := body["id"].(string)
		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{
			"name": "Ospiti", "code": "OSPITI", "action": map[string]any{"type": "percent_off", "percent": "10"}, "channels": []string{"desk"},
			"quota": map[string]any{"max_per_day": 2},
		}, func(w http.ResponseWriter, r *http.Request) { h.CreateAgreement(w, r, est, api.CreateAgreementParams{}) })
		if status != 201 {
			t.Fatalf("create agreement: %d %v", status, body)
		}
		agID := body["id"].(string)
		ifMatch1 := `"1"`
		for _, step := range []struct {
			name string
			fn   func(w http.ResponseWriter, r *http.Request)
			body any
		}{
			{"get partner", func(w http.ResponseWriter, r *http.Request) { h.GetPartner(w, r, otherEst, pid) }, nil},
			{"update partner", func(w http.ResponseWriter, r *http.Request) { h.UpdatePartner(w, r, otherEst, pid, api.UpdatePartnerParams{IfMatch: &ifMatch1}) },
				map[string]any{"name": "X", "kind": "struttura_ricettiva"}},
			{"delete partner", func(w http.ResponseWriter, r *http.Request) { h.DeletePartner(w, r, otherEst, pid, api.DeletePartnerParams{IfMatch: &ifMatch1}) }, nil},
			{"list payments", func(w http.ResponseWriter, r *http.Request) { h.ListPartnerPayments(w, r, otherEst, pid, api.ListPartnerPaymentsParams{}) }, nil},
			{"void payment", func(w http.ResponseWriter, r *http.Request) {
				h.VoidPartnerPayment(w, r, otherEst, pid, payID, api.VoidPartnerPaymentParams{})
			}, map[string]any{"reason": "errore"}},
			{"agreement quota", func(w http.ResponseWriter, r *http.Request) {
				h.GetAgreementQuotaUsage(w, r, otherEst, agID, api.GetAgreementQuotaUsageParams{From: api.ReportFrom(calendar.MustParse("2027-07-01")), To: api.ReportTo(calendar.MustParse("2027-07-31"))})
			}, nil},
		} {
			status, _, body := callTarget(t, other, http.MethodPost, "/", step.body, step.fn)
			if status != 404 {
				t.Fatalf("%s from B: %d %v", step.name, status, body)
			}
		}
		// A payment of another partner of the same establishment is not
		// voided through this partner's URL either.
		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{"name": "Camping Pineta", "kind": "struttura_ricettiva"},
			func(w http.ResponseWriter, r *http.Request) { h.CreatePartner(w, r, est, api.CreatePartnerParams{}) })
		if status != 201 {
			t.Fatalf("create second partner: %d %v", status, body)
		}
		pid2 := body["id"].(string)
		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{"reason": "errore"},
			func(w http.ResponseWriter, r *http.Request) { h.VoidPartnerPayment(w, r, est, pid2, payID, api.VoidPartnerPaymentParams{}) })
		if status != 404 {
			t.Fatalf("void through another partner: %d %v", status, body)
		}
	})
}
