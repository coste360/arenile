//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00002_ModelsRoundtrip inserts and reloads one row of every
// M1/M2 model on both dialects, and checks the unique constraints the
// services rely on.
func TestMigration00002_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m1", "Lido M1", t0)
		if err != nil {
			t.Fatal(err)
		}
		reloaded, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		e, err := store.GetEstablishment(ctx, db, reloaded)
		if err != nil || e.Version != 1 {
			t.Fatalf("establishment version: %d %v", e.Version, err)
		}
		insert := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		expectConflict := func(m any) {
			t.Helper()
			_, err := db.NewInsert().Model(m).Exec(ctx)
			if !store.IsUniqueViolation(err) {
				t.Fatalf("insert %T: want unique violation, got %v", m, err)
			}
		}
		d := calendar.MustParse
		season := &store.Season{
			ID: store.NewID(), EstablishmentID: est.ID, Year: 2027, OpensOn: d("2027-05-01"),
			ClosesOn: d("2027-09-30"), Status: "draft", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(season)
		insert(&store.ClosureDay{
			ID: store.NewID(), EstablishmentID: est.ID, Date: d("2027-06-02"),
			Slots: []calendar.Slot{calendar.AM}, Reason: "maltempo", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		rp := &store.RatePeriod{
			ID: store.NewID(), EstablishmentID: est.ID, SeasonID: season.ID, Code: "peak",
			Name: store.I18nText{"it": "Ferragosto"}, Color: "#ff0000",
			Ranges: []store.DateRange{{From: d("2027-08-10"), To: d("2027-08-20")}}, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(rp)
		m := &store.Map{
			ID: store.NewID(), EstablishmentID: est.ID, Name: store.I18nText{"it": "Spiaggia"}, Width: 1000,
			Height: 600, GridSize: 10, SeaSide: "bottom", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(m)
		zone := &store.Zone{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: &m.ID, Code: "row1",
			Name: store.I18nText{"it": "Prima fila"}, AssignmentMode: "zone", OnlineVisible: true,
			OnlineQuota: &store.OnlineQuota{Type: "percent", Value: 50}, PriceCategory: "row1", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(zone)
		spot := func(label string) *store.Spot {
			return &store.Spot{
				ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, ZoneID: zone.ID, Label: label,
				RowLabel: "A", Number: 1, Kind: "umbrella", X: 10.5, Y: 20, Shape: store.Shape{Type: "circle", W: 8, H: 8},
				Capacity: map[string]int{"umbrella": 1, "sunbed": 2}, MaxExtra: map[string]int{}, Attributes: []string{"accessible"},
				Active: true, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(spot("A1"))
		expectConflict(spot("A1"))
		insert(&store.MapDecoration{
			ID: store.NewID(), EstablishmentID: est.ID, MapID: m.ID, Kind: "shower",
			Shape: store.Shape{Type: "rect", W: 4, H: 4}, CreatedAt: t0, UpdatedAt: t0,
		})
		pool := func() *store.Pool {
			return &store.Pool{
				ID: store.NewID(), EstablishmentID: est.ID, ZoneID: &zone.ID, ComponentCode: "umbrella",
				Name: store.I18nText{"it": "Prima fila"}, Capacity: 1, CapacityOverrides: []store.CapacityOverride{},
				SystemManaged: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		pl := pool()
		insert(pl)
		expectConflict(pool())
		insert(&store.Component{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "sunbed", Name: store.I18nText{"it": "Lettino"},
			Tracked: true, PoolID: &pl.ID, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		vat := int64(2200)
		pack := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "base", Name: store.I18nText{"it": "Base"},
			Description: store.I18nText{}, Composition: map[string]int{"umbrella": 1, "sunbed": 2},
			AllowedFormulas: []string{"day"}, Channels: []string{"desk", "online"}, MaxPerBooking: 1, Active: true,
			VATRateBP: &vat, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pack)
		insert(&store.Extra{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "sunbed", Name: store.I18nText{"it": "Lettino"},
			Description: store.I18nText{}, PricingUnit: "per_day", MaxQuantity: 2, AttachTo: "line",
			Channels: []string{"desk"}, Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		insert(&store.PriceRule{
			ID: store.NewID(), EstablishmentID: est.ID, SeasonID: season.ID, Name: "base",
			TargetType: "pack", Conditions: store.JSON(`{"rate_periods":["peak"]}`), Action: store.JSON(`{"type":"set","amount":3500}`),
			Stage: "base", Active: true, Seq: 1, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		fixed := func(code *string) *store.FixedDurationPrice {
			return &store.FixedDurationPrice{
				ID: store.NewID(), EstablishmentID: est.ID, SeasonID: season.ID, PackID: pack.ID,
				Formula: "week", PriceCategory: "row1", RatePeriodCode: code, Amount: 21000, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(fixed(nil))
		expectConflict(fixed(nil))
		peak := "peak"
		insert(fixed(&peak))
		promo := func(code string) *store.PromoCode {
			return &store.PromoCode{
				ID: store.NewID(), EstablishmentID: est.ID, Code: code, Action: store.JSON(`{"type":"percent_off","percent":10}`),
				Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(promo("ESTATE10"))
		expectConflict(promo("estate10"))
		insert(&store.Agreement{
			ID: store.NewID(), EstablishmentID: est.ID, Name: "Hotel Mare", Action: store.JSON(`{"type":"percent_off","percent":5}`),
			Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		insert(&store.BookingPolicy{
			ID: store.NewID(), EstablishmentID: est.ID, SeasonID: season.ID, Name: "Agosto",
			Scope: store.JSON(`{"rate_periods":["peak"]}`), Rules: store.JSON(`{"approval_required":true}`), Priority: 10,
			Active: true, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})

		// Reload a few rows to check JSON, date and nullable columns.
		var gotRP store.RatePeriod
		if err := db.NewSelect().Model(&gotRP).Where("rp.id = ?", rp.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotRP.Name["it"] != "Ferragosto" || len(gotRP.Ranges) != 1 || gotRP.Ranges[0].To.String() != "2027-08-20" {
			t.Fatalf("rate period %+v", gotRP)
		}
		var gotPack store.Pack
		if err := db.NewSelect().Model(&gotPack).Where("pa.id = ?", pack.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotPack.AllowedZoneIDs != nil || gotPack.RequiresSpotKind != nil || gotPack.Composition["sunbed"] != 2 ||
			gotPack.VATRateBP == nil || *gotPack.VATRateBP != 2200 {
			t.Fatalf("pack %+v", gotPack)
		}
		var gotZone store.Zone
		if err := db.NewSelect().Model(&gotZone).Where("z.id = ?", zone.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotZone.OnlineQuota == nil || gotZone.OnlineQuota.Value != 50 || gotZone.MapID == nil {
			t.Fatalf("zone %+v", gotZone)
		}
		var gotSeason store.Season
		if err := db.NewSelect().Model(&gotSeason).Where("sn.id = ?", season.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotSeason.OpensOn.String() != "2027-05-01" {
			t.Fatalf("season %+v", gotSeason)
		}
	})
}
