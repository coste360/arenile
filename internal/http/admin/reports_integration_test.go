//go:build integration

package admin_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestReportsHTTP_Flow covers the report handlers end to end on a small
// fixture: JSON and CSV answers (format and Accept), validation, the
// permission matrix (viewer yes, cashier no) and tenant scoping.
func TestReportsHTTP_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Date(2027, 7, 10, 9, 0, 0, 0, time.UTC)
		orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
		if err != nil {
			t.Fatal(err)
		}
		orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", now)
		if err != nil {
			t.Fatal(err)
		}
		ins := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		a := estA.ID
		ins(&store.Season{ID: store.NewID(), EstablishmentID: a, Year: 2027, OpensOn: calendar.MustParse("2027-07-01"), ClosesOn: calendar.MustParse("2027-07-31"), Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now})
		m := &store.Map{ID: store.NewID(), EstablishmentID: a, Name: store.I18nText{"it": "S"}, Width: 10, Height: 10, GridSize: 1, SeaSide: "top", Version: 1, CreatedAt: now, UpdatedAt: now}
		ins(m)
		z := &store.Zone{ID: store.NewID(), EstablishmentID: a, MapID: &m.ID, Code: "Z1", Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "seat", Version: 1, CreatedAt: now, UpdatedAt: now}
		ins(z)
		sp := &store.Spot{
			ID: store.NewID(), EstablishmentID: a, MapID: m.ID, ZoneID: z.ID, Label: "A1", RowLabel: "A", Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1},
			Capacity: map[string]int{}, MaxExtra: map[string]int{}, Attributes: []string{}, Active: true, CreatedAt: now, UpdatedAt: now,
		}
		ins(sp)
		pk := &store.Pack{
			ID: store.NewID(), EstablishmentID: a, Code: "STD", Name: store.I18nText{"it": "Standard"}, Description: store.I18nText{}, Composition: map[string]int{},
			AllowedFormulas: []string{"day"}, Channels: []string{"desk"}, MaxPerBooking: 1, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		ins(pk)
		cu := &store.Customer{ID: store.NewID(), EstablishmentID: a, FirstName: "Mario", LastName: "Rossi", Email: strp("mario@example.test"), Phone: strp("+39333"), Locale: "it", Tags: []string{}, Version: 1, CreatedAt: now, UpdatedAt: now}
		ins(cu)
		day := calendar.MustParse("2027-07-10")
		b := &store.Booking{
			ID: store.NewID(), EstablishmentID: a, Code: "R1", CustomerID: cu.ID, Channel: "desk", Status: "confirmed", StartsOn: day, EndsOn: day,
			Slots: calendar.SlotsFull, Formula: "day", PriceSnapshot: store.JSON(`{"vat":[{"rate":22,"taxable":2049,"tax":451}]}`), TotalAmount: 2500,
			PaidAmount: 1000, PolicySnapshot: store.JSON("{}"), ConfirmedAt: &now, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		ins(b)
		line := &store.BookingLine{ID: store.NewID(), EstablishmentID: a, BookingID: b.ID, PackID: pk.ID, ZoneID: z.ID, SpotID: &sp.ID, AssignmentStatus: "assigned", Quantity: 1, LineTotal: 2500, CreatedAt: now, UpdatedAt: now}
		ins(line)
		for _, slot := range []calendar.Slot{calendar.AM, calendar.PM} {
			ins(&store.SpotAllocation{ID: store.NewID(), EstablishmentID: a, SpotID: sp.ID, Date: day, Slot: slot, Kind: "booking", BookingID: &b.ID, BookingLineID: &line.ID, CreatedAt: now})
		}
		tx, tax := money.VATSplit(1000, 2200)
		ins(&store.Payment{
			ID: store.NewID(), EstablishmentID: a, BookingID: &b.ID, CustomerID: &cu.ID, Provider: "manual", Method: "cash", Direction: "in", Amount: 1000, Status: "succeeded",
			VATBreakdown: []store.VATShare{{RateBP: 2200, Gross: 1000, Taxable: tx, Tax: tax}}, BusinessDate: day, Metadata: map[string]any{}, CreatedAt: now, UpdatedAt: now,
		})

		h := &admin.Handlers{Reports: report.NewService(report.Deps{DB: db, Clock: clock.NewFake(now)}), Log: slog.New(slog.DiscardHandler)}
		user := func(org, est string, role auth.Role) *auth.Principal {
			return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org, EstablishmentID: est, Role: role}}}
		}
		viewer, cashier, ownerB := user(orgA.ID, a, auth.RoleViewer), user(orgA.ID, a, auth.RoleCashier), user(orgB.ID, "", auth.RoleOwner)
		from, to := calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-31")
		csv := api.ReportFormatCSV

		status, hdr, body := callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetReportSummary(w, r, a, api.GetReportSummaryParams{From: from, To: to})
		})
		if status != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "application/json") {
			t.Fatalf("summary: %d %v", status, body)
		}
		if body["occupied_units"].(float64) != 2 || body["capacity_units"].(float64) != 62 || body["occupancy_bp"].(float64) != 322 ||
			body["revenue_collected"].(float64) != 1000 || body["revenue_booked"].(float64) != 2500 || body["open_balance_total"].(float64) != 1500 {
			t.Fatalf("summary body: %v", body)
		}
		if body["period"].(map[string]any)["days"].(float64) != 31 {
			t.Fatalf("period: %v", body["period"])
		}

		// CSV by format parameter.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, viewer))
		h.GetOpenBalancesReport(rec, req, a, api.GetOpenBalancesReportParams{From: from, To: to, Format: &csv})
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" ||
			rec.Header().Get("Content-Disposition") != `attachment; filename="saldi-aperti_2027-07-01_2027-07-31.csv"` {
			t.Fatalf("csv headers: %d %v", rec.Code, rec.Header())
		}
		out := rec.Body.String()
		if !strings.HasPrefix(out, "\xEF\xBB\xBFCodice;Cliente;Stato;Dal;Al;Totale;Pagato;Saldo\r\n") ||
			!strings.Contains(out, "R1;Mario Rossi;Confermata;10/07/2027;10/07/2027;25,00;10,00;15,00\r\n") {
			t.Fatalf("csv body: %q", out)
		}
		if strings.Contains(out, "example.test") || strings.Contains(out, "+39333") {
			t.Fatal("contact data leaked into the export")
		}
		// CSV by Accept header.
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, viewer))
		req.Header.Set("Accept", "text/csv")
		h.GetRevenueReport(rec, req, a, api.GetRevenueReportParams{From: from, To: to})
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "total;Totale;1;10,00;8,20;1,80;0,00\r\n") {
			t.Fatalf("revenue csv: %d %q", rec.Code, rec.Body.String())
		}

		// Comparison echoes the alignment.
		weekday := api.ComparisonAlignWeekday
		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetYearComparisonReport(w, r, a, api.GetYearComparisonReportParams{From: from, To: to, Align: &weekday})
		})
		if status != 200 || body["align"] != "weekday" || body["previous"].(map[string]any)["from"] != "2026-07-02" || len(body["metrics"].([]any)) != 10 {
			t.Fatalf("comparison: %d %v", status, body)
		}

		// Validation.
		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetReportSummary(w, r, a, api.GetReportSummaryParams{From: to, To: from})
		})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("reversed period: %d %v", status, body)
		}
		pack, collected := api.RevenueGroupBy("pack"), api.RevenueBasis("collected")
		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetRevenueReport(w, r, a, api.GetRevenueReportParams{From: from, To: to, GroupBy: &pack, Basis: &collected})
		})
		if status != 400 || body["type"] != "validation_failed" {
			t.Fatalf("pack on collected: %d %v", status, body)
		}
		zero := 0
		status, _, _ = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetTopCustomersReport(w, r, a, api.GetTopCustomersReportParams{From: from, To: to, Limit: &zero})
		})
		if status != 400 {
			t.Fatalf("limit 0: %d", status)
		}
		status, _, body = callTarget(t, viewer, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetTopCustomersReport(w, r, a, api.GetTopCustomersReportParams{From: from, To: to})
		})
		rows := body["rows"].([]any)
		if status != 200 || len(rows) != 1 || rows[0].(map[string]any)["display_name"] != "Mario Rossi" || rows[0].(map[string]any)["spot_days"].(float64) != 1 {
			t.Fatalf("top customers: %d %v", status, body)
		}
		if _, ok := rows[0].(map[string]any)["email"]; ok {
			t.Fatal("email in top customers")
		}

		// Permissions and scoping.
		status, _, _ = callTarget(t, cashier, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetOccupancyReport(w, r, a, api.GetOccupancyReportParams{From: from, To: to})
		})
		if status != 403 {
			t.Fatalf("cashier: %d", status)
		}
		status, _, _ = callTarget(t, ownerB, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetOccupancyReport(w, r, a, api.GetOccupancyReportParams{From: from, To: to})
		})
		if status != 404 {
			t.Fatalf("other tenant: %d", status)
		}
		status, _, body = callTarget(t, ownerB, http.MethodGet, "/", nil, func(w http.ResponseWriter, r *http.Request) {
			h.GetOccupancyReport(w, r, estB.ID, api.GetOccupancyReportParams{From: from, To: to})
		})
		if status != 200 || body["totals"].(map[string]any)["occupied_units"].(float64) != 0 {
			t.Fatalf("own empty establishment: %d %v", status, body)
		}
	})
}
