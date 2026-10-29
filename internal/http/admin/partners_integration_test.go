//go:build integration

package admin_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/partner"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/printout"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/internal/xlsx"
)

// TestPartnersHTTP_Flow covers the M8 partner handlers: partners and
// payments, the 409 of a partner in use, agreements with a partner target,
// the statement (json, html, xlsx), the report per partner and the
// agreements register (json, html, xlsx).
func TestPartnersHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		aw, ob := audit.NewWriter(f.Clock), event.NewOutbox(f.Clock)
		h := &admin.Handlers{
			Partners: partner.NewService(partner.Deps{DB: db, Clock: f.Clock, Audit: aw, Outbox: ob}),
			Reports:  report.NewService(report.Deps{DB: db, Clock: f.Clock}), Pricing: f.Pricing, Bookings: f.Svc,
			Log: slog.New(slog.DiscardHandler),
		}
		est := f.Est.ID
		status, hdr, body := callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{
			"name": "Hotel Miramare", "kind": "struttura_ricettiva", "accommodation_type": "hotel", "email": "info@miramare.it",
		}, func(w http.ResponseWriter, r *http.Request) { h.CreatePartner(w, r, est, api.CreatePartnerParams{}) })
		if status != 201 || hdr.Get("ETag") != `"1"` || body["name"] != "Hotel Miramare" {
			t.Fatalf("create partner: %d %v", status, body)
		}
		pid := body["id"].(string)

		status, _, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{
			"name": "Ospiti Miramare", "code": "MIRAMARE", "action": map[string]any{"type": "percent_off", "percent": "10"},
			"target": "partner", "partner_id": pid, "channels": []string{"desk", "online_link"},
			"quota": map[string]any{"reserved_per_day": 1}, "requirement": "conferma dell'hotel",
		}, func(w http.ResponseWriter, r *http.Request) {
			h.CreateAgreement(w, r, est, api.CreateAgreementParams{})
		})
		if status != 201 || body["public_link"] != "/b/lido-delfino?c=MIRAMARE" || body["partner"].(map[string]any)["name"] != "Hotel Miramare" ||
			body["quota"].(map[string]any)["release_days_before"].(float64) != 3 {
			t.Fatalf("create agreement: %d %v", status, body)
		}
		agID := body["id"].(string)
		status, _, body = callTarget(t, f.Owner, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.ListAgreementVersions(w, r, est, agID)
		})
		if status != 200 || len(body["items"].([]any)) != 1 {
			t.Fatalf("versions: %d %v", status, body)
		}

		in := f.Input(f.Spots[0], "2027-07-10", "2027-07-11")
		in.AgreementID = agID
		adults := 2
		in.Group = booking.GroupInput{GroupName: strp("Famiglia Bianchi"), PartyAdults: &adults, PartnerShare: &booking.PartnerShareInput{Type: booking.ShareFull}}
		d := f.MustCreate(in)
		status, _, body = callTarget(t, f.Owner, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetBooking(w, r, est, d.Booking.ID)
		})
		if status != 200 || body["partner"].(map[string]any)["id"] != pid || body["partner_amount"].(float64) != 3600 ||
			body["agreement"].(map[string]any)["requirement"] != "conferma dell'hotel" || body["group_name"] != "Famiglia Bianchi" {
			t.Fatalf("booking detail: %d %v", status, body)
		}

		// The verification answers with the updated booking (contract: BookingDetail + ETag).
		ifMatch := fmt.Sprintf(`"%d"`, d.Booking.Version)
		status, hdr, body = callTarget(t, f.Owner, http.MethodPost, "/", map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
			h.VerifyBookingRequirement(w, r, est, d.Booking.ID, api.VerifyBookingRequirementParams{IfMatch: &ifMatch})
		})
		rv, _ := body["requirement_verification"].(map[string]any)
		if status != 200 || body["id"] != d.Booking.ID || hdr.Get("ETag") != fmt.Sprintf(`"%d"`, d.Booking.Version+1) ||
			rv == nil || rv["requirement"] != "conferma dell'hotel" || rv["verified_by_name"] == nil {
			t.Fatalf("verify requirement: %d %s %v", status, hdr.Get("ETag"), body)
		}

		status, _, body = callTarget(t, f.Owner, http.MethodDelete, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.DeletePartner(w, r, est, pid, api.DeletePartnerParams{})
		})
		if status != 409 || body["type"] != "partner_in_use" {
			t.Fatalf("delete in use: %d %v", status, body)
		}

		status, _, body = callTarget(t, f.Cashier, http.MethodPost, "/", map[string]any{
			"amount": 1000, "received_on": "2027-07-12", "method": "bank_transfer", "reference": "CRO 42",
		}, func(w http.ResponseWriter, r *http.Request) {
			h.RecordPartnerPayment(w, r, est, pid, api.RecordPartnerPaymentParams{})
		})
		if status != 201 || body["amount"].(float64) != 1000 {
			t.Fatalf("record payment: %d %v", status, body)
		}

		from, to := calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-31")
		status, _, body = callTarget(t, f.Owner, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetPartnerStatement(w, r, est, pid, api.GetPartnerStatementParams{From: from, To: to})
		})
		totals, _ := body["totals"].(map[string]any)
		if status != 200 || totals["charged"].(float64) != 3600 || totals["paid"].(float64) != 1000 || totals["closing_balance"].(float64) != 2600 {
			t.Fatalf("statement json: %d %v", status, body)
		}
		html := "html"
		rec := httptest.NewRecorder()
		h.GetPartnerStatement(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Owner)), est, pid,
			api.GetPartnerStatementParams{From: from, To: to, Format: &html})
		out := rec.Body.String()
		if rec.Code != 200 || rec.Header().Get("Content-Security-Policy") != printout.ContentSecurityPolicy ||
			!strings.Contains(out, "Estratto conto") || !strings.Contains(out, "Famiglia Bianchi") || !strings.Contains(out, "26,00\u00a0€") {
			t.Fatalf("statement html: %d %s", rec.Code, out)
		}
		x := "xlsx"
		rec = httptest.NewRecorder()
		h.GetPartnerStatement(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Owner)), est, pid,
			api.GetPartnerStatementParams{From: from, To: to, Format: &x})
		if rec.Code != 200 || rec.Header().Get("Content-Type") != xlsx.MediaType || !strings.HasPrefix(rec.Body.String(), "PK") {
			t.Fatalf("statement xlsx: %d %v", rec.Code, rec.Header())
		}

		csv := api.ReportFormatCSV
		rec = httptest.NewRecorder()
		h.GetPartnersReport(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Owner)), est,
			api.GetPartnersReportParams{From: from, To: to, Format: &csv})
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Hotel Miramare;Struttura ricettiva;1;2;2;36,00;4,00;36,00;10,00") {
			t.Fatalf("partners csv: %d %q", rec.Code, rec.Body.String())
		}

		status, _, body = callTarget(t, f.Owner, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetAgreementsRegister(w, r, est, api.GetAgreementsRegisterParams{From: from, To: to})
		})
		if status != 200 || len(body["entries"].([]any)) != 1 || body["totals"].(map[string]any)["discount"].(float64) != 400 {
			t.Fatalf("register json: %d %v", status, body)
		}
		rec = httptest.NewRecorder()
		h.GetAgreementsRegister(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Owner)), est,
			api.GetAgreementsRegisterParams{From: from, To: to, Format: &html})
		out = rec.Body.String()
		if rec.Code != 200 || !strings.Contains(out, "Registro delle convenzioni") || !strings.Contains(out, "Strutture ricettive") ||
			!strings.Contains(out, "Firma del legale rappresentante") {
			t.Fatalf("register html: %d %s", rec.Code, out)
		}
		rec = httptest.NewRecorder()
		h.GetAgreementsRegister(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(f.Ctx, f.Owner)), est,
			api.GetAgreementsRegisterParams{From: from, To: to, Format: &x})
		if rec.Code != 200 || rec.Header().Get("Content-Type") != xlsx.MediaType {
			t.Fatalf("register xlsx: %d %v", rec.Code, rec.Header())
		}
		bad := "pdf"
		status, _, body = callTarget(t, f.Owner, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetAgreementsRegister(w, r, est, api.GetAgreementsRegisterParams{From: from, To: to, Format: &bad})
		})
		if status != 400 {
			t.Fatalf("unknown format: %d %v", status, body)
		}
		status, _, _ = callTarget(t, f.Cashier, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetAgreementsRegister(w, r, est, api.GetAgreementsRegisterParams{From: from, To: to})
		})
		if status != 403 {
			t.Fatalf("cashier reads the register: %d", status)
		}
	})
}
