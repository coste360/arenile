//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func seasonTestScopes(t *testing.T, db *store.DB) (store.EstablishmentScope, store.EstablishmentScope) {
	t.Helper()
	ctx := context.Background()
	_, a, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "season-a", "A", t0)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "season-b", "B", t0)
	if err != nil {
		t.Fatal(err)
	}
	esA, err := store.SystemEstablishmentScope(ctx, db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	esB, err := store.SystemEstablishmentScope(ctx, db, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	return esA, esB
}

func TestSeasonStore_SeasonsVersionedAndScoped(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		esA, esB := seasonTestScopes(t, db)
		for _, y := range []int{2025, 2026, 2027} {
			s := &store.Season{ID: store.NewID(), Year: y, OpensOn: calendar.New(y, 5, 1), ClosesOn: calendar.New(y, 9, 30), Status: "draft", CreatedAt: t0, UpdatedAt: t0}
			if err := store.InsertSeason(ctx, db, esA, s); err != nil {
				t.Fatal(err)
			}
		}
		dup := &store.Season{ID: store.NewID(), Year: 2027, OpensOn: calendar.New(2027, 5, 1), ClosesOn: calendar.New(2027, 9, 30), Status: "draft", CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertSeason(ctx, db, esA, dup); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate year: %v", err)
		}
		page, err := store.ListSeasons(ctx, db, esA, "", 2)
		if err != nil || len(page.Items) != 2 || page.Items[0].Year != 2027 || page.NextCursor == "" {
			t.Fatalf("page 1 %+v %v", page, err)
		}
		page, err = store.ListSeasons(ctx, db, esA, page.NextCursor, 2)
		if err != nil || len(page.Items) != 1 || page.Items[0].Year != 2025 || page.NextCursor != "" {
			t.Fatalf("page 2 %+v %v", page, err)
		}
		if _, err := store.ListSeasons(ctx, db, esA, "bogus!", 2); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("cursor %v", err)
		}
		s, err := store.GetSeasonByYear(ctx, db, esA, 2026)
		if err != nil {
			t.Fatal(err)
		}
		// Never visible through another establishment.
		if _, err := store.GetSeason(ctx, db, esB, s.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross scope get %v", err)
		}
		if ok, err := store.DeleteSeasonIfVersion(ctx, db, esB, s.ID, 1); err != nil || ok {
			t.Fatalf("cross scope delete %v %v", ok, err)
		}
		if err := store.LockSeason(ctx, db, esB, s.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross scope lock %v", err)
		}
		// Versioned update.
		s.Status, s.Version = "active", 2
		if ok, err := store.UpdateSeasonIfVersion(ctx, db, esA, s, 1); err != nil || !ok {
			t.Fatalf("update %v %v", ok, err)
		}
		if ok, err := store.UpdateSeasonIfVersion(ctx, db, esA, s, 1); err != nil || ok {
			t.Fatalf("stale update %v %v", ok, err)
		}
		if err := store.LockSeason(ctx, db, esA, s.ID); err != nil {
			t.Fatal(err)
		}
		got, _ := store.GetSeason(ctx, db, esA, s.ID)
		if got.Version != 2 || got.Status != "active" {
			t.Fatalf("lock changed the row %+v", got)
		}
		cover, err := store.SeasonsCovering(ctx, db, esA, calendar.New(2026, 8, 15))
		if err != nil || len(cover) != 1 || cover[0].ID != s.ID {
			t.Fatalf("covering %+v %v", cover, err)
		}
		if cover, _ := store.SeasonsCovering(ctx, db, esA, calendar.New(2026, 10, 1)); len(cover) != 0 {
			t.Fatalf("covering outside %+v", cover)
		}
	})
}

func TestSeasonStore_RatePeriodsAndClosures(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		esA, esB := seasonTestScopes(t, db)
		sn := &store.Season{ID: store.NewID(), Year: 2027, OpensOn: calendar.New(2027, 5, 1), ClosesOn: calendar.New(2027, 9, 30), Status: "draft", CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertSeason(ctx, db, esA, sn); err != nil {
			t.Fatal(err)
		}
		var ids []string
		for i, code := range []string{"low", "mid", "high"} {
			p := &store.RatePeriod{
				ID: store.NewID(), SeasonID: sn.ID, Code: code, Name: store.I18nText{"it": code}, SortOrder: 2 - i,
				Ranges: []store.DateRange{{From: calendar.New(2027, 6, 1+i*10), To: calendar.New(2027, 6, 5+i*10)}}, CreatedAt: t0, UpdatedAt: t0,
			}
			if err := store.InsertRatePeriod(ctx, db, esA, p); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, p.ID)
		}
		dup := &store.RatePeriod{ID: store.NewID(), SeasonID: sn.ID, Code: "low", CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertRatePeriod(ctx, db, esA, dup); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate code %v", err)
		}
		// Ordered by sort_order: high, mid, low; paginated one by one.
		var codes []string
		cursor := ""
		for {
			page, err := store.ListRatePeriods(ctx, db, esA, sn.ID, cursor, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range page.Items {
				codes = append(codes, p.Code)
			}
			if cursor = page.NextCursor; cursor == "" {
				break
			}
		}
		if len(codes) != 3 || codes[0] != "high" || codes[2] != "low" {
			t.Fatalf("order %v", codes)
		}
		all, err := store.ListSeasonRatePeriods(ctx, db, esA, sn.ID)
		if err != nil || len(all) != 3 || all[0].Ranges[0].From.IsZero() || all[0].Name["it"] == "" {
			t.Fatalf("season periods %+v %v", all, err)
		}
		if page, _ := store.ListRatePeriods(ctx, db, esB, "", "", 10); len(page.Items) != 0 {
			t.Fatal("rate periods leaked across establishments")
		}
		if ok, err := store.DeleteRatePeriodIfVersion(ctx, db, esA, ids[0], 2); err != nil || ok {
			t.Fatalf("stale delete %v %v", ok, err)
		}
		if ok, err := store.DeleteRatePeriodIfVersion(ctx, db, esA, ids[0], 1); err != nil || !ok {
			t.Fatalf("delete %v %v", ok, err)
		}

		for _, day := range []int{12, 3, 20} {
			c := &store.ClosureDay{ID: store.NewID(), Date: calendar.New(2027, 7, day), Slots: []calendar.Slot{calendar.AM, calendar.PM}, Reason: "x", CreatedAt: t0, UpdatedAt: t0}
			if err := store.InsertClosureDay(ctx, db, esA, c); err != nil {
				t.Fatal(err)
			}
		}
		dupDay := &store.ClosureDay{ID: store.NewID(), Date: calendar.New(2027, 7, 3), Slots: []calendar.Slot{calendar.AM}, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertClosureDay(ctx, db, esA, dupDay); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate date %v", err)
		}
		// The same date in another establishment is fine.
		if err := store.InsertClosureDay(ctx, db, esB, dupDay); err != nil {
			t.Fatal(err)
		}
		page, err := store.ListClosureDays(ctx, db, esA, store.ClosureDayFilter{From: calendar.New(2027, 7, 4)}, "", 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].Date.Day() != 12 || page.NextCursor == "" {
			t.Fatalf("closures page 1 %+v %v", page, err)
		}
		page, err = store.ListClosureDays(ctx, db, esA, store.ClosureDayFilter{From: calendar.New(2027, 7, 4), To: calendar.New(2027, 7, 31)}, page.NextCursor, 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].Date.Day() != 20 || len(page.Items[0].Slots) != 2 {
			t.Fatalf("closures page 2 %+v %v", page, err)
		}
		c := page.Items[0]
		c.Reason, c.Version = "Maltempo", 2
		if ok, err := store.UpdateClosureDayIfVersion(ctx, db, esA, &c, 1); err != nil || !ok {
			t.Fatalf("update closure %v %v", ok, err)
		}
		if ok, _ := store.UpdateClosureDayIfVersion(ctx, db, esB, &c, 2); ok {
			t.Fatal("cross scope update")
		}
	})
}
