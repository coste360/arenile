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
	"github.com/coste360/arenile/internal/xlsx"
)

// sheetValues reads an .xlsx body as rows of "ref=value".
func sheetValues(t *testing.T, body []byte) [][]string {
	t.Helper()
	rows, err := xlsx.ReadSheet(body)
	if err != nil {
		t.Fatalf("read xlsx: %v", err)
	}
	out := make([][]string, len(rows))
	for i, r := range rows {
		for _, c := range r {
			out[i] = append(out[i], c.Ref+"="+c.Value)
		}
	}
	return out
}

// TestExportsHTTP_XLSX covers the XLSX exports (ADR 0019 F): a report as
// XLSX holds the same cells as its CSV with typed numbers and dates, the
// list exports stream in CSV and XLSX, test bookings and payments are left
// out, contact data never appears, the placeholder customer is skipped and
// the permission is reports.read.
func TestExportsHTTP_XLSX(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Date(2027, 7, 10, 9, 0, 0, 0, time.UTC)
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", now)
		if err != nil {
			t.Fatal(err)
		}
		ins := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		a := est.ID
		day := calendar.MustParse("2027-07-10")
		cust := func(first, last string, tags []string) *store.Customer {
			c := &store.Customer{
				ID: store.NewID(), EstablishmentID: a, FirstName: first, LastName: last, Email: strp(strings.ToLower(first) + "@example.test"),
				Phone: strp("+39333000111"), Locale: "it", Tags: tags, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
			ins(c)
			return c
		}
		mario := cust("Mario", "Rossi", []string{"vip"})
		evil := cust("=HYPERLINK(\"x\")", "Bianchi", []string{})
		cust("Online", "Placeholder", []string{"_online_placeholder"})
		booking := func(code string, c *store.Customer, test, pass bool, total, paid int64) *store.Booking {
			b := &store.Booking{
				ID: store.NewID(), EstablishmentID: a, Code: code, CustomerID: c.ID, Channel: "desk", Status: "confirmed", StartsOn: day, EndsOn: day,
				Slots: calendar.SlotsFull, Formula: "day", PriceSnapshot: store.JSON(`{}`), TotalAmount: 0, PolicySnapshot: store.JSON("{}"),
				ConfirmedAt: &now, Test: test, IsSeasonPass: pass, Version: 1, CreatedAt: now, UpdatedAt: now,
			}
			b.TotalAmount, b.PaidAmount = money.Cents(total), money.Cents(paid)
			if pass {
				b.Formula = "season"
			}
			ins(b)
			return b
		}
		realBk := booking("R1", mario, false, false, 2500, 1000)
		booking("T1", mario, true, false, 9900, 0)
		booking("P1", evil, false, true, 120000, 0)
		pay := func(b *store.Booking, test bool, amount int64) {
			ins(&store.Payment{
				ID: store.NewID(), EstablishmentID: a, BookingID: &b.ID, Provider: "manual", Method: "cash", Direction: "in", Amount: money.Cents(amount),
				Status: "succeeded", VATBreakdown: []store.VATShare{}, BusinessDate: day, Metadata: map[string]any{}, Test: test, CreatedAt: now, UpdatedAt: now,
			})
		}
		pay(realBk, false, 1000)
		pay(realBk, true, 777)

		h := &admin.Handlers{Reports: report.NewService(report.Deps{DB: db, Clock: clock.NewFake(now)}), Log: slog.New(slog.DiscardHandler)}
		user := func(role auth.Role) *auth.Principal {
			return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: org.ID, EstablishmentID: a, Role: role}}}
		}
		viewer, cashier := user(auth.RoleViewer), user(auth.RoleCashier)
		get := func(p *auth.Principal, fn func(w http.ResponseWriter, r *http.Request)) *httptest.ResponseRecorder {
			rec := httptest.NewRecorder()
			fn(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, p)))
			return rec
		}
		from, to := calendar.MustParse("2027-07-01"), calendar.MustParse("2027-07-31")

		// A report as XLSX: same cells as the CSV, typed.
		xf := api.ReportFormatXLSX
		rec := get(viewer, func(w http.ResponseWriter, r *http.Request) {
			h.GetOpenBalancesReport(w, r, a, api.GetOpenBalancesReportParams{From: from, To: to, Format: &xf})
		})
		if rec.Code != 200 || rec.Header().Get("Content-Type") != xlsx.MediaType ||
			rec.Header().Get("Content-Disposition") != `attachment; filename="saldi-aperti_2027-07-01_2027-07-31.xlsx"` {
			t.Fatalf("xlsx report headers: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
		}
		rows := sheetValues(t, rec.Body.Bytes())
		if strings.Join(rows[0], "|") != "A1=Codice|B1=Cliente|C1=Stato|D1=Dal|E1=Al|F1=Totale|G1=Pagato|H1=Saldo" {
			t.Fatalf("xlsx header %v", rows[0])
		}
		joined := ""
		for _, r := range rows {
			joined += strings.Join(r, "|") + "\n"
		}
		if !strings.Contains(joined, "A3=R1|B3=Mario Rossi|C3=Confermata|D3=46578|E3=46578|F3=25.00|G3=10.00|H3=15.00") {
			t.Fatalf("xlsx report rows:\n%s", joined)
		}
		if strings.Contains(joined, "T1") {
			t.Fatal("test booking in the report export")
		}
		// Accept header selects XLSX too.
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(auth.WithPrincipal(ctx, viewer))
		req.Header.Set("Accept", xlsx.MediaType)
		h.GetOpenBalancesReport(rec, req, a, api.GetOpenBalancesReportParams{From: from, To: to})
		if rec.Header().Get("Content-Type") != xlsx.MediaType {
			t.Fatalf("accept xlsx: %v", rec.Header())
		}

		// List exports.
		list := func(p *auth.Principal, kind api.ExportListKind, format api.ExportFormat, from, to *calendar.Date) *httptest.ResponseRecorder {
			return get(p, func(w http.ResponseWriter, r *http.Request) {
				h.ExportList(w, r, a, kind, api.ExportListParams{Format: &format, From: from, To: to})
			})
		}
		csvBody := func(rec *httptest.ResponseRecorder) string {
			t.Helper()
			if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" {
				t.Fatalf("csv list: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
			}
			return rec.Body.String()
		}
		cs := csvBody(list(viewer, api.ExportListCustomers, api.ExportFormatCSV, nil, nil))
		if !strings.HasPrefix(cs, "\xEF\xBB\xBFCliente;Lingua;Etichette;Consenso marketing;Cliente dal\r\n") ||
			!strings.Contains(cs, "Mario Rossi;it;vip;No;10/07/2027\r\n") || strings.Contains(cs, "Placeholder") {
			t.Fatalf("customers csv: %q", cs)
		}
		if !strings.Contains(cs, "\"'=HYPERLINK(\"\"x\"\") Bianchi\"") {
			t.Fatalf("formula not neutralised: %q", cs)
		}
		bs := csvBody(list(viewer, api.ExportListBookings, api.ExportFormatCSV, &from, &to))
		if !strings.Contains(bs, "R1;Mario Rossi;Confermata;Cassa;10/07/2027;10/07/2027;Giornata intera;Giornaliero;No;25,00;10,00;15,00;10/07/2027") ||
			strings.Contains(bs, "T1") {
			t.Fatalf("bookings csv: %q", bs)
		}
		ps := csvBody(list(viewer, api.ExportListPayments, api.ExportFormatCSV, &from, &to))
		if !strings.Contains(ps, "10/07/2027;R1;Incasso;Contanti;manual;Riuscito;10,00") || strings.Contains(ps, "7,77") {
			t.Fatalf("payments csv: %q", ps)
		}
		for _, body := range []string{cs, bs, ps} {
			if strings.Contains(body, "example.test") || strings.Contains(body, "+39333") {
				t.Fatal("contact data leaked into a list export")
			}
		}

		rec = list(viewer, api.ExportListSeasonPasses, api.ExportFormatXLSX, nil, nil)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != xlsx.MediaType || rec.Header().Get("Content-Disposition") != `attachment; filename="abbonamenti.xlsx"` {
			t.Fatalf("passes xlsx: %d %v", rec.Code, rec.Header())
		}
		xr, err := xlsx.ReadSheet(rec.Body.Bytes())
		if err != nil || len(xr) != 2 {
			t.Fatalf("passes xlsx rows: %v %d", err, len(xr))
		}
		if xr[1][1].Type != "inlineStr" || xr[1][1].Value != `=HYPERLINK("x") Bianchi` || xr[1][6].Ref != "H2" || xr[1][6].Value != "1200.00" {
			t.Fatalf("passes xlsx cells: %+v", xr[1])
		}

		// Permission and validation.
		if rec := list(cashier, api.ExportListCustomers, api.ExportFormatCSV, nil, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("cashier export: %d", rec.Code)
		}
		far := calendar.MustParse("2030-01-01")
		if rec := list(viewer, api.ExportListBookings, api.ExportFormatCSV, &from, &far); rec.Code != http.StatusBadRequest {
			t.Fatalf("range too long: %d %s", rec.Code, rec.Body.String())
		}
	})
}
