//go:build integration

package pass_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// noPayments is the payments port of a flow where nothing is collected.
type noPayments struct{}

func (noPayments) RecordRefund(context.Context, bun.IDB, *auth.Principal, store.EstablishmentScope, payment.RefundRequest) (*store.Payment, error) {
	return nil, errors.New("no refunds in this test")
}

func (noPayments) ListByBooking(context.Context, bun.IDB, store.EstablishmentScope, string) ([]store.Payment, error) {
	return nil, nil
}

// TestPassFlow_RealServices runs a season pass end to end on the real
// booking, inventory, pricing and customer services wired as in
// internal/app/m3.go: creation priced by the fixed season price with season
// allocations, release, resale of the released day by a normal booking
// (MarkResold called by the booking service), cancellation of the resale
// (UnmarkResold), revocation (allocation restored); INV-10 holds at every step.
func TestPassFlow_RealServices(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
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
		if dt.Pass.Booking.TotalAmount != 90000 || !dt.Pass.Booking.IsSeasonPass || dt.Pass.SpotLabel != "A1" {
			t.Fatalf("pass %+v", dt.Pass)
		}
		if n := e.count(t, "spot_allocations", "booking_id = ? AND kind = ?", pid, store.AllocationSeason); n != 60 {
			t.Fatalf("season allocations %d", n)
		}

		rel, err := passes.Release(ctx, e.staff, "", e.estA.ID, pid, pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-10")}})
		if err != nil {
			t.Fatalf("release: %v", err)
		}
		e.assertINV10(t)

		// The released day is sold to another customer from the panel.
		buyer, err := e.customers.Create(ctx, e.cashier, "", e.estA.ID, customer.Input{LastName: "Verdi"}, false)
		if err != nil {
			t.Fatal(err)
		}
		sold, err := bookings.Create(ctx, e.cashier, e.estA.ID, booking.CreateInput{
			CustomerID: buyer.ID, StartsOn: d("2027-06-10"), EndsOn: d("2027-06-10"), Slots: calendar.SlotsFull, Formula: "day",
			Lines: []booking.LineInput{{PackID: e.pack.ID, ZoneID: e.zone.ID, SpotID: e.spot.ID, Quantity: 1}},
		})
		if err != nil {
			t.Fatalf("resale booking: %v", err)
		}
		rl, err := passes.ListReleases(ctx, e.viewer, e.estA.ID, pid)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rl.Releases {
			if r.ResoldBookingLineID == nil || rl.ResoldBookingIDs[*r.ResoldBookingLineID] != sold.Booking.ID {
				t.Fatalf("release not marked resold: %+v", r)
			}
		}
		e.assertINV10(t)
		if err := passes.Revoke(ctx, e.staff, "", e.estA.ID, pid, rel[0].ID); !errors.Is(err, pass.ErrReleaseResold) {
			t.Fatalf("revoke resold: %v", err)
		}
		// A day not released is still taken by the pass.
		if _, err := bookings.Create(ctx, e.cashier, e.estA.ID, booking.CreateInput{
			CustomerID: buyer.ID, StartsOn: d("2027-06-11"), EndsOn: d("2027-06-11"), Slots: calendar.SlotsFull, Formula: "day",
			Lines: []booking.LineInput{{PackID: e.pack.ID, ZoneID: e.zone.ID, SpotID: e.spot.ID, Quantity: 1}},
		}); err == nil {
			t.Fatal("a day of the pass not released must not be bookable")
		}

		// The resale is cancelled: the releases become resellable again and
		// can be revoked, which gives the day back to the holder.
		if _, err := bookings.Cancel(ctx, e.cashier, e.estA.ID, sold.Booking.ID, booking.CancelInput{Reason: "cambio programma"}); err != nil {
			t.Fatalf("cancel resale: %v", err)
		}
		e.clock.Advance(time.Minute)
		for _, r := range rel {
			if err := passes.Revoke(ctx, e.staff, "", e.estA.ID, pid, r.ID); err != nil {
				t.Fatalf("revoke %s %s: %v", r.Date, r.Slot, err)
			}
		}
		if n := e.count(t, "spot_allocations", "booking_id = ? AND kind = ?", pid, store.AllocationSeason); n != 60 {
			t.Fatalf("season allocations after revoke %d", n)
		}
		e.assertINV10(t)
	})
}

// TestConcurrency_RevokeVsCancel (INV-10, INV-20): on 100 passes, each with a
// released day, the revocation of the release races the cancellation of the
// pass through the real booking service. Whatever wins, a cancelled pass has
// no allocation left and an active one has its day back: Revoke locks the
// pass booking (the same lock Cancel takes) before reading its status.
func TestConcurrency_RevokeVsCancel(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx := e.ctx
		e.estA.Settings = store.JSON(`{"pricing":{"half_day_factor":"0.5","rounding":"none","default_vat_rate_bp":2200}}`)
		if _, err := db.NewUpdate().Model(e.estA).Column("settings").WherePK().Exec(ctx); err != nil {
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
		inv := inventory.NewService(inventory.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob})
		resale := pass.NewResale(pass.ResaleDeps{Clock: c, Credits: e.customers})
		bookings := booking.NewService(booking.Deps{
			DB: db, Clock: c, Audit: aw, Outbox: ob, Inventory: inv, Pricer: prices, Customers: e.customers,
			Payments: noPayments{}, Releases: resale,
		})
		passes := pass.NewService(pass.Deps{DB: db, Clock: c, Audit: aw, Outbox: ob, Bookings: bookings, Inventory: inv, Credits: e.customers})

		const attempts = 100
		m := new(store.Map)
		if err := db.NewSelect().Model(m).Where("establishment_id = ?", e.estA.ID).Limit(1).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		passIDs := make([]string, attempts)
		releaseIDs := make([]string, attempts)
		for i := range attempts {
			sp := e.newSpot(t, e.estA.ID, m.ID, e.zone.ID, fmt.Sprintf("R%03d", i), true)
			dt, err := passes.Create(ctx, e.cashier, "", e.estA.ID, pass.CreateInput{CustomerID: e.holder.ID, SeasonID: e.season.ID, SpotID: sp.ID, PackID: e.pack.ID})
			if err != nil {
				t.Fatalf("create pass %d: %v", i, err)
			}
			passIDs[i] = dt.Pass.Booking.ID
			rel, err := passes.Release(ctx, e.staff, "", e.estA.ID, passIDs[i], pass.ReleaseInput{Dates: []calendar.Date{d("2027-06-10")}, Slots: calendar.SlotsAM})
			if err != nil {
				t.Fatalf("release %d: %v", i, err)
			}
			releaseIDs[i] = rel[0].ID
		}
		var wg sync.WaitGroup
		var revoked, notActive, cancelled atomic.Int32
		start := make(chan struct{})
		for i := range attempts {
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				err := passes.Revoke(ctx, e.staff, "", e.estA.ID, passIDs[i], releaseIDs[i])
				switch {
				case err == nil:
					revoked.Add(1)
				case errors.Is(err, pass.ErrNotActive):
					notActive.Add(1)
				default:
					t.Errorf("revoke %d: %v", i, err)
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				if _, err := bookings.Cancel(ctx, e.cashier, e.estA.ID, passIDs[i], booking.CancelInput{Reason: "rinuncia"}); err != nil {
					t.Errorf("cancel %d: %v", i, err)
					return
				}
				cancelled.Add(1)
			}()
		}
		close(start)
		wg.Wait()
		t.Logf("revoked before the cancellation %d, refused after it %d, cancelled %d", revoked.Load(), notActive.Load(), cancelled.Load())
		if cancelled.Load() != attempts || revoked.Load()+notActive.Load() != attempts {
			t.Fatalf("outcomes: revoked %d not active %d cancelled %d", revoked.Load(), notActive.Load(), cancelled.Load())
		}
		if n := e.count(t, "spot_allocations AS sa", "EXISTS (SELECT 1 FROM bookings AS b WHERE b.id = sa.booking_id AND b.status = 'cancelled')"); n != 0 {
			t.Fatalf("INV-20: %d allocations of cancelled passes", n)
		}
		e.assertINV10(t)
	})
}
