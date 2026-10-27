//go:build integration

package checkout_test

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestCheckoutFlow_SpotNotOnlineBookable (spec 30.11): a spot excluded from
// online sale is absent from the public availability and the public
// checkout cannot sell it (409, like a taken spot), while the other spots
// sell online and the desk still sells it.
func TestCheckoutFlow_SpotNotOnlineBookable(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		hidden, sellable := e.F.Spots[0], e.F.Spots[1]
		if _, err := db.NewUpdate().Table("spots").Set("online_excluded = ?", true).Where("id = ?", hidden.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		day := calendar.MustParse("2027-07-10")
		av, err := e.Svc.Availability(ctx, checkouttest.Slug, checkout.AvailabilityQuery{From: day, To: day, Formula: "day"})
		if err != nil {
			t.Fatal(err)
		}
		seen := false
		for _, z := range av.Zones {
			for _, sp := range z.Spots {
				if sp.SpotID == hidden.ID {
					t.Fatalf("excluded spot in the public availability: %+v", sp)
				}
				seen = seen || sp.SpotID == sellable.ID
			}
		}
		if !seen {
			t.Fatal("sellable spot missing from the public availability")
		}
		if q, err := e.Svc.Quote(ctx, checkouttest.Slug, e.Seat(hidden, "2027-07-10")); err == nil && q.Available {
			t.Fatalf("quote of an excluded spot is available: %+v", q)
		}
		_, err = e.Svc.Create(ctx, checkouttest.Slug, checkout.CreateInput{Selection: e.Seat(hidden, "2027-07-10"), Locale: "it"})
		var ce *availability.ConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("online checkout of an excluded spot: %v", err)
		}
		if n := e.Count((*store.SpotAllocation)(nil), "spot_id = ?", hidden.ID); n != 0 {
			t.Fatalf("allocations left on the excluded spot: %d", n)
		}
		if s := e.Create(e.Seat(sellable, "2027-07-10")); s.Status != "held" {
			t.Fatalf("online checkout of a sellable spot: %+v", s)
		}
		if _, err := e.Book.Create(ctx, e.F.Owner, e.F.Est.ID, e.F.Input(hidden, "2027-07-10", "2027-07-10")); err != nil {
			t.Fatalf("desk sale of the excluded spot: %v", err)
		}
	})
}

// TestManage_SeasonPassReleaseRange (spec 30.8): the holder releases every
// weekend of the pass from the manage area with a range; the preview writes
// nothing and the same range sent again releases nothing new.
func TestManage_SeasonPassReleaseRange(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := checkouttest.New(t, db)
		ctx := e.Ctx
		in := e.F.Input(e.F.Spots[1], "2027-07-01", "2027-07-10")
		in.IsSeasonPass, in.AllocationKind = true, "season"
		d, err := e.Book.Create(ctx, e.F.Owner, e.F.Est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		link, err := e.Notify.IssueMagicLink(ctx, db, e.ES, d.Booking.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Saturdays and Sundays: 3, 4 and 10 July, both slots.
		weekends := &pass.ReleaseRange{From: calendar.MustParse("2027-07-01"), To: calendar.MustParse("2027-07-31"), Weekdays: []int{6, 7}}
		prev, err := e.Svc.ManageRelease(ctx, checkouttest.Slug, link.Token, checkout.ReleaseInput{Range: weekends, RangeSlots: calendar.SlotsFull, DryRun: true})
		if err != nil || prev.Applied || len(prev.Released) != 6 {
			t.Fatalf("preview %v %+v", err, prev)
		}
		if n := e.Count((*store.SeasonRelease)(nil), ""); n != 0 {
			t.Fatalf("preview wrote %d releases", n)
		}
		out, err := e.Svc.ManageRelease(ctx, checkouttest.Slug, link.Token, checkout.ReleaseInput{Range: weekends})
		if err != nil || !out.Applied || len(out.Released) != 6 || out.Managed == nil || len(out.Managed.Releases) != 6 {
			t.Fatalf("release %v %+v", err, out)
		}
		again, err := e.Svc.ManageRelease(ctx, checkouttest.Slug, link.Token, checkout.ReleaseInput{Range: weekends})
		if err != nil || !again.Applied || len(again.Released) != 0 {
			t.Fatalf("replay %v %+v", err, again)
		}
		if n := e.Count((*store.SeasonRelease)(nil), "revoked_at IS NULL"); n != 6 {
			t.Fatalf("active releases %d", n)
		}
		if _, err := e.Svc.ManageRelease(ctx, checkouttest.Slug, link.Token, checkout.ReleaseInput{}); err == nil {
			t.Fatal("empty request accepted")
		}
	})
}
