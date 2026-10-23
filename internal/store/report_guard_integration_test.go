//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/uptrace/bun/dialect"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestReportQueries_TestBookingsFlagNotYetFiltered guards the deviation
// recorded in ADR 0016: spec 15.4 excludes test bookings from reports and
// exports, but the bookings table has no test flag yet, so the queries of
// report.go do not filter it. The day a column such as bookings.test or
// bookings.is_test lands, this test fails until report.go filters on it
// (then update reportFiltersTestBookings and this test together).
func TestReportQueries_TestBookingsFlagNotYetFiltered(t *testing.T) {
	const reportFiltersTestBookings = false
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		var cols []string
		var err error
		if db.Dialect().Name() == dialect.PG {
			err = db.NewRaw("SELECT column_name FROM information_schema.columns WHERE table_name = 'bookings'").Scan(ctx, &cols)
		} else {
			var rows []struct {
				Name string `bun:"name"`
			}
			err = db.NewRaw("SELECT name FROM pragma_table_info('bookings')").Scan(ctx, &rows)
			for _, r := range rows {
				cols = append(cols, r.Name)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(cols) == 0 {
			t.Fatal("bookings columns not found")
		}
		for _, c := range cols {
			if (c == "test" || c == "is_test" || c == "test_mode") && !reportFiltersTestBookings {
				t.Fatalf("bookings.%s exists: internal/store/report.go must exclude test bookings from every report (spec 15.4)", c)
			}
		}
	})
}
