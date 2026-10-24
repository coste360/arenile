//go:build integration

package acceptance

import (
	"context"
	"net/http"
	"testing"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/store"
)

// TestAcceptance_TestMode covers the test mode end to end (spec 15.4, ADR
// 0019, INV-74): while it is on, desk bookings and their payments are marked
// test, stay out of the corrispettivi, the report summary and the money KPIs
// of the day, the public page books only with the preview link, and the
// bulk purge removes exactly the test rows (allocations, payments, fiscal
// documents) leaving the real ones and the audit trail.
func TestAcceptance_TestMode(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		d := newDay(t, e, 15_4)
		d.setVAT(2200)
		d.openCash(10000)
		d.createCustomers(2)

		r := d.c.must(d.c.get("getTestMode", e.path("/test-mode")), "get test mode")
		tm := decode[api.TestMode](t, r)
		if tm.Enabled || tm.TestBookingsCount != 0 || r.ETag != `"1"` {
			t.Fatalf("initial test mode %+v etag %s", tm, r.ETag)
		}
		// The preview link needs test mode on.
		if r := d.c.post("createTestModePreviewLink", e.path("/test-mode/preview-link"), nil); r.Status != http.StatusConflict {
			t.Fatalf("preview link while off: %s", r)
		}

		day := d.date(1)
		realBk, rr := d.createBooking("createBooking", d.bookingInput(e.dailySpots[0].ID, day, day, api.SlotsFULL))
		if realBk == nil {
			t.Fatalf("real booking: %s", rr)
		}
		d.c.must(d.pay(realBk, realBk.total, api.PaymentMethodCash), "pay real")

		// Switch on (If-Match), audited.
		on := true
		r = d.c.must(d.c.do("updateTestMode", http.MethodPut, e.path("/test-mode"), api.TestModeInput{Enabled: on}, ifMatch(tm.Version)), "enable")
		tm = decode[api.TestMode](t, r)
		if !tm.Enabled || tm.PublicEnabled || tm.EnabledAt == nil || tm.Version != 2 {
			t.Fatalf("enabled test mode %+v", tm)
		}
		if r := d.c.do("updateTestMode", http.MethodPut, e.path("/test-mode"), api.TestModeInput{Enabled: on}, ifMatch(1)); r.Status != http.StatusPreconditionFailed {
			t.Fatalf("stale If-Match: %s", r)
		}

		test, rr := d.createBooking("createBooking", d.bookingInput(e.dailySpots[1].ID, day, day, api.SlotsFULL))
		if test == nil {
			t.Fatalf("test booking: %s", rr)
		}
		d.c.must(d.pay(test, test.total, api.PaymentMethodCash), "pay test")
		detail := decode[api.BookingDetail](t, d.c.must(d.c.get("getBooking", e.path("/bookings/"+test.id)), "test detail"))
		if detail.Test == nil || !*detail.Test || len(detail.Payments) != 1 || detail.Payments[0].Test == nil || !*detail.Payments[0].Test {
			t.Fatalf("test booking not marked: test=%v payments=%+v", detail.Test, detail.Payments)
		}
		realDetail := decode[api.BookingDetail](t, d.c.must(d.c.get("getBooking", e.path("/bookings/"+realBk.id)), "real detail"))
		if realDetail.Test != nil {
			t.Fatalf("real booking marked test")
		}
		list := decode[api.BookingList](t, d.c.must(d.c.get("listBookings", e.path("/bookings?test=true")), "list test"))
		if len(list.Items) != 1 || list.Items[0].Id != test.id || list.Items[0].Test == nil || !*list.Items[0].Test {
			t.Fatalf("listBookings?test=true = %+v", list.Items)
		}

		// INV-74: corrispettivi, summary report and the day KPIs ignore the test rows.
		q := "?from=" + d.e.today.String() + "&to=" + d.e.today.AddDays(30).String()
		corr := decode[api.CorrispettiviReport](t, d.c.must(d.c.get("getCorrispettivi", e.path("/fiscal/corrispettivi"+q)), "corrispettivi"))
		if corr.GrandTotal != realBk.total {
			t.Fatalf("corrispettivi grand total %d, want only the real payment %d", corr.GrandTotal, realBk.total)
		}
		sum := decode[api.ReportSummary](t, d.c.must(d.c.get("getReportSummary", e.path("/reports/summary"+q)), "summary"))
		if sum.RevenueCollected != realBk.total {
			t.Fatalf("summary revenue %d, want %d", sum.RevenueCollected, realBk.total)
		}
		var fiscalDocs int
		if err := e.db.NewSelect().TableExpr("fiscal_documents AS fd").ColumnExpr("COUNT(*)").
			Join("JOIN payments AS p ON p.id = fd.payment_id").Where("p.booking_id = ?", test.id).Scan(ctx, &fiscalDocs); err != nil || fiscalDocs != 0 {
			t.Fatalf("fiscal documents of the test payment: %d %v", fiscalDocs, err)
		}

		// Public page: without the preview link nobody books in test mode.
		pub := newClient(t, e.srv.URL)
		body := api.PublicBookingCreate{StartsOn: day, EndsOn: day, Slots: api.SlotsFULL, Formula: api.FormulaDay, Lines: []api.PublicLineInput{}}
		if r := pub.post("createPublicBooking", d.pubPath("/bookings"), body); r.Status != http.StatusForbidden || r.Problem.Type != "test_mode_preview_required" {
			t.Fatalf("public create without preview: %s", r)
		}
		link := decode[api.TestModePreviewLink](t, d.c.must(d.c.post("createTestModePreviewLink", e.path("/test-mode/preview-link"), nil), "preview link"))
		if r := pub.do("createPublicBooking", http.MethodPost, d.pubPath("/bookings"), body, map[string]string{"X-Arenile-Preview": link.Token}); r.Problem.Type == "test_mode_preview_required" {
			t.Fatalf("public create with preview still refused: %s", r)
		}
		if r := pub.do("createPublicBooking", http.MethodPost, d.pubPath("/bookings"), body, map[string]string{"X-Arenile-Preview": link.Token + "x"}); r.Problem.Type != "test_mode_preview_required" {
			t.Fatalf("tampered preview token accepted: %s", r)
		}

		// A real booking paid online with Stripe in test mode carries a test
		// payment (gateway: payments.test = provider mode test). The purge
		// never deletes it, so the dry run must not count it either.
		if _, err := e.db.NewUpdate().TableExpr("payments").Set("test = ?", true).
			Where("booking_id = ?", realBk.id).Exec(ctx); err != nil {
			t.Fatalf("mark the real payment as a Stripe test-mode payment: %v", err)
		}
		// Purge: dry run counts, then deletes only the test rows.
		res := decode[api.TestBookingsPurgeResult](t, d.c.must(d.c.post("purgeTestBookings", e.path("/test-bookings/purge"), api.TestBookingsPurgeRequest{DryRun: &on}), "dry run"))
		if res.Deleted || res.Bookings != 1 || res.Payments != 1 {
			t.Fatalf("dry run %+v", res)
		}
		res = decode[api.TestBookingsPurgeResult](t, d.c.must(d.c.post("purgeTestBookings", e.path("/test-bookings/purge"), api.TestBookingsPurgeRequest{}), "purge"))
		if !res.Deleted || res.Bookings != 1 || res.Payments != 1 {
			t.Fatalf("purge %+v", res)
		}
		if r := d.c.get("getBooking", e.path("/bookings/"+test.id)); r.Status != http.StatusNotFound {
			t.Fatalf("purged booking still readable: %s", r)
		}
		d.c.must(d.c.get("getBooking", e.path("/bookings/"+realBk.id)), "real booking after purge")
		var realPayments int
		if err := e.db.NewSelect().TableExpr("payments").ColumnExpr("COUNT(*)").Where("booking_id = ?", realBk.id).Scan(ctx, &realPayments); err != nil || realPayments != 1 {
			t.Fatalf("payments of the real booking after purge: %d %v", realPayments, err)
		}
		var left int
		if err := e.db.NewSelect().TableExpr("spot_allocations").ColumnExpr("COUNT(*)").Where("booking_id = ?", test.id).Scan(ctx, &left); err != nil || left != 0 {
			t.Fatalf("allocations of the purged booking: %d %v", left, err)
		}
		var audits int
		if err := e.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").
			Where("establishment_id = ?", e.estID).Where("action = ?", "test_bookings.purged").Scan(ctx, &audits); err != nil || audits != 1 {
			t.Fatalf("purge audit entries: %d %v", audits, err)
		}
		var createdAudit int
		if err := e.db.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").
			Where("establishment_id = ?", e.estID).Where("entity_id = ?", test.id).Scan(ctx, &createdAudit); err != nil || createdAudit == 0 {
			t.Fatalf("audit trail of the purged booking must stay: %d %v", createdAudit, err)
		}
		tm = decode[api.TestMode](t, d.c.must(d.c.get("getTestMode", e.path("/test-mode")), "after purge"))
		if tm.TestBookingsCount != 0 || !tm.Enabled {
			t.Fatalf("after purge %+v", tm)
		}
		// The spot of the purged booking is free again: a new booking takes it.
		off := false
		d.c.must(d.c.do("updateTestMode", http.MethodPut, e.path("/test-mode"), api.TestModeInput{Enabled: off}, ifMatch(tm.Version)), "disable")
		again, rr := d.createBooking("createBooking", d.bookingInput(e.dailySpots[1].ID, day, day, api.SlotsFULL))
		if again == nil {
			t.Fatalf("spot of the purged booking not free: %s", rr)
		}
		var isTest bool
		if err := e.db.NewSelect().Model((*store.Booking)(nil)).Column("test").Where("id = ?", again.id).Scan(ctx, &isTest); err != nil || isTest {
			t.Fatalf("booking after switching off marked test=%v %v", isTest, err)
		}
	})
}
