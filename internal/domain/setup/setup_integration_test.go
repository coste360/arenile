//go:build integration

package setup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// fixture is an establishment with principals of every role.
type fixture struct {
	db    *store.DB
	svc   *Service
	clock *clock.Fake
	est   *store.Establishment
	org   *store.Organization
}

var t0 = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC) // next season: 2027

func newFixture(t *testing.T, db *store.DB, slug string) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(t0)
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org "+slug, slug, "Lido "+slug, t0)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)})
	return &fixture{db: db, svc: svc, clock: c, est: est, org: org}
}

func (f *fixture) principal(role auth.Role) *auth.Principal {
	return &auth.Principal{
		Kind: auth.PrincipalUser, UserID: store.NewID(), Email: "x@example.it",
		Grants: []auth.Grant{{OrganizationID: f.org.ID, Role: role}},
	}
}

func (f *fixture) owner() *auth.Principal { return f.principal(auth.RoleOwner) }

// engineData loads the catalog and the rule set of a season in the shapes of
// the pure pricing engine.
func engineData(t *testing.T, db *store.DB, est *store.Establishment, seasonID string) (pricing.Catalog, pricing.RuleSet) {
	t.Helper()
	ctx := context.Background()
	settings, err := establishment.ParseSettings(est.Settings, est.Currency)
	if err != nil {
		t.Fatal(err)
	}
	cat := pricing.Catalog{Settings: settings.Pricing}
	var packs []store.Pack
	var extras []store.Extra
	var zones []store.Zone
	var spots []store.Spot
	var periods []store.RatePeriod
	var rules []store.PriceRule
	var fixed []store.FixedDurationPrice
	for _, q := range []struct {
		dst   any
		alias string
	}{{&packs, "pa"}, {&extras, "ex"}, {&zones, "z"}, {&spots, "sp"}} {
		if err := db.NewSelect().Model(q.dst).Where(q.alias+".establishment_id = ?", est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []struct {
		dst   any
		alias string
	}{{&periods, "rp"}, {&rules, "pr"}, {&fixed, "fp"}} {
		if err := db.NewSelect().Model(q.dst).Where(q.alias+".establishment_id = ?", est.ID).
			Where(q.alias+".season_id = ?", seasonID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range packs {
		cat.Packs = append(cat.Packs, pricing.Pack{ID: p.ID, Code: p.Code, VATRateBP: p.VATRateBP, Active: p.Active})
	}
	for _, x := range extras {
		cat.Extras = append(cat.Extras, pricing.Extra{ID: x.ID, Code: x.Code, PricingUnit: pricing.PricingUnit(x.PricingUnit), VATRateBP: x.VATRateBP, Active: x.Active})
	}
	for _, z := range zones {
		cat.Zones = append(cat.Zones, pricing.Zone{ID: z.ID, Code: z.Code, PriceCategory: z.PriceCategory})
	}
	for _, s := range spots {
		ps := pricing.Spot{ID: s.ID, Label: s.Label, ZoneID: s.ZoneID, Attributes: s.Attributes}
		if s.PriceCategoryOverride != nil {
			ps.PriceCategoryOverride = *s.PriceCategoryOverride
		}
		cat.Spots = append(cat.Spots, ps)
	}
	var rs pricing.RuleSet
	for _, p := range periods {
		rp := pricing.RatePeriod{ID: p.ID, Code: p.Code}
		for _, r := range p.Ranges {
			rp.Ranges = append(rp.Ranges, pricing.DateRange{From: r.From, To: r.To})
		}
		rs.RatePeriods = append(rs.RatePeriods, rp)
	}
	for _, r := range rules {
		er := pricing.Rule{
			ID: r.ID, Name: r.Name, TargetType: pricing.TargetType(r.TargetType), TargetIDs: r.TargetIDs,
			Priority: r.Priority, Stage: pricing.Stage(r.Stage), Stackable: r.Stackable, Active: r.Active, Seq: r.Seq,
		}
		if err := r.Conditions.Unmarshal(&er.Conditions); err != nil {
			t.Fatal(err)
		}
		if err := r.Action.Unmarshal(&er.Action); err != nil {
			t.Fatal(err)
		}
		rs.Rules = append(rs.Rules, er)
	}
	for _, fp := range fixed {
		rs.FixedPrices = append(rs.FixedPrices, pricing.FixedDurationPrice{
			ID: fp.ID, PackID: fp.PackID, Formula: pricing.Formula(fp.Formula), PriceCategory: fp.PriceCategory,
			RatePeriodCode: fp.RatePeriodCode, Amount: money.Cents(fp.Amount),
		})
	}
	if err := pricing.ValidateRuleSet(rs); err != nil {
		t.Fatalf("rule set: %v", err)
	}
	return cat, rs
}

func loadSeasonRow(t *testing.T, db *store.DB, id string) store.Season {
	t.Helper()
	var sn store.Season
	if err := db.NewSelect().Model(&sn).Where("sn.id = ?", id).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sn
}

func assertNoGaps(t *testing.T, db *store.DB, est *store.Establishment, seasonID string, slots calendar.Slots) {
	t.Helper()
	sn := loadSeasonRow(t, db, seasonID)
	cat, rs := engineData(t, db, est, seasonID)
	gaps, err := pricing.Coverage(pricing.CoverageRequest{From: sn.OpensOn, To: sn.ClosesOn, Slots: slots}, cat, rs)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) > 0 {
		t.Fatalf("coverage gaps (%s): %+v", slots, gaps)
	}
}

// quickPricesFor builds a complete quick price list for the categories in
// use and the template periods.
func quickPricesFor(t *testing.T, db *store.DB, est *store.Establishment) []QuickPrice {
	t.Helper()
	es, err := store.SystemEstablishmentScope(context.Background(), db, est.ID)
	if err != nil {
		t.Fatal(err)
	}
	cats, err := categoriesInUse(context.Background(), db, es)
	if err != nil {
		t.Fatal(err)
	}
	var out []QuickPrice
	for i, c := range cats {
		for j, p := range Periods {
			out = append(out, QuickPrice{PriceCategory: c, RatePeriodCode: p.Code, Amount: money.Cents(1500 + 100*i + 300*j)})
		}
	}
	return out
}

func count(t *testing.T, db *store.DB, model any, alias, estID string) int {
	t.Helper()
	n, err := db.NewSelect().Model(model).Where(alias+".establishment_id = ?", estID).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// TestApply_ClassicWithQuickPricesHasNoCoverageGaps is the M1 acceptance
// path: classic template 8 x 15 with the quick price list, then the pure
// coverage check on the stored data reports no gap for full and half days.
func TestApply_ClassicWithQuickPricesHasNoCoverageGaps(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "classic")
		var prices []QuickPrice
		for i, c := range []string{CategoryRow1, CategoryRow2, CategoryStandard, CategoryGazebo} {
			for j, p := range Periods {
				prices = append(prices, QuickPrice{PriceCategory: c, RatePeriodCode: p.Code, Amount: money.Cents(2000 + 500*i + 250*j)})
			}
		}
		half := pricing.MustParseDecimal("0.6")
		res, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{
			Template: CodeClassicRows, QuickPrices: &QuickPrices{Prices: prices, HalfDayFactor: &half}, ClientIP: "203.0.113.1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.SeasonYear != 2027 || !res.SeasonCreated || res.SpotsCreated != 8*15+4 || len(res.ZoneIDs) != 4 ||
			len(res.RatePeriodIDs) != 4 || len(res.PackIDs) != 3 || len(res.ExtraIDs) != 4 || len(res.ComponentIDs) != 5 ||
			len(res.PolicyIDs) != 3 || len(res.QuickRules) != 16*3 || res.MapID == "" {
			t.Fatalf("result: %+v", res)
		}
		sn := loadSeasonRow(t, db, res.SeasonID)
		if sn.OpensOn.String() != "2027-05-01" || sn.ClosesOn.String() != "2027-09-30" || sn.Status != SeasonDraft {
			t.Fatalf("season: %+v", sn)
		}

		// Rows A-H with 15 umbrellas each; row A is row1, row B row2, then standard.
		var spots []store.Spot
		if err := db.NewSelect().Model(&spots).Where("sp.establishment_id = ?", f.est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		var zones []store.Zone
		if err := db.NewSelect().Model(&zones).Where("z.establishment_id = ?", f.est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		zoneCat := map[string]string{}
		for _, z := range zones {
			zoneCat[z.ID] = z.PriceCategory
		}
		perRow := map[string]int{}
		for _, s := range spots {
			perRow[s.RowLabel]++
			want := CategoryStandard
			switch s.RowLabel {
			case "A":
				want = CategoryRow1
			case "B":
				want = CategoryRow2
			case "GZ":
				want = CategoryGazebo
			}
			if zoneCat[s.ZoneID] != want {
				t.Fatalf("spot %s in category %s, want %s", s.Label, zoneCat[s.ZoneID], want)
			}
			if s.Label != fmt.Sprintf("%s%d", s.RowLabel, s.Number) {
				t.Fatalf("label %s", s.Label)
			}
		}
		for _, r := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
			if perRow[r] != 15 {
				t.Fatalf("row %s: %d spots", r, perRow[r])
			}
		}
		if perRow["GZ"] != 4 {
			t.Fatalf("gazebos: %d", perRow["GZ"])
		}
		var m store.Map
		if err := db.NewSelect().Model(&m).Where("m.id = ?", res.MapID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, s := range spots {
			if s.X < 0 || s.Y < 0 || s.X > float64(m.Width) || s.Y > float64(m.Height) {
				t.Fatalf("spot %s outside the map %dx%d: %v,%v", s.Label, m.Width, m.Height, s.X, s.Y)
			}
		}
		if count(t, db, (*store.MapDecoration)(nil), "md", f.est.ID) == 0 {
			t.Fatal("decorations expected")
		}

		assertNoGaps(t, db, f.est, res.SeasonID, calendar.SlotsFull)
		assertNoGaps(t, db, f.est, res.SeasonID, calendar.SlotsAM)

		// Half day uses the generated slot rule: round_half_even(2000 * 0.6).
		cat, rs := engineData(t, db, f.est, res.SeasonID)
		var rowA string
		for _, s := range spots {
			if s.Label == "A1" {
				rowA = s.ID
			}
		}
		var basePack string
		for _, p := range cat.Packs {
			if p.Code == PackBase {
				basePack = p.ID
			}
		}
		d := calendar.MustParse("2027-05-10")
		q, err := pricing.Quote(pricing.QuoteInput{
			Channel: pricing.ChannelOnline, StartsOn: d, EndsOn: d, Slots: calendar.SlotsPM, Formula: pricing.FormulaHalfDay,
			Today: d.AddDays(-3), Lines: []pricing.LineInput{{PackID: basePack, SpotID: rowA}},
		}, cat, rs)
		if err != nil || q.Total != 1200 {
			t.Fatalf("half day quote: %d %v", q.Total, err)
		}

		// Audit and events in the same transaction.
		var actions []string
		if err := db.NewSelect().Model((*store.AuditEntry)(nil)).Column("action").
			Where("establishment_id = ?", f.est.ID).Scan(ctx, &actions); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(actions, "setup.template_applied") {
			t.Fatalf("audit: %v", actions)
		}
		var types []string
		if err := db.NewSelect().Model((*store.OutboxEvent)(nil)).Column("type").
			Where("establishment_id = ?", f.est.ID).Scan(ctx, &types); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(types, event.ConfigMapChanged) || !slices.Contains(types, event.ConfigPricingChanged) {
			t.Fatalf("events: %v", types)
		}
	})
}

// TestApply_EveryTemplate applies each template to a fresh establishment,
// then the quick price list, and checks the coverage.
func TestApply_EveryTemplate(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		want := map[Code]struct{ spots, zones, maps, pools int }{
			CodeClassicRows:   {124, 4, 1, 2},
			CodeQuantityBeach: {0, 1, 0, 2},
			CodeLidoWithPool:  {6*12 + 20, 5, 2, 2 + 1 + 2},
			CodeEmpty:         {0, 0, 0, 0},
		}
		for _, tpl := range Templates() {
			t.Run(string(tpl.Code), func(t *testing.T) {
				f := newFixture(t, db, "tpl-"+string(tpl.Code[:4]))
				res, err := f.svc.Apply(ctx, f.principal(auth.RoleManager), f.est.ID, ApplyInput{Template: tpl.Code})
				if err != nil {
					t.Fatal(err)
				}
				w := want[tpl.Code]
				if res.SpotsCreated != w.spots || len(res.ZoneIDs) != w.zones || len(res.MapIDs) != w.maps || len(res.PoolIDs) != w.pools {
					t.Fatalf("result %+v", res)
				}
				if len(res.RatePeriodIDs) != 4 || res.SeasonID == "" {
					t.Fatalf("season/periods: %+v", res)
				}
				prices := quickPricesFor(t, db, f.est)
				if tpl.Code == CodeEmpty {
					prices = []QuickPrice{
						{PriceCategory: "standard", RatePeriodCode: PeriodLow, Amount: 1000},
						{PriceCategory: "standard", RatePeriodCode: PeriodMid, Amount: 1200},
						{PriceCategory: "standard", RatePeriodCode: PeriodHigh, Amount: 1500},
						{PriceCategory: "standard", RatePeriodCode: PeriodPeak, Amount: 1800},
					}
				}
				qr, err := f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, QuickPricelistInput{SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: prices}})
				if err != nil {
					t.Fatal(err)
				}
				if len(qr.Rules) != len(prices) || qr.Replaced != 0 {
					t.Fatalf("quick: %d rules, %d replaced", len(qr.Rules), qr.Replaced)
				}
				assertNoGaps(t, db, f.est, res.SeasonID, calendar.SlotsFull)
				assertNoGaps(t, db, f.est, res.SeasonID, calendar.SlotsPM)
				if tpl.Code == CodeLidoWithPool {
					var pl store.Pool
					if err := db.NewSelect().Model(&pl).Where("pl.establishment_id = ?", f.est.ID).
						Where("pl.system_managed = ?", true).Scan(ctx); err != nil {
						t.Fatal(err)
					}
					if pl.Capacity != 20 || pl.ZoneID == nil {
						t.Fatalf("implicit pool: %+v", pl)
					}
				}
			})
		}
	})
}

func TestApply_Idempotency(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "idem")
		first, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: CodeClassicRows})
		if err != nil {
			t.Fatal(err)
		}
		spots := count(t, db, (*store.Spot)(nil), "sp", f.est.ID)
		// A second template is refused and changes nothing.
		for _, code := range []Code{CodeClassicRows, CodeQuantityBeach, CodeLidoWithPool} {
			if _, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: code}); !errors.Is(err, ErrAlreadyConfigured) ||
				!errors.Is(err, configerr.ErrConflict) {
				t.Fatalf("%s: want ErrAlreadyConfigured, got %v", code, err)
			}
		}
		if got := count(t, db, (*store.Spot)(nil), "sp", f.est.ID); got != spots {
			t.Fatalf("spots changed: %d -> %d", spots, got)
		}
		// "empty" is always allowed: it reuses the season of the year.
		empty, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: CodeEmpty})
		if err != nil || empty.SeasonID != first.SeasonID || empty.SeasonCreated || !slices.Equal(empty.RatePeriodIDs, first.RatePeriodIDs) {
			t.Fatalf("empty: %+v %v", empty, err)
		}
		// Force adds a second map: catalog reused, zone codes suffixed, policies
		// kept. With 2 rows the "standard" zone has no spots and is not created.
		forced, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: CodeClassicRows, Force: true, Rows: ptr(2), SpotsPerRow: ptr(5)})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(forced.PackIDs, first.PackIDs) || !slices.Equal(forced.ComponentIDs, first.ComponentIDs) ||
			!slices.Equal(forced.ExtraIDs, first.ExtraIDs) || len(forced.PolicyIDs) != 0 || forced.SpotsCreated != 2*5+4 {
			t.Fatalf("forced: %+v", forced)
		}
		var codes []string
		if err := db.NewSelect().Model((*store.Zone)(nil)).Column("code").Where("z.establishment_id = ?", f.est.ID).
			OrderExpr("z.code").Scan(ctx, &codes); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(codes, "row1_2") || !slices.Contains(codes, "gazebo_2") || slices.Contains(codes, "standard_2") || len(codes) != 7 {
			t.Fatalf("zone codes: %v", codes)
		}
		if n := count(t, db, (*store.Map)(nil), "m", f.est.ID); n != 2 {
			t.Fatalf("maps: %d", n)
		}
	})
}

func TestApply_Permissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "perm")
		other := newFixture(t, db, "perm-other")
		for _, role := range []auth.Role{auth.RoleCashier, auth.RoleBeachStaff, auth.RoleViewer} {
			if _, err := f.svc.Apply(ctx, f.principal(role), f.est.ID, ApplyInput{Template: CodeEmpty}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s apply: %v", role, err)
			}
			if _, err := f.svc.QuickPricelist(ctx, f.principal(role), f.est.ID, QuickPricelistInput{SeasonID: "x"}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s quick: %v", role, err)
			}
			// Every role can read templates and status.
			if ts, err := f.svc.ListTemplates(ctx, f.principal(role), f.est.ID); err != nil || len(ts) != 4 {
				t.Fatalf("%s templates: %v", role, err)
			}
			if _, err := f.svc.Status(ctx, f.principal(role), f.est.ID); err != nil {
				t.Fatalf("%s status: %v", role, err)
			}
		}
		if _, err := f.svc.Apply(ctx, nil, f.est.ID, ApplyInput{Template: CodeEmpty}); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
		// Another organization's owner cannot see the establishment.
		if _, err := f.svc.Apply(ctx, other.owner(), f.est.ID, ApplyInput{Template: CodeEmpty}); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign: %v", err)
		}
		if _, err := f.svc.ListTemplates(ctx, other.owner(), f.est.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("foreign templates: %v", err)
		}
		// A season of another establishment is not found.
		res, err := other.svc.Apply(ctx, other.owner(), other.est.ID, ApplyInput{Template: CodeEmpty})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, QuickPricelistInput{
			SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: []QuickPrice{{PriceCategory: "a", RatePeriodCode: PeriodLow, Amount: 1}}},
		})
		if !errors.Is(err, ErrSeasonNotFound) {
			t.Fatalf("foreign season: %v", err)
		}
		// Nothing was written for the refused calls.
		if n := count(t, db, (*store.Season)(nil), "sn", f.est.ID); n != 0 {
			t.Fatalf("seasons: %d", n)
		}
	})
}

// TestApply_ClassicTiming is the timing sanity check of the M1 acceptance
// (the manager flow must take minutes, the server part well under a second).
// Enforced: the template is written in a bounded number of statements
// (measured 43 for 124 spots: batched inserts, not one per spot); the 1s budget applies only with
// ARENILE_PERF_BUDGETS=1 (storetest.Perf), the time is always logged.
func TestApply_ClassicTiming(t *testing.T) {
	db, qc := storetest.CountQueries(storetest.SQLite(t))
	f := newFixture(t, db, "timing")
	var res *ApplyResult
	storetest.Perf(t, qc, "classic template", 60, time.Second, func() (err error) {
		res, err = f.svc.Apply(context.Background(), f.owner(), f.est.ID, ApplyInput{Template: CodeClassicRows})
		return err
	})
	t.Logf("%d spots", res.SpotsCreated)
}

func TestApply_Validation(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	f := newFixture(t, db, "valid")
	d := calendar.MustParse
	tests := []struct {
		name  string
		in    ApplyInput
		field string
	}{
		{"unknown template", ApplyInput{Template: "beach_club"}, "template"},
		{"zero rows", ApplyInput{Template: CodeClassicRows, Rows: ptr(0)}, "rows"},
		{"too many spots", ApplyInput{Template: CodeLidoWithPool, SpotsPerRow: ptr(201)}, "spots_per_row"},
		{"closes before opens", ApplyInput{Template: CodeEmpty, OpensOn: ptr(d("2027-06-01")), ClosesOn: ptr(d("2027-05-01"))}, "closes_on"},
		{"opens in another year", ApplyInput{Template: CodeEmpty, SeasonYear: ptr(2027), OpensOn: ptr(d("2028-05-01"))}, "opens_on"},
		{"year out of range", ApplyInput{Template: CodeEmpty, SeasonYear: ptr(1999)}, "season_year"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.svc.Apply(ctx, f.owner(), f.est.ID, tt.in)
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) || !slices.ContainsFunc(ve.Fields, func(fe configerr.FieldError) bool { return fe.Field == tt.field }) {
				t.Fatalf("want validation error on %s, got %v", tt.field, err)
			}
		})
	}
	// Custom dates: the periods follow the season and still cover it.
	res, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{
		Template: CodeQuantityBeach, OpensOn: ptr(d("2027-06-15")), ClosesOn: ptr(d("2027-09-10")),
		MapName: store.I18nText{"it": "ignorato"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RatePeriodIDs) != 3 { // mid, high, peak
		t.Fatalf("periods: %d", len(res.RatePeriodIDs))
	}
}

func TestQuickPricelist_Scenarios(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	f := newFixture(t, db, "quick")
	res, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: CodeQuantityBeach})
	if err != nil {
		t.Fatal(err)
	}
	full := func(amount money.Cents) []QuickPrice {
		var out []QuickPrice
		for _, p := range Periods {
			out = append(out, QuickPrice{PriceCategory: CategoryStandard, RatePeriodCode: p.Code, Amount: amount})
		}
		return out
	}
	in := func(prices []QuickPrice) QuickPricelistInput {
		return QuickPricelistInput{SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: prices}}
	}
	invalid := []struct {
		name   string
		in     QuickPricelistInput
		field  string
		substr string
	}{
		{"missing period", in(full(1000)[:3]), "prices", "standard/peak"},
		{"unknown category", in(append(full(1000), QuickPrice{PriceCategory: "row1", RatePeriodCode: PeriodLow, Amount: 1})), "prices[4].price_category", ""},
		{"unknown period", in(append(full(1000), QuickPrice{PriceCategory: CategoryStandard, RatePeriodCode: "xmas", Amount: 1})), "prices[4].rate_period_code", ""},
		{"duplicate", in(append(full(1000), QuickPrice{PriceCategory: CategoryStandard, RatePeriodCode: PeriodLow, Amount: 1})), "prices[4]", ""},
		{"negative", in([]QuickPrice{{PriceCategory: CategoryStandard, RatePeriodCode: PeriodLow, Amount: -1}}), "prices[0].amount", ""},
		{"bad category", in([]QuickPrice{{PriceCategory: "Row 1", RatePeriodCode: PeriodLow, Amount: 1}}), "prices[0].price_category", ""},
		{"empty", in(nil), "prices", ""},
		{"unknown pack", QuickPricelistInput{SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: full(1), PackIDs: []string{store.NewID()}}}, "pack_ids[0]", ""},
		{"bad factor", QuickPricelistInput{SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: full(1), HalfDayFactor: ptr(pricing.MustParseDecimal("1.5"))}}, "half_day_factor", ""},
		{"no season", QuickPricelistInput{QuickPrices: QuickPrices{Prices: full(1)}}, "season_id", ""},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, tt.in)
			var ve *configerr.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want validation error, got %v", err)
			}
			found := false
			for _, fe := range ve.Fields {
				if fe.Field == tt.field && (tt.substr == "" || strings.Contains(fe.Message, tt.substr)) {
					found = true
				}
			}
			if !found {
				t.Fatalf("fields %+v, want %s", ve.Fields, tt.field)
			}
		})
	}

	// A selected pack restricts the target; generated rules carry it.
	var packs []store.Pack
	if err := db.NewSelect().Model(&packs).Where("pa.establishment_id = ?", f.est.ID).OrderExpr("pa.sort_order").Scan(ctx); err != nil {
		t.Fatal(err)
	}
	half := pricing.MustParseDecimal("0.5")
	first, err := f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, QuickPricelistInput{
		SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: full(1001), PackIDs: []string{packs[0].ID, packs[0].ID}, HalfDayFactor: &half},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rules) != 12 || !slices.Equal(first.Rules[0].TargetIDs, []string{packs[0].ID}) {
		t.Fatalf("rules: %d %+v", len(first.Rules), first.Rules[0])
	}
	var halfAmount pricing.Action
	if err := first.Rules[1].Action.Unmarshal(&halfAmount); err != nil || halfAmount.Amount != 500 { // 1001 * 0.5 half-even
		t.Fatalf("half rule: %+v %v", halfAmount, err)
	}
	seqs := map[int64]bool{}
	for _, r := range first.Rules {
		seqs[r.Seq] = true
	}
	if len(seqs) != 12 {
		t.Fatal("seq must be distinct")
	}
	// Without replace_existing the second list is a conflict.
	if _, err := f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, in(full(2000))); !errors.Is(err, ErrQuickRulesExist) {
		t.Fatalf("want conflict, got %v", err)
	}
	// A manual base rule survives a replace.
	if _, err := db.NewInsert().Model(&store.PriceRule{
		ID: store.NewID(), EstablishmentID: f.est.ID, SeasonID: res.SeasonID, Name: "Manuale", TargetType: "pack",
		Conditions: store.JSON(`{}`), Action: store.JSON(`{"type":"set","amount":1}`), Stage: "base", Active: true, Seq: 999,
		Version: 1, CreatedAt: t0, UpdatedAt: t0,
	}).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.QuickPricelist(ctx, f.owner(), f.est.ID, QuickPricelistInput{
		SeasonID: res.SeasonID, QuickPrices: QuickPrices{Prices: full(2000)}, ReplaceExisting: true,
	})
	if err != nil || second.Replaced != 12 || len(second.Rules) != 4 || second.Rules[0].TargetIDs != nil || second.Rules[0].Seq <= 999 {
		t.Fatalf("replace: %+v %v", second, err)
	}
	if n := count(t, db, (*store.PriceRule)(nil), "pr", f.est.ID); n != 5 {
		t.Fatalf("rules after replace: %d", n)
	}
	assertNoGaps(t, db, f.est, res.SeasonID, calendar.SlotsFull)
}

func TestStatus_Progress(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	f := newFixture(t, db, "status")
	st, err := f.svc.Status(ctx, f.owner(), f.est.ID)
	if err != nil || st.HasSeason || st.HasRatePeriods || st.HasMap || st.SpotCount != 0 || st.HasCatalog || st.HasPrices || st.BookingEnabled || len(st.PriceCategories) != 0 {
		t.Fatalf("empty status: %+v %v", st, err)
	}
	half := pricing.MustParseDecimal("0.5")
	var prices []QuickPrice
	for _, c := range []string{CategoryRow1, CategoryRow2, CategoryStandard, CategoryGazebo} {
		for _, p := range Periods {
			prices = append(prices, QuickPrice{PriceCategory: c, RatePeriodCode: p.Code, Amount: 1000})
		}
	}
	if _, err := f.svc.Apply(ctx, f.owner(), f.est.ID, ApplyInput{Template: CodeClassicRows, QuickPrices: &QuickPrices{Prices: prices, HalfDayFactor: &half}}); err != nil {
		t.Fatal(err)
	}
	st, err = f.svc.Status(ctx, f.principal(auth.RoleViewer), f.est.ID)
	if err != nil || !st.HasSeason || !st.HasRatePeriods || !st.HasMap || st.SpotCount != 124 || !st.HasCatalog || !st.HasPrices || st.BookingEnabled {
		t.Fatalf("status: %+v %v", st, err)
	}
	// The categories the quick price list requires: zones plus spot
	// overrides (the wizard grid builds its rows from them).
	if want := []string{CategoryGazebo, CategoryRow1, CategoryRow2, CategoryStandard}; !slices.Equal(st.PriceCategories, want) {
		t.Fatalf("price categories %v, want %v", st.PriceCategories, want)
	}
	if _, err := db.NewUpdate().TableExpr("spots").Set("price_category_override = ?", "angolo").
		Where("establishment_id = ? AND label = ?", f.est.ID, "A1").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	st, err = f.svc.Status(ctx, f.owner(), f.est.ID)
	if err != nil || !slices.Contains(st.PriceCategories, "angolo") || len(st.PriceCategories) != 5 {
		t.Fatalf("categories with override: %v %v", st.PriceCategories, err)
	}
}

func TestSeedDemo_IdempotentAndComplete(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "lido-demo")
		first, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil {
			t.Fatal(err)
		}
		// The fixture clock is in October 2026: the demo season is still 2026,
		// open every day of the year.
		if !first.Applied || first.SeasonYear != 2026 || first.SpotsCreated != 124 || first.RulesCreated != 16*3+4+1 {
			t.Fatalf("first: %+v", first)
		}
		second, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil || second.Applied || second.MapsUpgraded != 0 {
			t.Fatalf("second: %+v %v", second, err)
		}
		if n := count(t, db, (*store.Spot)(nil), "sp", f.est.ID); n != 124 {
			t.Fatalf("spots after second seed: %d", n)
		}
		sn := loadSeasonRow(t, db, first.SeasonID)
		if sn.Status != SeasonActive || sn.OpensOn.String() != "2026-01-01" || sn.ClosesOn.String() != "2026-12-31" {
			t.Fatalf("season %s %s – %s", sn.Status, sn.OpensOn, sn.ClosesOn)
		}
		for _, c := range []struct {
			model any
			alias string
			want  int
		}{
			{(*store.BookingPolicy)(nil), "bp", 4},
			{(*store.FixedDurationPrice)(nil), "fp", 6},
			{(*store.PromoCode)(nil), "pc", 1},
			{(*store.Agreement)(nil), "ag", 1},
		} {
			if n := count(t, db, c.model, c.alias, f.est.ID); n != c.want {
				t.Fatalf("%T: %d, want %d", c.model, n, c.want)
			}
		}
		assertNoGaps(t, db, f.est, first.SeasonID, calendar.SlotsFull)
		assertNoGaps(t, db, f.est, first.SeasonID, calendar.SlotsAM)

		// A Ferragosto day in row 1 with an extra sunbed and a cabin.
		cat, rs := engineData(t, db, f.est, first.SeasonID)
		ids := map[string]string{}
		for _, p := range cat.Packs {
			ids["pack:"+p.Code] = p.ID
		}
		for _, x := range cat.Extras {
			ids["extra:"+x.Code] = x.ID
		}
		for _, s := range cat.Spots {
			ids["spot:"+s.Label] = s.ID
		}
		d := calendar.MustParse("2026-08-15")
		q, err := pricing.Quote(pricing.QuoteInput{
			Channel: pricing.ChannelDesk, StartsOn: d, EndsOn: d, Slots: calendar.SlotsFull, Formula: pricing.FormulaDay, Today: d,
			Lines:  []pricing.LineInput{{PackID: ids["pack:base"], SpotID: ids["spot:A7"], Extras: []pricing.ExtraInput{{ExtraID: ids["extra:extra_sunbed"], Quantity: 1}}}},
			Extras: []pricing.ExtraInput{{ExtraID: ids["extra:cabin"], Quantity: 1}},
		}, cat, rs)
		if err != nil || q.Total != 4500+600+1000 {
			t.Fatalf("quote: %d %v", q.Total, err)
		}
		// Bookable all year: January and December are low season.
		for _, day := range []string{"2026-01-15", "2026-12-20"} {
			d := calendar.MustParse(day)
			q, err := pricing.Quote(pricing.QuoteInput{
				Channel: pricing.ChannelDesk, StartsOn: d, EndsOn: d, Slots: calendar.SlotsFull, Formula: pricing.FormulaDay, Today: d,
				Lines: []pricing.LineInput{{PackID: ids["pack:base"], SpotID: ids["spot:A7"]}},
			}, cat, rs)
			if err != nil || q.Total != money.Cents(DemoDayPrices[CategoryRow1][PeriodLow]) {
				t.Fatalf("%s quote: %d %v", day, q.Total, err)
			}
		}
		// A week in August (fixed price, Saturday to Saturday).
		w := calendar.MustParse("2026-08-01")
		q, err = pricing.Quote(pricing.QuoteInput{
			Channel: pricing.ChannelOnline, StartsOn: w, EndsOn: w.AddDays(6), Slots: calendar.SlotsFull, Formula: pricing.FormulaWeek,
			Today: w.AddDays(-30), Lines: []pricing.LineInput{{PackID: ids["pack:base"], SpotID: ids["spot:B3"]}},
		}, cat, rs)
		if err != nil || q.Total != 24000 {
			t.Fatalf("week quote: %d %v", q.Total, err)
		}

		// Policies: online in August resolves to Saturday-to-Saturday weeks.
		var rows []store.BookingPolicy
		if err := db.NewSelect().Model(&rows).Where("bp.establishment_id = ?", f.est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		var pols []policy.Policy
		for _, r := range rows {
			p := policy.Policy{ID: r.ID, Priority: r.Priority}
			if err := r.Scope.Unmarshal(&p.Scope); err != nil {
				t.Fatal(err)
			}
			if err := r.Rules.Unmarshal(&p.Rules); err != nil {
				t.Fatal(err)
			}
			pols = append(pols, p)
		}
		res := policy.Resolve(pols, policy.DefaultRules(), policy.Target{Channel: policy.ChannelOnline, Packs: []string{ids["pack:base"], PackBase}, RatePeriods: []string{PeriodPeak}})
		if res.Rules.Duration == nil || res.Rules.Duration.FixedStartWeekday == nil || *res.Rules.Duration.FixedStartWeekday != 6 {
			t.Fatalf("august online: %+v", res)
		}
		res = policy.Resolve(pols, policy.DefaultRules(), policy.Target{Channel: policy.ChannelDesk, Packs: []string{PackBase}, RatePeriods: []string{PeriodLow}})
		if res.Rules.EffectivePaymentMode() != policy.PaymentNone {
			t.Fatalf("desk: %+v", res.Rules.Payment)
		}
		res = policy.Resolve(pols, policy.DefaultRules(), policy.Target{Channel: policy.ChannelOnline, Packs: []string{PackBase}, RatePeriods: []string{PeriodMid}})
		if res.Rules.EffectivePaymentMode() != policy.PaymentFull {
			t.Fatalf("online: %+v", res.Rules.Payment)
		}
		res = policy.Resolve(pols, policy.DefaultRules(), policy.Target{Channel: policy.ChannelOnline, Packs: []string{PackGazebo}, RatePeriods: []string{PeriodMid}})
		if !res.Rules.RequiresApproval() {
			t.Fatal("gazebo requires approval")
		}
	})
}

func ptr[T any](v T) *T { return &v }

// A demo seeded before the map kit (spots without asset, the four plain
// template decorations) gets the kit on the next seed, once; decorations
// added by hand are kept, and the change is audited.
func TestSeedDemo_UpgradesPreKitMaps(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db, "lido-demo")
		if _, err := f.svc.SeedDemo(ctx, f.est.ID); err != nil {
			t.Fatal(err)
		}
		var m store.Map
		if err := db.NewSelect().Model(&m).Where("m.establishment_id = ?", f.est.ID).Limit(1).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		// Back to the pre-kit state: the template bar, plus a hand-made sign.
		if _, err := db.NewUpdate().TableExpr("spots").Set("asset = NULL").Where("establishment_id = ?", f.est.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewDelete().TableExpr("map_decorations").Where("establishment_id = ?", f.est.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		old := store.MapDecoration{
			ID: store.NewID(), EstablishmentID: f.est.ID, MapID: m.ID, Kind: "bar", X: float64(m.Width) - 40, Y: 40,
			Shape: store.Shape{Type: "rect", W: 40, H: 30}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		mine := store.MapDecoration{
			ID: store.NewID(), EstablishmentID: f.est.ID, MapID: m.ID, Kind: "text", X: 300, Y: 300,
			Shape: store.Shape{Type: "rect", W: 80, H: 20}, Label: store.I18nText{"it": "Mio"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		for _, d := range []*store.MapDecoration{&old, &mine} {
			if _, err := db.NewInsert().Model(d).Exec(ctx); err != nil {
				t.Fatal(err)
			}
		}
		up, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil || up.Applied || up.MapsUpgraded != 1 || up.DecorationsRemoved != 1 || up.DecorationsAdded < 5 || up.SpotsIllustrated == 0 {
			t.Fatalf("upgrade: %+v %v", up, err)
		}
		var spots []store.Spot
		if err := db.NewSelect().Model(&spots).Where("sp.establishment_id = ?", f.est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, sp := range spots {
			if sp.Asset == nil || !mapping.IsKitBookableAsset(*sp.Asset) {
				t.Fatalf("spot %s without asset after upgrade", sp.Label)
			}
		}
		var decos []store.MapDecoration
		if err := db.NewSelect().Model(&decos).Where("md.establishment_id = ?", f.est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		kept := false
		for _, d := range decos {
			if d.ID == old.ID {
				t.Fatalf("pre-kit template decoration left: %+v", d)
			}
			if d.ID == mine.ID {
				kept = true
				continue
			}
			if d.Asset == nil {
				t.Fatalf("decoration without asset: %+v", d)
			}
		}
		if !kept || len(decos) < 6 {
			t.Fatalf("hand-made decoration kept=%v, %d decorations", kept, len(decos))
		}
		if n, err := db.NewSelect().Model((*store.AuditEntry)(nil)).
			Where("establishment_id = ? AND action = ? AND entity_id = ?", f.est.ID, "setup.demo_map_kit_upgraded", m.ID).Count(ctx); err != nil || n != 1 {
			t.Fatalf("audit entries: %d %v", n, err)
		}
		var after store.Map
		if err := db.NewSelect().Model(&after).Where("m.id = ?", m.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if after.Version != m.Version+1 {
			t.Fatalf("version %d -> %d", m.Version, after.Version)
		}
		again, err := f.svc.SeedDemo(ctx, f.est.ID)
		if err != nil || again.MapsUpgraded != 0 {
			t.Fatalf("again: %+v %v", again, err)
		}
	})
}
