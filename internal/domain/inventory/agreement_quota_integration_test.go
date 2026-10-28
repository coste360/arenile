//go:build integration

package inventory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// Agreement quota (M8, INV-103): the cap of an agreement and the capacity
// reserved to it, enforced by the database counters and the recount after
// the insert.

type quotaSpec struct {
	max, reserved, release *int
	zones                  []string
}

func (f *fixture) addQuotaAgreement(t testing.TB, q quotaSpec) string {
	t.Helper()
	cfg := map[string]any{}
	if q.max != nil {
		cfg["max_per_day"] = *q.max
	}
	if q.reserved != nil {
		cfg["reserved_per_day"] = *q.reserved
	}
	if q.release != nil {
		cfg["release_days_before"] = *q.release
	}
	if len(q.zones) > 0 {
		cfg["zone_ids"] = q.zones
	}
	raw, err := json.Marshal(cfg)
	must(t, err)
	id := store.NewID()
	code := "AG" + id[len(id)-8:] // the random tail: two agreements in the same millisecond share the prefix
	ag := &store.Agreement{
		ID: store.NewID(), Code: &code, Name: "Residenti", Action: store.JSON(`{"type":"percent_off","percent":"10"}`),
		Conditions: store.JSON(`{}`), Active: true, Quota: store.JSON(raw), Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}
	must(t, store.InsertAgreement(context.Background(), f.db, f.es, ag))
	return ag.ID
}

// bookAs books spot on day with or without the agreement.
func (f *fixture) bookAs(t testing.TB, agreementID, spot, day string) (testBooking, error) {
	t.Helper()
	spec := bookingSpec{from: day, to: day, lines: []lineSpec{{zone: f.seat, spot: f.spots[spot]}}}
	var tb testBooking
	err := store.RunInTx(context.Background(), f.db, func(ctx context.Context, tx bun.Tx) error {
		tb = f.insertBooking(t, tx, spec)
		req := f.allocRequest(spec, tb, nil)
		req.AgreementID = agreementID
		_, err := f.svc.Allocate(ctx, tx, f.es, req)
		return err
	})
	return tb, err
}

func quotaReason(err error) string {
	var ce *availability.ConflictError
	if !errors.As(err, &ce) || len(ce.Conflicts) == 0 {
		return ""
	}
	if !errors.Is(ce.Err, availability.ErrAgreementQuotaExceeded) {
		return "other:" + ce.Conflicts[0].Reason
	}
	return ce.Conflicts[0].Reason
}

func (f *fixture) quotaUsed(t testing.TB, agreementID, day string) int {
	t.Helper()
	used, err := store.AgreementUsedOn(context.Background(), f.db, f.es, []string{agreementID}, d(day), d(day))
	must(t, err)
	return used[store.QuotaKey{AgreementID: agreementID, Date: d(day), Slot: calendar.AM}]
}

func TestAgreementQuota_ReservedBlocksOtherSales(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		ag := f.addQuotaAgreement(t, quotaSpec{reserved: ptr(2), zones: []string{f.seat.ID}})
		day := "2027-07-15" // released on 2027-07-12, today is 2027-07-01
		for _, s := range []string{"A1", "A2", "A3"} {
			if _, err := f.bookAs(t, "", s, day); err != nil {
				t.Fatalf("%s outside the agreement: %v", s, err)
			}
		}
		_, err := f.bookAs(t, "", "A4", day)
		if got := quotaReason(err); got != availability.ReasonAgreementReserved {
			t.Fatalf("A4 outside the agreement: %v (%s), want agreement_reserved", err, got)
		}
		if _, err := f.bookAs(t, ag, "A4", day); err != nil {
			t.Fatalf("A4 in the agreement: %v", err)
		}
		if _, err := f.bookAs(t, ag, "A5", day); err != nil {
			t.Fatalf("A5 in the agreement: %v", err)
		}
		if used := f.quotaUsed(t, ag, day); used != 2 {
			t.Fatalf("used = %d, want 2", used)
		}
		f.checkInvariants(t)
	})
}

func TestAgreementQuota_ReleasedBeforeDate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.addQuotaAgreement(t, quotaSpec{reserved: ptr(2), release: ptr(3), zones: []string{f.seat.ID}})
		day := "2027-07-04" // release_on 2027-07-01 = today: back on general sale
		for _, s := range []string{"A1", "A2", "A3", "A4", "A5"} {
			if _, err := f.bookAs(t, "", s, day); err != nil {
				t.Fatalf("%s after the release: %v", s, err)
			}
		}
		// The day after the release window is still reserved.
		_, err := f.bookAs(t, "", "A5", "2027-07-05")
		if err != nil {
			t.Fatalf("A5 on 07-05 (4 spots still free): %v", err)
		}
		for _, s := range []string{"A1", "A2"} {
			if _, err := f.bookAs(t, "", s, "2027-07-05"); err != nil {
				t.Fatalf("%s on 07-05: %v", s, err)
			}
		}
		if _, err := f.bookAs(t, "", "A3", "2027-07-05"); quotaReason(err) != availability.ReasonAgreementReserved {
			t.Fatalf("A3 on 07-05 must hit the reserve: %v", err)
		}
	})
}

func TestAgreementQuota_MaxPerDayAndRelease(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		ag := f.addQuotaAgreement(t, quotaSpec{max: ptr(1)})
		day := "2027-07-10"
		first, err := f.bookAs(t, ag, "A1", day)
		if err != nil {
			t.Fatalf("first: %v", err)
		}
		if _, err := f.bookAs(t, ag, "A2", day); quotaReason(err) != availability.ReasonAgreementQuota {
			t.Fatalf("second: %v, want agreement_quota", err)
		}
		if n := f.count(t, "agreement_quota_allocations", "agreement_id = ?", ag); n != 2 {
			t.Fatalf("detail rows = %d, want 2 (AM, PM)", n)
		}
		// Cancelling the first booking gives the quota back.
		must(t, store.RunInTx(context.Background(), f.db, func(ctx context.Context, tx bun.Tx) error {
			_, err := f.svc.Release(ctx, tx, f.es, inventory.ReleaseRequest{BookingID: first.id})
			return err
		}))
		if used := f.quotaUsed(t, ag, day); used != 0 {
			t.Fatalf("used after release = %d", used)
		}
		if _, err := f.bookAs(t, ag, "A2", day); err != nil {
			t.Fatalf("after release: %v", err)
		}
		if _, err := f.bookAs(t, "", "A3", day); err != nil {
			t.Fatalf("outside the agreement, no reserve: %v", err)
		}
	})
}

// TestConcurrency_AgreementReservedQuota: 120 parallel sales inside and
// outside an agreement with 2 units reserved and a cap of 3 on a zone of 5
// spots: never more than 3 sold outside before the release, never more
// than 3 in the agreement, the counter equals its detail rows.
func TestConcurrency_AgreementReservedQuota(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		ag := f.addQuotaAgreement(t, quotaSpec{max: ptr(3), reserved: ptr(2), zones: []string{f.seat.ID}})
		day := "2027-07-21"
		ok, refused := race(t, func(i int) error {
			agreement := ""
			if i%2 == 0 {
				agreement = ag
			}
			_, err := f.bookAs(t, agreement, fmt.Sprintf("A%d", i%5+1), day)
			return err
		}, isAvailabilityConflict)
		if ok+refused != attempts {
			t.Fatalf("ok=%d refused=%d", ok, refused)
		}
		var inside, outside int
		must(t, f.db.NewSelect().TableExpr("bookings AS bk").ColumnExpr("COUNT(*)").
			Where("EXISTS (SELECT 1 FROM agreement_quota_allocations AS q WHERE q.booking_id = bk.id)").Scan(context.Background(), &inside))
		must(t, f.db.NewSelect().TableExpr("bookings AS bk").ColumnExpr("COUNT(*)").
			Where("NOT EXISTS (SELECT 1 FROM agreement_quota_allocations AS q WHERE q.booking_id = bk.id)").Scan(context.Background(), &outside))
		if outside > 3 {
			t.Fatalf("INV-103: %d sold outside the agreement, at most 3 (5 spots - 2 reserved)", outside)
		}
		if inside > 3 {
			t.Fatalf("INV-103: %d sold in the agreement, cap 3", inside)
		}
		if int64(inside+outside) != ok {
			t.Fatalf("bookings %d+%d != successes %d", inside, outside, ok)
		}
		if used := f.quotaUsed(t, ag, day); used != inside {
			t.Fatalf("counter %d != agreement bookings %d", used, inside)
		}
		if n := f.count(t, "agreement_quota_allocations", "agreement_id = ?", ag); n != 2*inside {
			t.Fatalf("detail rows %d != 2 x %d", n, inside)
		}
		f.checkInvariants(t)
	})
}

// TestAgreementQuota_CapEditedAfterFirstSale (INV-103): the cap compared is
// the one of the agreement as configured now, not the one copied into the
// counter row by the first sale of the half-day: a cap lowered to the units
// already sold refuses the next sale, a cap added later is enforced, a cap
// raised lets the sale through.
func TestAgreementQuota_CapEditedAfterFirstSale(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		ag := f.addQuotaAgreement(t, quotaSpec{max: ptr(3)})
		day := "2027-07-12"
		setCap := func(cfg string) {
			t.Helper()
			_, err := db.NewUpdate().Table("agreements").Set("quota = ?", cfg).Where("id = ?", ag).Exec(ctx)
			must(t, err)
		}
		if _, err := f.bookAs(t, ag, "A1", day); err != nil {
			t.Fatalf("first: %v", err)
		}
		// Lowered to 1 after the first sale: the second is over the cap.
		setCap(`{"max_per_day":1}`)
		if _, err := f.bookAs(t, ag, "A2", day); quotaReason(err) != availability.ReasonAgreementQuota {
			t.Fatalf("after lowering the cap: %v, want agreement_quota", err)
		}
		// Raised to 2: one more goes through, the next is refused.
		setCap(`{"max_per_day":2}`)
		if _, err := f.bookAs(t, ag, "A2", day); err != nil {
			t.Fatalf("after raising the cap: %v", err)
		}
		if _, err := f.bookAs(t, ag, "A3", day); quotaReason(err) != availability.ReasonAgreementQuota {
			t.Fatalf("third with cap 2: %v", err)
		}
		// A cap added on a half-day first sold without one.
		ag2 := f.addQuotaAgreement(t, quotaSpec{reserved: ptr(0)})
		day2 := "2027-07-13"
		if _, err := f.bookAs(t, ag2, "A1", day2); err != nil {
			t.Fatalf("no cap: %v", err)
		}
		_, err := db.NewUpdate().Table("agreements").Set("quota = ?", `{"max_per_day":1}`).Where("id = ?", ag2).Exec(ctx)
		must(t, err)
		if _, err := f.bookAs(t, ag2, "A2", day2); quotaReason(err) != availability.ReasonAgreementQuota {
			t.Fatalf("cap added after the first sale: %v, want agreement_quota", err)
		}
		if used := f.quotaUsed(t, ag2, day2); used != 1 {
			t.Fatalf("used = %d", used)
		}
	})
}
