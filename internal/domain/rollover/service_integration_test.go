//go:build integration

package rollover_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var d = calendar.MustParse

// world is the bookingtest lido (2027 season, three price rules) with a
// rate period, two closure days inside the season, one outside and a
// booking policy, plus the rollover service on the real price list copier.
func world(t *testing.T, db *store.DB) (*bookingtest.Fixture, *rollover.Service) {
	t.Helper()
	f := bookingtest.New(t, db)
	now := f.Clock.Now()
	f.Insert(&store.RatePeriod{
		ID: store.NewID(), EstablishmentID: f.Est.ID, SeasonID: f.Season.ID, Code: "high", Name: store.I18nText{"it": "Alta"},
		Color: "#ff0000", Ranges: []store.DateRange{{From: d("2027-07-03"), To: d("2027-08-29")}}, Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	for _, c := range []struct{ date, reason string }{{"2027-07-17", "festa del santo"}, {"2027-08-15", "ferragosto"}, {"2027-12-25", "natale"}} {
		f.Insert(&store.ClosureDay{
			ID: store.NewID(), EstablishmentID: f.Est.ID, Date: d(c.date), Slots: []calendar.Slot{calendar.AM, calendar.PM},
			Reason: c.reason, Version: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	maxSpots := 4
	ps := pricingconfig.NewPolicyService(pricingconfig.Deps{DB: db, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock)})
	if _, err := ps.Create(f.Ctx, f.Owner, f.Est.ID, pricingconfig.PolicyInput{
		SeasonID: f.Season.ID, Name: "Standard", Active: true,
		Rules: policy.Rules{MaxSpotsPerBooking: &maxSpots},
	}); err != nil {
		t.Fatalf("policy: %v", err)
	}
	return f, rollover.NewService(rollover.Deps{DB: db, Clock: f.Clock, Copier: f.Pricing})
}

func all(year int, dry bool) rollover.Input {
	return rollover.Input{
		TargetYear: year, IncludeRatePeriods: true, IncludeClosureDays: true, IncludePriceRules: true,
		IncludeFixedPrices: true, IncludePolicies: true, DryRun: dry,
	}
}

// TestRollover_PreviewApplyIdempotent: the preview reports exactly what
// the apply copies without writing; the apply creates the draft season of
// the next year with rate periods, price rules, policies and the closure
// days shifted (weekday kept, fixed holidays on their date), audited with
// season.rolled_over; a second apply returns the recorded result.
func TestRollover_PreviewApplyIdempotent(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f, svc := world(t, db)
		ctx := f.Ctx
		want := rollover.Counts{RatePeriods: 1, ClosureDays: 2, PriceRules: 3, FixedPrices: 0, Policies: 1}

		prev, created, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2028, true))
		if err != nil || created || !prev.DryRun || prev.AlreadyApplied || prev.Counts != want || prev.Season != nil || prev.RolloverID != "" {
			t.Fatalf("preview: %v %v %+v", err, created, prev)
		}
		if prev.OpensOn.String() != "2028-05-13" || prev.ClosesOn.String() != "2028-09-13" {
			t.Fatalf("preview dates %s..%s", prev.OpensOn, prev.ClosesOn)
		}
		if _, err := store.GetSeasonByYear(ctx, db, mustScope(t, f), 2028); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("preview created the season: %v", err)
		}
		if n := f.Count((*store.AuditEntry)(nil), "action = 'season.rolled_over'"); n != 0 {
			t.Fatalf("preview audited %d", n)
		}

		res, created, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2028, false))
		if err != nil || !created || res.DryRun || res.AlreadyApplied || res.Counts != want || res.Season == nil || res.RolloverID == "" {
			t.Fatalf("apply: %v %v %+v", err, created, res)
		}
		if res.Season.Status != "draft" || res.Season.Year != 2028 || res.Season.OpensOn.String() != "2028-05-13" {
			t.Fatalf("season %+v", res.Season)
		}
		es := mustScope(t, f)
		closures, err := store.RolloverClosureDays(ctx, db, es, d("2028-01-01"), d("2028-12-31"))
		if err != nil || len(closures) != 2 || closures[0].Date.String() != "2028-07-15" || closures[1].Date.String() != "2028-08-15" ||
			closures[0].Reason != "festa del santo" {
			t.Fatalf("closures: %v %+v", err, closures)
		}
		periods, err := store.PricingListRatePeriods(ctx, db, es, res.Season.ID)
		if err != nil || len(periods) != 1 || periods[0].Ranges[0].From.String() != "2028-07-01" {
			t.Fatalf("rate periods: %v %+v", err, periods)
		}
		if n, err := store.CountSeasonPriceRules(ctx, db, es, res.Season.ID); err != nil || n != 3 {
			t.Fatalf("price rules: %v %d", err, n)
		}

		again, created, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2028, false))
		if err != nil || created || !again.AlreadyApplied || again.RolloverID != res.RolloverID || again.Counts != want || again.Season.ID != res.Season.ID {
			t.Fatalf("second apply: %v %v %+v", err, created, again)
		}
		if again, _, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2028, true)); err != nil || !again.AlreadyApplied {
			t.Fatalf("preview after apply: %v %+v", err, again)
		}
		for what, n := range map[string]int{
			"seasons 2028": f.Count((*store.Season)(nil), "year = 2028"),
			"rollovers":    f.Count((*store.SeasonRollover)(nil), ""),
			"audit":        f.Count((*store.AuditEntry)(nil), "action = 'season.rolled_over'"),
			"outbox":       f.Count((*store.OutboxEvent)(nil), "type = ?", event.SeasonRolledOver),
		} {
			if n != 1 {
				t.Errorf("%s = %d, want 1", what, n)
			}
		}
		page, err := svc.List(ctx, f.Owner, f.Est.ID, "", 10)
		if err != nil || len(page.Items) != 1 || rollover.CountsOf(&page.Items[0]) != want || page.Items[0].CreatedByUserID == nil {
			t.Fatalf("list: %v %+v", err, page)
		}
	})
}

// TestRollover_Refusals: an existing season of the target year (not from
// this rollover) is a conflict; permissions and tenancy are enforced;
// partial copies leave out what is not included.
func TestRollover_Refusals(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f, svc := world(t, db)
		ctx, now := f.Ctx, f.Clock.Now()
		f.Insert(&store.Season{ID: store.NewID(), EstablishmentID: f.Est.ID, Year: 2029, OpensOn: d("2029-05-12"), ClosesOn: d("2029-09-15"), Status: "draft", Version: 1, CreatedAt: now, UpdatedAt: now})
		for _, dry := range []bool{true, false} {
			if _, _, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2029, dry)); !errors.Is(err, rollover.ErrTargetExists) {
				t.Fatalf("existing target (dry %v): %v", dry, err)
			}
		}
		if _, _, err := svc.Rollover(ctx, f.Cashier, f.Est.ID, f.Season.ID, all(2028, false)); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		if _, _, err := svc.Rollover(ctx, f.Outsider, f.Est.ID, f.Season.ID, all(2028, false)); err == nil {
			t.Fatal("outsider rolled over")
		}
		if _, _, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, all(2027, true)); err == nil {
			t.Fatal("same year accepted")
		}
		in := all(2028, false)
		in.IncludeClosureDays, in.IncludePolicies = false, false
		res, _, err := svc.Rollover(ctx, f.Owner, f.Est.ID, f.Season.ID, in)
		if err != nil || res.Counts.ClosureDays != 0 || res.Counts.Policies != 0 || res.Counts.PriceRules != 3 {
			t.Fatalf("partial copy: %v %+v", err, res)
		}
	})
}

// TestConcurrency_RolloverApply: 100 parallel applies of the same rollover
// create exactly one season and one rollover row; every other call answers
// with the recorded result (lock on the source season, unique index).
func TestConcurrency_RolloverApply(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		f, svc := world(t, db)
		const parallel = 100
		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			created, replay int
			ids             = map[string]bool{}
			errs            []error
		)
		start := make(chan struct{})
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				res, c, err := svc.Rollover(f.Ctx, f.Owner, f.Est.ID, f.Season.ID, all(2028, false))
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err != nil:
					errs = append(errs, err)
				case c:
					created++
					ids[res.RolloverID] = true
				case res.AlreadyApplied:
					replay++
					ids[res.RolloverID] = true
				}
			}()
		}
		close(start)
		wg.Wait()
		if len(errs) > 0 {
			t.Fatalf("errors (%d), first: %v", len(errs), errs[0])
		}
		if created != 1 || replay != parallel-1 || len(ids) != 1 {
			t.Fatalf("created %d, replayed %d, ids %d", created, replay, len(ids))
		}
		if n := f.Count((*store.Season)(nil), "year = 2028"); n != 1 {
			t.Fatalf("seasons 2028 = %d", n)
		}
		if n := f.Count((*store.ClosureDay)(nil), "date >= '2028-01-01'"); n != 2 {
			t.Fatalf("closures copied %d times", n)
		}
	})
}

func mustScope(t *testing.T, f *bookingtest.Fixture) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.Ctx, f.DB, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}
