//go:build integration

package pricingconfig

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var d = calendar.MustParse

// fixture is a lido with a 2027 season, three rate periods, two zones with
// different price categories, three packs and an extra.
type fixture struct {
	t        *testing.T
	ctx      context.Context
	db       *store.DB
	clock    *clock.Fake
	svc      *Service
	pol      *PolicyService
	est      *store.Establishment
	other    *store.Establishment
	season   *store.Season
	front    *store.Zone // category "front"
	back     *store.Zone // category "back"
	base     *store.Pack
	premium  *store.Pack
	kayak    *store.Pack // active, never priced (coverage gaps)
	sunbed   *store.Extra
	owner    *auth.Principal
	manager  *auth.Principal
	cashier  *auth.Principal
	viewer   *auth.Principal
	outsider *auth.Principal // owner of the other organization
}

func principal(orgID string, role auth.Role) *auth.Principal {
	return &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgID, Role: role}}}
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	// 09:00 in Rome: before the default same-day cutoff of 13:00.
	c := clock.NewFake(time.Date(2027, 3, 1, 8, 0, 0, 0, time.UTC))
	deps := Deps{DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c)}
	f := &fixture{t: t, ctx: ctx, db: db, clock: c, svc: NewService(deps), pol: NewPolicyService(deps)}
	now := c.Now()
	org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-prezzi", "Lido Prezzi", now)
	if err != nil {
		t.Fatal(err)
	}
	est.Settings = store.JSON(`{"pricing":{"half_day_factor":"0.5","rounding":"none","default_vat_rate_bp":2200}}`)
	if _, err := db.NewUpdate().Model(est).WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	orgB, other, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-altro", "Lido Altro", now)
	if err != nil {
		t.Fatal(err)
	}
	f.est, f.other = est, other
	f.owner = principal(org.ID, auth.RoleOwner)
	f.manager = principal(org.ID, auth.RoleManager)
	f.cashier = principal(org.ID, auth.RoleCashier)
	f.viewer = principal(org.ID, auth.RoleViewer)
	f.outsider = principal(orgB.ID, auth.RoleOwner)

	f.season = &store.Season{
		ID: store.NewID(), EstablishmentID: est.ID, Year: 2027, OpensOn: d("2027-05-15"),
		ClosesOn: d("2027-09-15"), Status: "active", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.insert(f.season)
	period := func(code string, sort int, ranges ...store.DateRange) {
		f.insert(&store.RatePeriod{
			ID: store.NewID(), EstablishmentID: est.ID, SeasonID: f.season.ID, Code: code,
			Name: store.I18nText{"it": code}, Color: "#123456", Ranges: ranges, SortOrder: sort, Version: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	period("low", 1, store.DateRange{From: d("2027-05-15"), To: d("2027-06-30")}, store.DateRange{From: d("2027-09-01"), To: d("2027-09-15")})
	period("mid", 2, store.DateRange{From: d("2027-07-01"), To: d("2027-07-31")})
	period("high", 3, store.DateRange{From: d("2027-08-01"), To: d("2027-08-31")})
	zone := func(code, cat string, sort int) *store.Zone {
		z := &store.Zone{
			ID: store.NewID(), EstablishmentID: est.ID, Code: code, Name: store.I18nText{"it": code},
			AssignmentMode: "zone", OnlineVisible: true, PriceCategory: cat, SortOrder: sort, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		f.insert(z)
		return z
	}
	f.front = zone("row1", "front", 1)
	f.back = zone("row2", "back", 2)
	pack := func(code string, sort int) *store.Pack {
		p := &store.Pack{
			ID: store.NewID(), EstablishmentID: est.ID, Code: code, Name: store.I18nText{"it": code},
			Description: store.I18nText{}, Composition: map[string]int{"umbrella": 1}, AllowedFormulas: []string{"day"},
			Channels: []string{"desk", "online"}, MaxPerBooking: 4, SortOrder: sort, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		f.insert(p)
		return p
	}
	f.base = pack("base", 1)
	f.premium = pack("premium", 2)
	f.kayak = pack("kayak", 3)
	f.sunbed = &store.Extra{
		ID: store.NewID(), EstablishmentID: est.ID, Code: "sunbed", Name: store.I18nText{"it": "Lettino"},
		Description: store.I18nText{}, PricingUnit: "per_day", MaxQuantity: 3, AttachTo: "line", Channels: []string{"desk"},
		Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	f.insert(f.sunbed)
	return f
}

func (f *fixture) insert(m any) {
	f.t.Helper()
	if _, err := f.db.NewInsert().Model(m).Exec(f.ctx); err != nil {
		f.t.Fatalf("insert %T: %v", m, err)
	}
}

func setRule(name string, amount int64, cond pricing.Conditions, targets ...string) RuleInput {
	return RuleInput{
		Name: name, TargetType: pricing.TargetPack, TargetIDs: targets, Conditions: cond,
		Action: pricing.Action{Type: pricing.ActionSet, Amount: money.Cents(amount)}, Stage: pricing.StageBase, Active: true,
	}
}

// priceList creates the price list of the scenario (see TestScenario_*):
//
//	base:    low 2000, mid 2500, high 3500 (by rate period)
//	premium: 5000 every day
//	front category: +500 per day; weekend (Sat, Sun): +300 per day (both stackable)
//	multi_day of 7+ days: 10% off
func (f *fixture) priceList() {
	f.t.Helper()
	sid := f.season.ID
	for _, in := range []RuleInput{
		setRule("base low", 2000, pricing.Conditions{RatePeriods: []string{"low"}}, f.base.ID),
		setRule("base mid", 2500, pricing.Conditions{RatePeriods: []string{"mid"}}, f.base.ID),
		setRule("base high", 3500, pricing.Conditions{RatePeriods: []string{"high"}}, f.base.ID),
		setRule("premium", 5000, pricing.Conditions{}, f.premium.ID),
		{
			Name: "front", TargetType: pricing.TargetPack, Conditions: pricing.Conditions{PriceCategories: []string{"front"}},
			Action: pricing.Action{Type: pricing.ActionAdd, Amount: 500}, Stage: pricing.StageModifier, Stackable: true, Active: true,
		},
		{
			Name: "weekend", TargetType: pricing.TargetPack, Conditions: pricing.Conditions{Weekdays: []int{6, 7}},
			Action: pricing.Action{Type: pricing.ActionAdd, Amount: 300}, Stage: pricing.StageModifier, Stackable: true, Active: true,
		},
		{
			Name: "long stay", TargetType: pricing.TargetPack,
			Conditions: pricing.Conditions{MinDays: intp(7), Formulas: []pricing.Formula{pricing.FormulaMultiDay}},
			Action:     pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")}, Stage: pricing.StageDiscount, Active: true,
		},
		{
			Name: "sunbed", TargetType: pricing.TargetExtra, Action: pricing.Action{Type: pricing.ActionSet, Amount: 800},
			Stage: pricing.StageBase, Active: true,
		},
	} {
		in.SeasonID = sid
		if _, err := f.svc.CreateRule(f.ctx, f.manager, f.est.ID, in); err != nil {
			f.t.Fatalf("create rule %q: %v", in.Name, err)
		}
	}
}

func intp(v int) *int { return &v }

func decp(s string) *pricing.Decimal {
	v := pricing.MustParseDecimal(s)
	return &v
}

func (f *fixture) countEvents(eventType string) int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*store.OutboxEvent)(nil)).Where("type = ?", eventType).
		Where("establishment_id = ?", f.est.ID).Count(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return int(n)
}

func (f *fixture) countAudit(action string) int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*store.AuditEntry)(nil)).Where("action = ?", action).
		Where("establishment_id = ?", f.est.ID).Count(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return int(n)
}
