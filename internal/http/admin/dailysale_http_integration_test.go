//go:build integration

package admin_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/ticketassign"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// newDailySaleHandlers wires the admin handlers of the daily sale on the
// bookingtest lido with the real services (as internal/http/dailysale_wiring.go).
func newDailySaleHandlers(t *testing.T, db *store.DB) (*admin.Handlers, *bookingtest.Fixture) {
	t.Helper()
	f := bookingtest.New(t, db)
	c := f.Clock
	c.Set(time.Date(2027, 7, 10, 7, 0, 0, 0, time.UTC))
	aw, ob := audit.NewWriter(c), event.NewOutbox(c)
	reg := app.NewRegistry(c, ext.Extensions{})
	inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cust := customer.NewService(customer.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	cs := cash.NewService(cash.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
	fs := fiscal.NewService(fiscal.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Drivers: reg})
	pay := payment.NewService(payment.Deps{
		DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}),
		Cash: cs, Fiscal: fs, Credits: cust, Providers: reg,
	})
	bk := booking.NewService(booking.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: f.Pricing, Customers: cust, Payments: pay})
	pay.SetConfirmer(bk)
	tk := ticketing.NewService(ticketing.Deps{
		DB: db, Clock: c, Audit: aw, Bookings: bk, BookingsIn: func(db bun.IDB) ticketing.Bookings { return bk.WithDB(db) },
	})
	sales := dailysale.NewService(dailysale.Deps{
		DB: db, Clock: c, Audit: aw, Bookings: bk, Payments: dailysale.PaymentRecorder{Svc: pay}, Tickets: tk, Customers: cust,
		AvailabilityIn: func(db bun.IDB) dailysale.Availability { return inv.WithDB(db) },
	})
	h := &admin.Handlers{
		Bookings: bk, Payments: pay, Cash: cs, Fiscal: fs, Tickets: tk, DailySale: sales,
		TicketAssign: ticketassign.NewService(ticketassign.Deps{DB: db, Clock: c, Bookings: bk}),
		Log:          slog.New(slog.DiscardHandler),
	}
	return h, f
}

func TestDailySaleHTTP_SaleListPrintVoid(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		h, f := newDailySaleHandlers(t, db)
		est := f.Est.ID
		date := bookingtest.D("2027-07-10")
		sale := api.DailySaleCreate{
			Date: date, Slots: api.Slots("FULL"), SpotId: &f.Spots[0].ID, PackId: f.Base.ID,
			Payment: api.DailySalePayment{Method: api.TicketPaymentMethod("cash")},
			Holder:  &api.DailySaleHolder{LastName: strp("Bianchi")},
		}
		create := func(p *auth.Principal, in api.DailySaleCreate) (int, map[string]any) {
			status, _, body := call(t, p, http.MethodPost, in, func(w http.ResponseWriter, r *http.Request) {
				h.CreateDailySale(w, r, est, api.CreateDailySaleParams{})
			})
			return status, body
		}
		// Cash without an open session: 409 cash_session_closed.
		if status, body := create(f.Cashier, sale); status != 409 || body["type"] != "cash_session_closed" {
			t.Fatalf("no session: %d %v", status, body)
		}
		if status, _, body := call(t, f.Cashier, http.MethodPost, api.CashSessionOpen{OpeningFloat: 0}, func(w http.ResponseWriter, r *http.Request) {
			h.OpenCashSession(w, r, est, api.OpenCashSessionParams{})
		}); status != 201 {
			t.Fatalf("open: %d %v", status, body)
		}
		// Quote.
		status, _, body := call(t, f.Cashier, http.MethodPost, api.DailySaleQuoteRequest{Date: date, Slots: "FULL", ZoneId: &f.SeatZone.ID, PackId: f.Base.ID},
			func(w http.ResponseWriter, r *http.Request) { h.QuoteDailySale(w, r, est, api.QuoteDailySaleParams{}) })
		if status != 200 || body["total"] != float64(bookingtest.BasePerDay) || body["zone_free"] != float64(4) || body["available"] != true {
			t.Fatalf("quote: %d %v", status, body)
		}
		status, body = create(f.Cashier, sale)
		if status != 201 || body["payment_id"] == "" {
			t.Fatalf("create: %d %v", status, body)
		}
		ticket := body["ticket"].(map[string]any)
		if ticket["number_label"] != "#001" || ticket["holder_name"] != "Bianchi" || ticket["state"] != "assigned" || ticket["spot_label"] != "A1" {
			t.Fatalf("ticket %v", ticket)
		}
		id := ticket["id"].(string)
		// Taken spot: 409 spot_unavailable.
		if status, body := create(f.Cashier, sale); status != 409 || body["type"] != "spot_unavailable" {
			t.Fatalf("taken: %d %v", status, body)
		}
		// Zone full: zone-mode zone without pool capacity left.
		// Print data.
		status, _, body = call(t, f.Viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetTicketPrint(w, r, est, id, api.GetTicketPrintParams{})
		})
		// The QR carries the random gate token, never the sequential booking code.
		qr, _ := body["qr_payload"].(string)
		if status != 200 || body["format"] != "thermal_80" || body["reprint"] != false || len(qr) < 20 || qr == ticket["booking_code"] {
			t.Fatalf("print: %d %v", status, body)
		}
		// Day list as JSON and CSV.
		status, _, body = call(t, f.Viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListTickets(w, r, est, api.ListTicketsParams{Date: date})
		})
		summary, _ := body["summary"].(map[string]any)
		if status != 200 || len(body["items"].([]any)) != 1 || summary["issued"] != float64(1) || summary["total_amount"] != float64(bookingtest.BasePerDay) {
			t.Fatalf("list: %d %v", status, body)
		}
		// Void: If-Match with the ticket version is mandatory (428), a stale
		// version is 412 (INV-80), then a cash refund (manager) and 409
		// ticket_voided on the second attempt.
		voidWith := func(ifMatch string, reason string) (int, map[string]any) {
			status, _, body := call(t, f.Manager, http.MethodPost, api.TicketVoidRequest{Reason: reason}, func(w http.ResponseWriter, r *http.Request) {
				h.VoidTicket(w, r, est, id, api.VoidTicketParams{IfMatch: ifMatch})
			})
			return status, body
		}
		if status, body := voidWith("", "errore"); status != 428 || body["type"] != "precondition_required" {
			t.Fatalf("void without If-Match: %d %v", status, body)
		}
		if status, body := voidWith(`"9"`, "errore"); status != 412 || body["type"] != "version_conflict" {
			t.Fatalf("void with a stale version: %d %v", status, body)
		}
		status, body = voidWith(`"1"`, "errore")
		if status != 200 || body["state"] != "voided" || body["ticket_number"] != float64(1) || body["refund_payment_id"] == nil {
			t.Fatalf("void: %d %v", status, body)
		}
		if status, body := voidWith("*", "ancora"); status != 409 || body["type"] != "ticket_voided" {
			t.Fatalf("second void: %d %v", status, body)
		}
		format := api.ReportFormatCSV
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(t.Context(), f.Viewer))
		w := httptest.NewRecorder()
		h.ListTickets(w, r, est, api.ListTicketsParams{Date: date, Format: &format})
		csv := w.Body.String()
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(csv, "\xEF\xBB\xBF") ||
			!strings.Contains(csv, "#001;2027-07-10;") || !strings.Contains(csv, ";annullato;20,00;contante;") {
			t.Fatalf("csv %d %q", w.Code, csv)
		}
		// Settings: defaults, then saved with If-Match.
		status, hdr, body := call(t, f.Viewer, http.MethodGet, nil, func(w http.ResponseWriter, r *http.Request) { h.GetDailySaleSettings(w, r, est) })
		if status != 200 || hdr.Get("ETag") != `"1"` || body["print_format"] != "thermal_80" {
			t.Fatalf("settings: %d %v", status, body)
		}
		v1 := `"1"`
		status, hdr, body = call(t, f.Manager, http.MethodPut, api.DailySaleSettingsUpdate{PrintFormat: "thermal_58", AutoPrint: false, ZoneFirstEnabled: true, FooterText: api.I18nText{"it": "Grazie!"}},
			func(w http.ResponseWriter, r *http.Request) {
				h.UpdateDailySaleSettings(w, r, est, api.UpdateDailySaleSettingsParams{IfMatch: &v1})
			})
		if status != 200 || hdr.Get("ETag") != `"2"` || body["print_format"] != "thermal_58" {
			t.Fatalf("update settings: %d %v", status, body)
		}
		if status, _, body := call(t, f.Manager, http.MethodPut, api.DailySaleSettingsUpdate{PrintFormat: "a4", FooterText: api.I18nText{}},
			func(w http.ResponseWriter, r *http.Request) {
				h.UpdateDailySaleSettings(w, r, est, api.UpdateDailySaleSettingsParams{IfMatch: &v1})
			}); status != 412 {
			t.Fatalf("stale settings: %d %v", status, body)
		}
	})
}
