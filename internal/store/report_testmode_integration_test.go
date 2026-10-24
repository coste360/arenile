//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestReportQueries_TestBookingsExcluded checks that the report queries of
// report.go ignore bookings and payments created in test mode (spec 15.4,
// migration 00009): a real and a test booking with the same data, only the
// real one (and its payment) is reported.
func TestReportQueries_TestBookingsExcluded(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-tm", "Lido TM", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		insert := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		d := calendar.MustParse
		cust := &store.Customer{ID: store.NewID(), EstablishmentID: est.ID, LastName: "Rossi", Locale: "it", Tags: []string{}, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		insert(cust)
		ids := map[bool]string{}
		for i, test := range []bool{false, true} {
			bk := &store.Booking{
				ID: store.NewID(), EstablishmentID: est.ID, Code: []string{"AR-REAL1", "AR-TEST1"}[i], CustomerID: cust.ID,
				Channel: "desk", Status: "confirmed", StartsOn: d("2027-07-10"), EndsOn: d("2027-07-10"), Slots: calendar.SlotsFull,
				Formula: "day", PriceSnapshot: store.JSON(`{}`), PolicySnapshot: store.JSON(`{}`), TotalAmount: 5000, PaidAmount: 2000,
				ConfirmedAt: &t0, Test: test, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
			insert(bk)
			ids[test] = bk.ID
			insert(&store.Payment{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: &bk.ID, CustomerID: &cust.ID, Provider: "manual",
				Method: store.PaymentMethodCash, Direction: store.PaymentDirectionIn, Amount: 2000, Status: store.PaymentSucceeded,
				BusinessDate: d("2027-07-01"), VATBreakdown: []store.VATShare{}, Metadata: map[string]any{}, Test: test,
				CreatedAt: t0, UpdatedAt: t0,
			})
		}

		from, to := d("2027-07-01"), d("2027-07-31")
		pays, err := store.ReportPayments(ctx, db, es, from, to)
		if err != nil || len(pays) != 1 || pays[0].BookingID == nil || *pays[0].BookingID != ids[false] {
			t.Fatalf("payments = %+v, %v", pays, err)
		}
		bks, err := store.ReportBookings(ctx, db, es, store.ReportBookingWindow{
			CreatedFrom: t0.Add(-time.Hour), CreatedTo: t0.Add(time.Hour), StartsFrom: from, StartsTo: to,
		})
		if err != nil || len(bks) != 1 || bks[0].ID != ids[false] {
			t.Fatalf("bookings = %+v, %v", bks, err)
		}
		open, err := store.ReportOpenBalances(ctx, db, es, from, to)
		if err != nil || len(open) != 1 || open[0].BookingID != ids[false] {
			t.Fatalf("open balances = %+v, %v", open, err)
		}
		top, err := store.ReportTopCustomers(ctx, db, es, from, to, false, 10)
		if err != nil || len(top) != 1 || top[0].Bookings != 1 {
			t.Fatalf("top customers = %+v, %v", top, err)
		}
	})
}
