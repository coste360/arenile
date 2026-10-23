//go:build integration

package inventory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestQueries_Performance500Spots measures the operational queries on 500
// spots x 30 days with 60% occupancy on SQLite (target < 150 ms each, spec
// 25 / task M3). What is enforced does not depend on the machine: each query
// issues a bounded number of SQL statements, which a per-spot or per-day
// pattern (N+1) would multiply by hundreds. The measured times are logged;
// the 2s limit applies only with ARENILE_PERF_BUDGETS=1 (storetest.Perf).
func TestQueries_Performance500Spots(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	db, qc := storetest.CountQueries(storetest.SQLiteFile(t))
	f := newFixture(t, db)
	var spots []store.Spot
	for i := 0; i < 500; i++ {
		spots = append(spots, store.Spot{
			ID: store.NewID(), MapID: f.mapID, ZoneID: f.seat.ID, Label: fmt.Sprintf("P%03d", i), RowLabel: fmt.Sprintf("R%02d", i/25),
			Number: i % 25, Kind: "umbrella", Shape: store.Shape{Type: "circle", W: 1, H: 1}, Capacity: map[string]int{}, MaxExtra: map[string]int{},
			Attributes: []string{}, Active: true, CreatedAt: t0, UpdatedAt: t0,
		})
	}
	must(t, store.InsertSpots(ctx, db, f.es, spots))
	from, to := d("2027-07-01"), d("2027-07-30")
	var rows []store.SpotAllocation
	for i := 0; i < 300; i++ {
		tb := f.insertBooking(t, db, bookingSpec{from: "2027-07-01", to: "2027-07-30", lines: []lineSpec{{zone: f.seat, spot: &spots[i]}}})
		for _, day := range calendar.Range(from, to) {
			for _, sl := range []calendar.Slot{calendar.AM, calendar.PM} {
				rows = append(rows, store.SpotAllocation{
					ID: store.NewID(), SpotID: spots[i].ID, Date: day, Slot: sl, Kind: store.AllocationBooking,
					BookingID: ptr(tb.id), BookingLineID: ptr(tb.lineIDs[0]), CreatedAt: t0,
				})
			}
		}
	}
	n, err := store.InsertSpotAllocations(ctx, db, f.es, rows)
	must(t, err)
	if n != len(rows) {
		t.Fatalf("inserted %d of %d", n, len(rows))
	}
	// Query limits: measured 7, 8 and 12, plus headroom.
	measure := func(name string, maxQueries int, fn func() error) {
		t.Helper()
		must(t, fn()) // warm up
		storetest.Perf(t, qc, name, maxQueries, 2*time.Second, fn)
	}
	measure("planning 30 days", 10, func() error {
		pl, err := f.svc.GetPlanning(ctx, f.viewer, f.est.ID, inventory.PlanningQuery{From: from, To: to})
		if err == nil && len(pl.Rows) != 300 {
			err = fmt.Errorf("rows = %d", len(pl.Rows))
		}
		return err
	})
	measure("availability 30 days", 12, func() error {
		res, err := f.svc.GetAvailability(ctx, f.viewer, f.est.ID, inventory.AvailabilityQuery{From: from, To: to})
		if err == nil && res.Zones[0].Available != 205 {
			err = fmt.Errorf("available = %d", res.Zones[0].Available)
		}
		return err
	})
	measure("operations day", 16, func() error {
		_, err := f.svc.GetOperationsDay(ctx, f.viewer, f.est.ID, inventory.OperationsQuery{Date: d("2027-07-15")})
		return err
	})
}
