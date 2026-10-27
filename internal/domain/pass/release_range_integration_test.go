//go:build integration

package pass_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestSeasonRelease_RangeWeekdays (spec 30.8): a range with recurring
// weekdays releases the matching days of the pass in one transaction (one
// event); sent again it releases nothing new (idempotent per day); explicit
// dates add to the range and keep their strict check; the preview of the
// manage area applies the same plan and writes nothing.
func TestSeasonRelease_RangeWeekdays(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		dt := e.createPass(t, pass.CreateInput{})
		pid, lineID := dt.Pass.Booking.ID, dt.Pass.LineID
		mondays := pass.ReleaseInput{Range: &pass.ReleaseRange{From: d("2027-06-01"), To: d("2027-06-30"), Weekdays: []int{1}}, Note: "corsi"}

		rel, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, mondays)
		if err != nil {
			t.Fatal(err)
		}
		if len(rel) != 8 || rel[0].Date.String() != "2027-06-07" || rel[7].Date.String() != "2027-06-28" || rel[0].Note != "corsi" {
			t.Fatalf("releases %+v", rel)
		}
		for _, r := range rel {
			if r.Date.ISOWeekday() != 1 {
				t.Fatalf("%s is not a Monday", r.Date)
			}
		}
		if n := e.count(t, "outbox_events", "type = ?", event.SeasonDayReleased); n != 1 {
			t.Fatalf("events %d", n)
		}
		// Same range again: nothing new, no error, no event.
		again, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, mondays)
		if err != nil || len(again) != 0 {
			t.Fatalf("idempotent release: %v %+v", err, again)
		}
		if n := e.count(t, "season_releases", "booking_id = ? AND revoked_at IS NULL", pid); n != 8 {
			t.Fatalf("active releases %d", n)
		}
		if n := e.count(t, "outbox_events", "type = ?", event.SeasonDayReleased); n != 1 {
			t.Fatalf("events after the replay %d", n)
		}
		// Range + dates, afternoon only: 06-14 PM is already released and
		// skipped by the range, 06-15/16 PM and the explicit 06-20 PM added.
		mixed, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{
			Dates: []calendar.Date{d("2027-06-20")}, Slots: calendar.SlotsPM,
			Range: &pass.ReleaseRange{From: d("2027-06-14"), To: d("2027-06-16")},
		})
		if err != nil || len(mixed) != 3 || mixed[0].Date.String() != "2027-06-15" || mixed[2].Date.String() != "2027-06-20" || mixed[0].Slot != calendar.PM {
			t.Fatalf("mixed release %v %+v", err, mixed)
		}
		// An explicit date already released still fails (strict).
		if _, err := e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{
			Dates: []calendar.Date{d("2027-06-07")}, Range: &pass.ReleaseRange{From: d("2027-06-22"), To: d("2027-06-22")},
		}); !errors.Is(err, pass.ErrAlreadyReleased) {
			t.Fatalf("explicit released date: %v", err)
		}
		_, err = e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Range: &pass.ReleaseRange{From: d("2027-06-01"), To: d("2027-06-30"), Weekdays: []int{9}}})
		mustField(t, err, "weekdays[0]")
		_, err = e.svc.Release(e.ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Range: &pass.ReleaseRange{From: d("2027-07-01"), To: d("2027-07-31")}})
		mustField(t, err, "from")
		if _, err := e.svc.Release(e.ctx, e.viewer, "", e.estA.ID, pid, mondays); err == nil {
			t.Fatal("viewer released days")
		}
		e.assertINV10(t)

		// Manage area (customer): allowed by the policy snapshot; the
		// preview writes nothing, the release applies the same plan.
		if _, err := db.NewUpdate().Table("bookings").Set("policy_snapshot = ?",
			`{"version":1,"policy_id":"p1","rules":{"customer_can_release_season_days":true,"season_release_credit":{"enabled":true,"percent":50}}}`).
			Where("id = ?", pid).Exec(e.ctx); err != nil {
			t.Fatal(err)
		}
		holder := audit.Actor{Type: audit.ActorCustomer, ID: e.holder.ID}
		tuesdays := pass.ReleaseInput{Range: &pass.ReleaseRange{From: d("2027-06-01"), To: d("2027-06-30"), Weekdays: []int{2}}, Slots: calendar.SlotsAM}
		before := e.count(t, "season_releases", "booking_id = ?", pid)
		preview, err := e.svc.PreviewReleaseAsCustomer(e.ctx, holder, e.esA, pid, tuesdays)
		if err != nil || len(preview) != 5 { // 1, 8, 15, 22, 29 June
			t.Fatalf("preview %v %d", err, len(preview))
		}
		if n := e.count(t, "season_releases", "booking_id = ?", pid); n != before {
			t.Fatalf("preview wrote %d releases", n-before)
		}
		if got, err := e.svc.ReleaseAsCustomer(e.ctx, holder, e.esA, pid, tuesdays); err != nil || len(got) != 5 {
			t.Fatalf("customer release %v %d", err, len(got))
		}
		if n := e.count(t, "spot_allocations", "booking_line_id = ? AND date = ? AND slot = 'AM'", lineID, d("2027-06-15")); n != 0 {
			t.Fatalf("season row left on a released half-day: %d", n)
		}
		e.assertINV10(t)
	})
}

// TestConcurrency_ReleaseRangeVsSale (spec 30.8, INV-01, INV-10): 50
// identical range releases of a pass race 50 desk sales of a day inside
// the range on the same spot, through the real booking and inventory
// services. The season allocation keeps the spot until the release
// commits: at most one sale wins, only after the release; the range is
// released exactly once; the sold day is marked resold and the holder's
// credit is confirmed per season_release_credit.
func TestConcurrency_ReleaseRangeVsSale(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx := e.ctx
		e.estA.Settings = store.JSON(`{"pricing":{"half_day_factor":"0.5","rounding":"none","default_vat_rate_bp":2200}}`)
		if _, err := db.NewUpdate().Model(e.estA).Column("settings").WherePK().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		e.pack.AllowedFormulas = []string{"day", "season"}
		if _, err := db.NewUpdate().Model(e.pack).Column("allowed_formulas").WherePK().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		c := e.clock
		aw, ob := audit.NewWriter(c), event.NewOutbox(c)
		prices := pricingconfig.NewService(pricingconfig.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		if _, err := prices.CreateFixedPrice(ctx, e.owner, e.estA.ID, pricingconfig.FixedPriceInput{
			SeasonID: e.season.ID, PackID: e.pack.ID, Formula: pricing.FormulaSeason, PriceCategory: "row1", Amount: 90000,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := prices.CreateRule(ctx, e.owner, e.estA.ID, pricingconfig.RuleInput{
			Name: "giornata", SeasonID: e.season.ID, TargetType: pricing.TargetPack, TargetIDs: []string{e.pack.ID},
			Action: pricing.Action{Type: pricing.ActionSet, Amount: 3000}, Stage: pricing.StageBase, Active: true,
		}); err != nil {
			t.Fatal(err)
		}
		inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		resale := pass.NewResale(pass.ResaleDeps{Clock: c, Credits: e.customers})
		bookings := booking.NewService(booking.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: prices, Customers: e.customers,
			Payments: noPayments{}, Releases: resale,
		})
		passes := pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: bookings, Inventory: inv, Credits: e.customers})
		dt, err := passes.Create(ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: e.spot.ID, PackID: e.pack.ID})
		if err != nil {
			t.Fatalf("create pass: %v", err)
		}
		pid := dt.Pass.Booking.ID
		// The holder's policy gives back 50% of the resale (spec 9).
		if _, err := db.NewUpdate().Table("bookings").Set("policy_snapshot = ?", creditPolicy).Where("id = ?", pid).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		buyer, err := e.customers.Create(ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "Verdi"}, false)
		if err != nil {
			t.Fatal(err)
		}
		sale := booking.CreateInput{
			CustomerID: buyer.ID, StartsOn: d("2027-06-15"), EndsOn: d("2027-06-15"), Slots: calendar.SlotsFull, Formula: "day",
			Lines: []booking.LineInput{{PackID: e.pack.ID, ZoneID: e.zone.ID, SpotID: e.spot.ID, Quantity: 1}},
		}
		// Mondays and Tuesdays of 14-20 June: 14 and 15 June, both slots.
		rng := pass.ReleaseInput{Range: &pass.ReleaseRange{From: d("2027-06-14"), To: d("2027-06-20"), Weekdays: []int{1, 2}}}

		const attempts = 100
		var wg sync.WaitGroup
		var released, sold, refused atomic.Int32
		start := make(chan struct{})
		for i := range attempts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if i%2 == 0 {
					rows, err := passes.Release(ctx, e.staff, "", e.estA.ID, pid, rng)
					if err != nil {
						t.Errorf("release %d: %v", i, err)
						return
					}
					released.Add(int32(len(rows)))
					return
				}
				_, err := bookings.Create(ctx, e.cashier, e.estA.ID, sale)
				var ce *availability.ConflictError
				switch {
				case err == nil:
					sold.Add(1)
				case errors.As(err, &ce):
					refused.Add(1)
				default:
					t.Errorf("sale %d: %v", i, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		t.Logf("half-days released %d, sales %d, refused %d", released.Load(), sold.Load(), refused.Load())
		if released.Load() != 4 || sold.Load() > 1 || sold.Load()+refused.Load() != attempts/2 {
			t.Fatalf("outcomes: released %d sold %d refused %d", released.Load(), sold.Load(), refused.Load())
		}
		if sold.Load() == 0 { // every sale ran before the release committed
			if _, err := bookings.Create(ctx, e.cashier, e.estA.ID, sale); err != nil {
				t.Fatalf("sale after the release: %v", err)
			}
		}
		if n := e.count(t, "season_releases", "booking_id = ? AND revoked_at IS NULL", pid); n != 4 {
			t.Fatalf("active releases %d", n)
		}
		if n := e.count(t, "season_releases", "booking_id = ? AND date = ? AND resold_booking_line_id IS NOT NULL", pid, d("2027-06-15")); n != 2 {
			t.Fatalf("resold half-days of 15 June %d", n)
		}
		if n := e.count(t, "spot_allocations", "spot_id = ? AND date = ?", e.spot.ID, d("2027-06-15")); n != 2 {
			t.Fatalf("allocations of 15 June %d", n)
		}
		// Credit confirmed: 50% of the resale share of each half-day.
		if bal := e.balance(t); bal != 1500 {
			t.Fatalf("holder credit %v, want 1500", bal)
		}
		e.assertINV10(t)
	})
}
