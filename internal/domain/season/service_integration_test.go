//go:build integration

package season_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type fixture struct {
	db      *store.DB
	svc     *season.Service
	estA    *store.Establishment
	estB    *store.Establishment
	owner   *auth.Principal // owner of org A
	manager *auth.Principal
	cashier *auth.Principal
	viewer  *auth.Principal
	ownerB  *auth.Principal
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC))
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	user := func(id, org string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: id, Grants: []auth.Grant{{OrganizationID: org, Role: role}}}
	}
	return &fixture{
		db:   db,
		svc:  season.NewService(season.Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}),
		estA: estA, estB: estB,
		owner: user("u-owner", orgA.ID, auth.RoleOwner), manager: user("u-manager", orgA.ID, auth.RoleManager),
		cashier: user("u-cashier", orgA.ID, auth.RoleCashier), viewer: user("u-viewer", orgA.ID, auth.RoleViewer),
		ownerB: user("u-owner-b", orgB.ID, auth.RoleOwner),
	}
}

func d(s string) calendar.Date { return calendar.MustParse(s) }

func rg(from, to string) store.DateRange { return store.DateRange{From: d(from), To: d(to)} }

func count(t *testing.T, db *store.DB, table, where string, args ...any) int64 {
	t.Helper()
	n, err := db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestSeasonService_SeasonLifecycle(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		in := season.SeasonInput{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30"), IP: "203.0.113.7"}
		sn, err := f.svc.CreateSeason(ctx, f.manager, f.estA.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if sn.Status != "draft" || sn.Version != 1 || sn.EstablishmentID != f.estA.ID {
			t.Fatalf("created %+v", sn)
		}
		if _, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, in); !errors.Is(err, season.ErrSeasonYearTaken) {
			t.Fatalf("duplicate year: %v", err)
		}
		// The same year in another establishment is fine.
		if _, err := f.svc.CreateSeason(ctx, f.ownerB, f.estB.ID, in); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "audit_log", "action = ? AND entity_id = ?", season.ActionSeasonCreated, sn.ID); n != 1 {
			t.Fatalf("audit entries %d", n)
		}
		if n := count(t, db, "outbox_events", "type = ? AND establishment_id = ?", event.ConfigPricingChanged, f.estA.ID); n != 1 {
			t.Fatalf("events %d", n)
		}

		// Activate with If-Match.
		up := in
		up.Status, up.IfMatch = season.StatusActive, ptr(int64(1))
		sn, err = f.svc.UpdateSeason(ctx, f.owner, f.estA.ID, sn.ID, up)
		if err != nil || sn.Status != "active" || sn.Version != 2 {
			t.Fatalf("activate: %+v %v", sn, err)
		}
		// Stale If-Match.
		_, err = f.svc.UpdateSeason(ctx, f.owner, f.estA.ID, sn.ID, up)
		var vc *configerr.VersionConflictError
		if !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale: %v", err)
		}
		// Invalid transition active -> closed -> draft.
		up.Status, up.IfMatch = season.StatusClosed, nil
		if sn, err = f.svc.UpdateSeason(ctx, f.owner, f.estA.ID, sn.ID, up); err != nil {
			t.Fatal(err)
		}
		up.Status = season.StatusDraft
		if _, err := f.svc.UpdateSeason(ctx, f.owner, f.estA.ID, sn.ID, up); !errors.Is(err, season.ErrInvalidStatus) {
			t.Fatalf("closed -> draft: %v", err)
		}
		// Closed seasons cannot be deleted.
		if err := f.svc.DeleteSeason(ctx, f.owner, f.estA.ID, sn.ID, nil, ""); !errors.Is(err, season.ErrSeasonNotDraft) {
			t.Fatalf("delete closed: %v", err)
		}
		// Validation.
		bad := season.SeasonInput{Year: 2028, OpensOn: d("2028-09-30"), ClosesOn: d("2028-05-01")}
		if _, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, bad); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("closes before opens: %v", err)
		}
		// Draft season deleted.
		draft, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, season.SeasonInput{Year: 2028, OpensOn: d("2028-05-01"), ClosesOn: d("2028-09-30")})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeleteSeason(ctx, f.owner, f.estA.ID, draft.ID, ptr(int64(9)), ""); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("delete stale: %v", err)
		}
		if err := f.svc.DeleteSeason(ctx, f.owner, f.estA.ID, draft.ID, ptr(int64(1)), ""); err != nil {
			t.Fatal(err)
		}
		page, err := f.svc.ListSeasons(ctx, f.viewer, f.estA.ID, "", 10)
		if err != nil || len(page.Items) != 1 || page.Items[0].Year != 2027 {
			t.Fatalf("list %+v %v", page, err)
		}
	})
}

func TestSeasonService_PermissionsAndTenancy(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		in := season.SeasonInput{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")}
		for _, p := range []*auth.Principal{f.cashier, f.viewer} {
			if _, err := f.svc.CreateSeason(ctx, p, f.estA.ID, in); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s: %v", p.UserID, err)
			}
		}
		sn, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		// Every role can read.
		for _, p := range []*auth.Principal{f.cashier, f.viewer, f.manager} {
			if _, err := f.svc.GetSeason(ctx, p, f.estA.ID, sn.ID); err != nil {
				t.Fatalf("%s read: %v", p.UserID, err)
			}
		}
		// Another tenant sees nothing: the establishment is not found, and a
		// season id is never resolved through a foreign establishment.
		if _, err := f.svc.GetSeason(ctx, f.ownerB, f.estA.ID, sn.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant get: %v", err)
		}
		if _, err := f.svc.GetSeason(ctx, f.ownerB, f.estB.ID, sn.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant id: %v", err)
		}
		if err := f.svc.DeleteSeason(ctx, f.ownerB, f.estB.ID, sn.ID, nil, ""); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant delete: %v", err)
		}
		if _, err := f.svc.CreateRatePeriod(ctx, f.ownerB, f.estB.ID, season.RatePeriodInput{
			SeasonID: sn.ID, Code: "low", Name: map[string]string{"it": "Bassa"}, Ranges: []store.DateRange{rg("2027-06-01", "2027-06-10")},
		}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("rate period on a foreign season: %v", err)
		}
	})
}

func TestSeasonService_RatePeriods(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		sn, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, season.SeasonInput{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")})
		if err != nil {
			t.Fatal(err)
		}
		mk := func(code string, ranges ...store.DateRange) season.RatePeriodInput {
			return season.RatePeriodInput{SeasonID: sn.ID, Code: code, Name: map[string]string{"it": code}, Ranges: ranges}
		}
		low, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, mk("low", rg("2027-05-15", "2027-06-30"), rg("2027-09-01", "2027-09-30")))
		if err != nil {
			t.Fatal(err)
		}
		high, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, mk("high", rg("2027-07-01", "2027-08-31")))
		if err != nil {
			t.Fatal(err)
		}
		if low.SortOrder != 0 || high.SortOrder != 1 {
			t.Fatalf("sort orders %d %d", low.SortOrder, high.SortOrder)
		}
		if _, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, mk("peak", rg("2027-08-10", "2027-08-20"))); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("overlap: %v", err)
		}
		if _, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, mk("high", rg("2027-05-15", "2027-05-15"))); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("overlap with low: %v", err)
		}
		// Shrink high, then peak fits.
		upd := mk("high", rg("2027-07-01", "2027-08-09"), rg("2027-08-21", "2027-08-31"))
		upd.IfMatch = ptr(int64(1))
		if high, err = f.svc.UpdateRatePeriod(ctx, f.manager, f.estA.ID, high.ID, upd); err != nil || high.Version != 2 {
			t.Fatalf("update: %v", err)
		}
		peak, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, mk("peak", rg("2027-08-10", "2027-08-20")))
		if err != nil {
			t.Fatal(err)
		}
		// Duplicate code on update.
		dup := mk("low", rg("2027-08-10", "2027-08-20"))
		if _, err := f.svc.UpdateRatePeriod(ctx, f.manager, f.estA.ID, peak.ID, dup); !errors.Is(err, season.ErrCodeTaken) {
			t.Fatalf("duplicate code: %v", err)
		}
		// Season dates cannot cut periods.
		if _, err := f.svc.UpdateSeason(ctx, f.owner, f.estA.ID, sn.ID, season.SeasonInput{Year: 2027, OpensOn: d("2027-06-01"), ClosesOn: d("2027-09-30")}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("season shrink: %v", err)
		}
		page, err := f.svc.ListRatePeriods(ctx, f.viewer, f.estA.ID, sn.ID, "", 2)
		if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
			t.Fatalf("page 1: %+v %v", page, err)
		}
		page2, err := f.svc.ListRatePeriods(ctx, f.viewer, f.estA.ID, sn.ID, page.NextCursor, 2)
		if err != nil || len(page2.Items) != 1 || page2.Items[0].Code != "peak" || page2.NextCursor != "" {
			t.Fatalf("page 2: %+v %v", page2, err)
		}
		if _, err := f.svc.ListRatePeriods(ctx, f.viewer, f.estA.ID, "", "garbage", 2); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}
		if err := f.svc.DeleteRatePeriod(ctx, f.manager, f.estA.ID, peak.ID, ptr(int64(1)), ""); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "outbox_events", "type = ? AND establishment_id = ?", event.ConfigPricingChanged, f.estA.ID); n != 6 {
			t.Fatalf("events %d (season + 3 creates + update + delete)", n)
		}
		if n := count(t, db, "audit_log", "action LIKE 'rate_period.%' AND establishment_id = ?", f.estA.ID); n != 5 {
			t.Fatalf("audit %d", n)
		}
	})
}

func TestSeasonService_ClosureDays(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		if _, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, season.SeasonInput{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")}); err != nil {
			t.Fatal(err)
		}
		in := season.ClosureDayInput{Date: d("2027-07-10"), Slots: []calendar.Slot{calendar.PM}, Reason: " Maltempo "}
		c, err := f.svc.CreateClosureDay(ctx, f.manager, f.estA.ID, in)
		if err != nil || c.Reason != "Maltempo" || len(c.Slots) != 1 {
			t.Fatalf("create: %+v %v", c, err)
		}
		if _, err := f.svc.CreateClosureDay(ctx, f.manager, f.estA.ID, in); !errors.Is(err, season.ErrClosureDateTaken) {
			t.Fatalf("duplicate date: %v", err)
		}
		out := in
		out.Date = d("2027-10-10")
		if _, err := f.svc.CreateClosureDay(ctx, f.manager, f.estA.ID, out); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("outside season: %v", err)
		}
		noSlots := in
		noSlots.Date, noSlots.Slots = d("2027-07-11"), nil
		if _, err := f.svc.CreateClosureDay(ctx, f.manager, f.estA.ID, noSlots); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("no slots: %v", err)
		}
		up := in
		up.Slots, up.IfMatch = []calendar.Slot{calendar.PM, calendar.AM}, ptr(int64(1))
		if c, err = f.svc.UpdateClosureDay(ctx, f.manager, f.estA.ID, c.ID, up); err != nil || len(c.Slots) != 2 || c.Version != 2 {
			t.Fatalf("update: %+v %v", c, err)
		}
		if _, err := f.svc.CreateClosureDay(ctx, f.manager, f.estA.ID, season.ClosureDayInput{Date: d("2027-08-01"), Slots: []calendar.Slot{calendar.AM}}); err != nil {
			t.Fatal(err)
		}
		page, err := f.svc.ListClosureDays(ctx, f.cashier, f.estA.ID, d("2027-07-15"), calendar.Date{}, "", 10)
		if err != nil || len(page.Items) != 1 || !page.Items[0].Date.Equal(d("2027-08-01")) {
			t.Fatalf("filtered list: %+v %v", page, err)
		}
		if _, err := f.svc.ListClosureDays(ctx, f.ownerB, f.estA.ID, calendar.Date{}, calendar.Date{}, "", 10); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant list: %v", err)
		}
		if err := f.svc.DeleteClosureDay(ctx, f.viewer, f.estA.ID, c.ID, nil, ""); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer delete: %v", err)
		}
		if err := f.svc.DeleteClosureDay(ctx, f.owner, f.estA.ID, c.ID, nil, ""); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSeasonService_CopySeason(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		src, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, season.SeasonInput{Year: 2026, OpensOn: d("2026-05-16"), ClosesOn: d("2026-09-27")})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []season.RatePeriodInput{
			{SeasonID: src.ID, Code: "low", Name: map[string]string{"it": "Bassa"}, Ranges: []store.DateRange{rg("2026-05-16", "2026-06-30"), rg("2026-09-01", "2026-09-27")}},
			{SeasonID: src.ID, Code: "high", Name: map[string]string{"it": "Alta"}, Ranges: []store.DateRange{rg("2026-07-01", "2026-08-31")}},
		} {
			if _, err := f.svc.CreateRatePeriod(ctx, f.owner, f.estA.ID, p); err != nil {
				t.Fatal(err)
			}
		}
		res, err := f.svc.CopySeason(ctx, f.manager, f.estA.ID, src.ID, season.CopyInput{TargetYear: 2027, IncludeRatePeriods: true})
		if err != nil {
			t.Fatal(err)
		}
		if res.Season.Year != 2027 || res.Season.Status != "draft" || len(res.RatePeriods) != 2 {
			t.Fatalf("copy %+v", res)
		}
		// 2026-05-16 is a Saturday: the copy opens on a Saturday.
		if res.Season.OpensOn.ISOWeekday() != 6 {
			t.Fatalf("opens on %s", res.Season.OpensOn)
		}
		stored, err := f.svc.ListRatePeriods(ctx, f.viewer, f.estA.ID, res.Season.ID, "", 10)
		if err != nil || len(stored.Items) != 2 {
			t.Fatalf("stored copies %+v %v", stored, err)
		}
		if _, err := f.svc.CopySeason(ctx, f.manager, f.estA.ID, src.ID, season.CopyInput{TargetYear: 2027}); !errors.Is(err, season.ErrTargetYearTaken) {
			t.Fatalf("second copy: %v", err)
		}
		if _, err := f.svc.CopySeason(ctx, f.viewer, f.estA.ID, src.ID, season.CopyInput{TargetYear: 2028}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("viewer copy: %v", err)
		}
		// Composition inside a caller's transaction: a failure afterwards
		// rolls the copy back.
		boom := errors.New("price list copy failed")
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			if _, err := f.svc.WithDB(tx).CopySeason(ctx, f.manager, f.estA.ID, src.ID, season.CopyInput{TargetYear: 2028, IncludeRatePeriods: true}); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatal(err)
		}
		if n := count(t, db, "seasons", "establishment_id = ? AND year = 2028", f.estA.ID); n != 0 {
			t.Fatal("copy not rolled back")
		}
	})
}

// TestConcurrency_RatePeriodsNeverOverlap: 100 concurrent creations of
// overlapping periods in the same season; exactly one wins.
func TestConcurrency_RatePeriodsNeverOverlap(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		sn, err := f.svc.CreateSeason(ctx, f.owner, f.estA.ID, season.SeasonInput{Year: 2027, OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-30")})
		if err != nil {
			t.Fatal(err)
		}
		const n = 100
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, other := 0, []error{}
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := f.svc.CreateRatePeriod(ctx, f.manager, f.estA.ID, season.RatePeriodInput{
					SeasonID: sn.ID, Code: "p" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Name: map[string]string{"it": "P"},
					Ranges: []store.DateRange{rg("2027-07-01", "2027-07-31")},
				})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, configerr.ErrValidation):
				default:
					other = append(other, err)
				}
			}()
		}
		wg.Wait()
		if ok != 1 || len(other) > 0 {
			t.Fatalf("winners %d, unexpected errors %v", ok, other)
		}
	})
}
