//go:build integration

package pricingconfig

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

func dayQuote(f *fixture, packID, zoneID string, date calendar.Date, ch pricing.Channel) SimulateInput {
	return SimulateInput{
		SeasonID: f.season.ID, Channel: ch, StartsOn: date, EndsOn: date, Slots: calendar.SlotsFull,
		Formula: pricing.FormulaDay, Today: &date, Lines: []pricing.LineInput{{PackID: packID, ZoneID: zoneID}},
	}
}

// TestScenario_SimulateMatrixConsistency builds a full price list and checks
// that the matrix equals the expected numbers and that every matrix cell
// equals the simulator's day price for the same input.
func TestScenario_SimulateMatrixConsistency(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()

		m, err := f.svc.Matrix(f.ctx, f.manager, f.est.ID, MatrixInput{SeasonID: f.season.ID, PackIDs: []string{f.base.ID, f.premium.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if got := m.Categories; len(got) != 2 || got[0] != "front" || got[1] != "back" {
			t.Fatalf("categories %v", got)
		}
		if got := m.RatePeriods; len(got) != 3 || got[0] != "low" || got[2] != "high" {
			t.Fatalf("rate periods %v", got)
		}
		want := map[[3]string]money.Cents{
			{f.base.ID, "front", "low"}: 2500, {f.base.ID, "back", "low"}: 2000,
			{f.base.ID, "front", "mid"}: 3000, {f.base.ID, "back", "mid"}: 2500,
			{f.base.ID, "front", "high"}: 4000, {f.base.ID, "back", "high"}: 3500,
			{f.premium.ID, "front", "low"}: 5500, {f.premium.ID, "back", "low"}: 5000,
			{f.premium.ID, "front", "mid"}: 5500, {f.premium.ID, "back", "mid"}: 5000,
			{f.premium.ID, "front", "high"}: 5500, {f.premium.ID, "back", "high"}: 5000,
		}
		if len(m.Cells) != len(want) {
			t.Fatalf("cells: got %d want %d", len(m.Cells), len(want))
		}
		for _, c := range m.Cells {
			w, ok := want[[3]string{c.PackID, c.PriceCategory, c.RatePeriod}]
			if !ok || !c.Priced || c.Price != w {
				t.Errorf("cell %s/%s/%s: priced=%v price=%d want %d", c.PackID, c.PriceCategory, c.RatePeriod, c.Priced, c.Price, w)
			}
			// simulate == matrix
			res, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, dayQuote(f, c.PackID, c.ZoneID, c.Date, pricing.ChannelDesk))
			if err != nil {
				t.Fatalf("simulate %v: %v", c, err)
			}
			if got := res.Breakdown.Lines[0].Days[0].Total; got != c.Price {
				t.Errorf("simulate %s on %s: day total %d, matrix %d", c.PriceCategory, c.Date, got, c.Price)
			}
			if res.Breakdown.Total != c.Price {
				t.Errorf("simulate total %d, matrix %d", res.Breakdown.Total, c.Price)
			}
		}
		// The representative dates are the first weekday of each period.
		for _, c := range m.Cells {
			if c.RatePeriod == "low" && c.Date != d("2027-05-17") || c.RatePeriod == "high" && c.Date != d("2027-08-02") {
				t.Errorf("representative date of %s: %s", c.RatePeriod, c.Date)
			}
		}
	})
}

func TestSimulate_StepsDiscountsExtrasAndPolicy(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()

		// Saturday in high season, front row: 3500 + 500 + 300, plus a sunbed.
		sat := d("2027-08-14")
		in := dayQuote(f, f.base.ID, f.front.ID, sat, pricing.ChannelOnline)
		in.Lines[0].Extras = []pricing.ExtraInput{{ExtraID: f.sunbed.ID, Quantity: 1}}
		res, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		day := res.Breakdown.Lines[0].Days[0]
		if day.Base != 3500 || day.Total != 4300 || len(day.Applied) != 3 {
			t.Fatalf("day %+v", day)
		}
		if res.Breakdown.Total != 5100 {
			t.Fatalf("total %d, want 5100", res.Breakdown.Total)
		}
		if res.Policy.PolicyID != "" || res.DepositDue != 5100 || len(res.Violations) != 0 {
			t.Fatalf("default policy: %+v deposit %d violations %v", res.Policy, res.DepositDue, res.Violations)
		}

		// Multi-day week in July, back row: 5 x 2500 + 2 x 2800 = 18100, -10%.
		week := SimulateInput{
			SeasonID: f.season.ID, Channel: pricing.ChannelDesk, StartsOn: d("2027-07-05"), EndsOn: d("2027-07-11"),
			Slots: calendar.SlotsFull, Formula: pricing.FormulaMultiDay,
			Lines: []pricing.LineInput{{PackID: f.base.ID, ZoneID: f.back.ID}},
		}
		res, err = f.svc.Simulate(f.ctx, f.manager, f.est.ID, week)
		if err != nil {
			t.Fatal(err)
		}
		if res.Breakdown.Lines[0].Total != 18100 || res.Breakdown.Total != 16290 {
			t.Fatalf("week: line %d total %d", res.Breakdown.Lines[0].Total, res.Breakdown.Total)
		}
		// today defaults to the clock (2027-03-01): 126 days of lead > 120.
		if len(res.Violations) != 1 || res.Violations[0].Code != policy.CodeLeadTimeTooLong {
			t.Fatalf("violations %v", res.Violations)
		}

		// A policy scoped to the high period requires a 30% deposit.
		pol, err := f.pol.Create(f.ctx, f.manager, f.est.ID, PolicyInput{
			SeasonID: f.season.ID, Name: "Agosto acconto", Priority: 10, Active: true,
			Scope: policy.Scope{RatePeriods: []string{"high"}},
			Rules: policy.Rules{Payment: &policy.Payment{
				Mode:    policy.Ptr(policy.PaymentDeposit),
				Deposit: &policy.DepositRule{Type: policy.Ptr(policy.DepositPercent), Value: policy.Ptr(int64(30))},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err = f.svc.Simulate(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if res.Policy.PolicyID != pol.ID || res.DepositDue != 1530 {
			t.Fatalf("policy %s deposit %d", res.Policy.PolicyID, res.DepositDue)
		}
		// Inherited fields come from the establishment default policy.
		if res.Policy.Rules.MaxLeadTimeDays == nil || *res.Policy.Rules.MaxLeadTimeDays != 120 {
			t.Fatalf("inherited rules %+v", res.Policy.Rules)
		}

		// Fixed week price in August, front row.
		code := "high"
		if _, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, FixedPriceInput{
			SeasonID: f.season.ID, PackID: f.base.ID, Formula: pricing.FormulaWeek, PriceCategory: "front",
			RatePeriodCode: &code, Amount: 21000,
		}); err != nil {
			t.Fatal(err)
		}
		today := d("2027-07-20")
		res, err = f.svc.Simulate(f.ctx, f.manager, f.est.ID, SimulateInput{
			Channel: pricing.ChannelDesk, StartsOn: d("2027-08-07"), EndsOn: d("2027-08-13"), Slots: calendar.SlotsFull,
			Formula: pricing.FormulaWeek, Today: &today, Lines: []pricing.LineInput{{PackID: f.base.ID, ZoneID: f.front.ID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if fx := res.Breakdown.Lines[0].Fixed; fx == nil || fx.Amount != 21000 || res.Breakdown.Total != 21000 {
			t.Fatalf("fixed week: %+v total %d", fx, res.Breakdown.Total)
		}
		if res.SeasonID != f.season.ID {
			t.Fatalf("season resolved from starts_on: %s", res.SeasonID)
		}

		// No base price: 422 no_price.
		_, err = f.svc.Simulate(f.ctx, f.manager, f.est.ID, dayQuote(f, f.kayak.ID, f.back.ID, sat, pricing.ChannelDesk))
		if !errors.Is(err, pricing.ErrNoPrice) {
			t.Fatalf("kayak: %v", err)
		}
		// Outside every season.
		_, err = f.svc.Simulate(f.ctx, f.manager, f.est.ID, SimulateInput{
			Channel: pricing.ChannelDesk, StartsOn: d("2027-12-01"), EndsOn: d("2027-12-01"), Slots: calendar.SlotsFull,
			Formula: pricing.FormulaDay, Lines: []pricing.LineInput{{PackID: f.base.ID, ZoneID: f.back.ID}},
		})
		if !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("no season: %v", err)
		}
	})
}

func TestPromoCode_CaseInsensitive(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		in := PromoCodeInput{Code: "ESTATE10", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")}, Active: true}
		pc, err := f.svc.CreatePromoCode(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		in.Code = "estate10"
		if _, err := f.svc.CreatePromoCode(f.ctx, f.manager, f.est.ID, in); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate ignoring case: %v", err)
		}
		// The same code in another establishment is fine.
		otherOwner := f.outsider
		if _, err := f.svc.CreatePromoCode(f.ctx, otherOwner, f.other.ID, in); err != nil {
			t.Fatalf("other establishment: %v", err)
		}
		// Renaming to its own code with another case is allowed.
		in.Code = "Estate10"
		upd, err := f.svc.UpdatePromoCode(f.ctx, f.manager, f.est.ID, pc.ID, &pc.Version, in)
		if err != nil || upd.Code != "Estate10" || upd.Version != 2 {
			t.Fatalf("update: %+v %v", upd, err)
		}
		// The simulator finds it with any case and applies it.
		q := dayQuote(f, f.base.ID, f.back.ID, d("2027-07-05"), pricing.ChannelOnline)
		q.PromoCode = "eStAtE10"
		res, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if res.Breakdown.Total != 2250 || len(res.Breakdown.Discounts) != 1 {
			t.Fatalf("promo applied: total %d discounts %+v", res.Breakdown.Total, res.Breakdown.Discounts)
		}
		q.PromoCode = "NOPE"
		if _, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, q); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("unknown promo: %v", err)
		}
		// Validity window and limits are validated.
		bad := PromoCodeInput{
			Code: "X1", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")},
			ValidFrom: ptrDate(d("2027-08-01")), ValidTo: ptrDate(d("2027-07-01")), MaxUses: intp(0),
		}
		var ve *configerr.ValidationError
		if _, err := f.svc.CreatePromoCode(f.ctx, f.manager, f.est.ID, bad); !errors.As(err, &ve) || len(ve.Fields) != 2 {
			t.Fatalf("invalid promo: %v", err)
		}
		// Expired code: not applicable in the simulator.
		exp := PromoCodeInput{
			Code: "OLD", Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("5")},
			ValidTo: ptrDate(d("2027-01-31")), Active: true,
		}
		if _, err := f.svc.CreatePromoCode(f.ctx, f.manager, f.est.ID, exp); err != nil {
			t.Fatal(err)
		}
		q.PromoCode = "old"
		if _, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, q); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("expired promo: %v", err)
		}
	})
}

func ptrDate(v calendar.Date) *calendar.Date { return &v }

func TestPermissions_Spec16(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		in := setRule("x", 100, pricing.Conditions{}, f.base.ID)
		in.SeasonID = f.season.ID
		for _, p := range []*auth.Principal{f.cashier, f.viewer} {
			if _, err := f.svc.CreateRule(f.ctx, p, f.est.ID, in); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s create rule: %v", p.Grants[0].Role, err)
			}
			if _, err := f.svc.Matrix(f.ctx, p, f.est.ID, MatrixInput{SeasonID: f.season.ID}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s matrix: %v", p.Grants[0].Role, err)
			}
			if _, err := f.svc.Simulate(f.ctx, p, f.est.ID, dayQuote(f, f.base.ID, f.back.ID, d("2027-07-05"), pricing.ChannelDesk)); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s simulate: %v", p.Grants[0].Role, err)
			}
			if _, err := f.pol.Create(f.ctx, p, f.est.ID, PolicyInput{SeasonID: f.season.ID, Name: "x"}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s create policy: %v", p.Grants[0].Role, err)
			}
			// View permission: everyone reads the price list.
			if items, _, err := f.svc.ListRules(f.ctx, p, f.est.ID, RuleListFilter{SeasonID: f.season.ID}, "", 50); err != nil || len(items) != 8 {
				t.Fatalf("%s list rules: %d %v", p.Grants[0].Role, len(items), err)
			}
		}
		for _, p := range []*auth.Principal{f.owner, f.manager} {
			if _, err := f.svc.CreateRule(f.ctx, p, f.est.ID, in); err != nil {
				t.Fatalf("%s create rule: %v", p.Grants[0].Role, err)
			}
		}
		if _, _, err := f.svc.ListRules(f.ctx, nil, f.est.ID, RuleListFilter{}, "", 10); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
	})
}

func TestCrossTenant_Isolation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		rules, _, err := f.svc.ListRules(f.ctx, f.owner, f.est.ID, RuleListFilter{}, "", 1)
		if err != nil || len(rules) != 1 {
			t.Fatal(err)
		}
		// The outsider cannot reach establishment A at all.
		if _, _, err := f.svc.ListRules(f.ctx, f.outsider, f.est.ID, RuleListFilter{}, "", 10); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("outsider list: %v", err)
		}
		if _, err := f.svc.Matrix(f.ctx, f.outsider, f.est.ID, MatrixInput{SeasonID: f.season.ID}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("outsider matrix: %v", err)
		}
		// Through its own establishment, A's IDs are unknown.
		if _, err := f.svc.GetRule(f.ctx, f.outsider, f.other.ID, rules[0].ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("outsider get: %v", err)
		}
		if err := f.svc.DeleteRule(f.ctx, f.outsider, f.other.ID, rules[0].ID, nil); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("outsider delete: %v", err)
		}
		in := setRule("x", 100, pricing.Conditions{})
		in.SeasonID = f.season.ID // season of A
		if _, err := f.svc.CreateRule(f.ctx, f.outsider, f.other.ID, in); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign season: %v", err)
		}
		in = setRule("x", 100, pricing.Conditions{}, f.base.ID) // pack of A
		otherSeason := &store.Season{
			ID: store.NewID(), EstablishmentID: f.other.ID, Year: 2027, OpensOn: d("2027-06-01"),
			ClosesOn: d("2027-09-01"), Status: "draft", Version: 1, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
		}
		f.insert(otherSeason)
		in.SeasonID = otherSeason.ID
		if _, err := f.svc.CreateRule(f.ctx, f.outsider, f.other.ID, in); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign pack: %v", err)
		}
		if _, err := f.svc.Simulate(f.ctx, f.outsider, f.other.ID, SimulateInput{
			SeasonID: otherSeason.ID, Channel: pricing.ChannelDesk, StartsOn: d("2027-07-05"), EndsOn: d("2027-07-05"),
			Slots: calendar.SlotsFull, Formula: pricing.FormulaDay, Lines: []pricing.LineInput{{PackID: f.base.ID, ZoneID: f.back.ID}},
		}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("foreign catalog in simulate: %v", err)
		}
	})
}

func TestPriceRule_VersionSeqAuditOutbox(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		in := setRule("base", 2000, pricing.Conditions{RatePeriods: []string{"low"}}, f.base.ID)
		in.SeasonID = f.season.ID
		r1, err := f.svc.CreateRule(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		r2, err := f.svc.CreateRule(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if r1.Seq != 1 || r2.Seq != 2 || r1.Version != 1 {
			t.Fatalf("seq %d %d version %d", r1.Seq, r2.Seq, r1.Version)
		}
		in.Name = "base v2"
		upd, err := f.svc.UpdateRule(f.ctx, f.manager, f.est.ID, r1.ID, &r1.Version, in)
		if err != nil {
			t.Fatal(err)
		}
		if upd.Seq != 3 || upd.Version != 2 || upd.Name != "base v2" {
			t.Fatalf("updated %+v", upd)
		}
		var vc *configerr.VersionConflictError
		if _, err := f.svc.UpdateRule(f.ctx, f.manager, f.est.ID, r1.ID, &r1.Version, in); !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale update: %v", err)
		}
		if err := f.svc.DeleteRule(f.ctx, f.manager, f.est.ID, r1.ID, &r1.Version); !errors.As(err, &vc) {
			t.Fatalf("stale delete: %v", err)
		}
		if err := f.svc.DeleteRule(f.ctx, f.manager, f.est.ID, r1.ID, &upd.Version); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.GetRule(f.ctx, f.manager, f.est.ID, r1.ID); !errors.Is(err, configerr.ErrNotFound) {
			t.Fatalf("deleted: %v", err)
		}
		if n := f.countEvents(event.ConfigPricingChanged); n != 4 {
			t.Fatalf("pricing_changed events: %d", n)
		}
		for action, n := range map[string]int{"price_rule.created": 2, "price_rule.updated": 1, "price_rule.deleted": 1} {
			if got := f.countAudit(action); got != n {
				t.Fatalf("audit %s: %d want %d", action, got, n)
			}
		}
		// Engine validation surfaces as field errors and writes nothing.
		bad := in
		bad.Stage = pricing.StageBase
		bad.Action = pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("10")}
		var ve *configerr.ValidationError
		if _, err := f.svc.CreateRule(f.ctx, f.manager, f.est.ID, bad); !errors.As(err, &ve) || ve.Fields[0].Field != "action.type" {
			t.Fatalf("invalid action: %v", err)
		}
		if n := f.countEvents(event.ConfigPricingChanged); n != 4 {
			t.Fatalf("events after failed create: %d", n)
		}
		// Pagination.
		for i := 0; i < 3; i++ {
			if _, err := f.svc.CreateRule(f.ctx, f.manager, f.est.ID, in); err != nil {
				t.Fatal(err)
			}
		}
		page1, next, err := f.svc.ListRules(f.ctx, f.viewer, f.est.ID, RuleListFilter{SeasonID: f.season.ID}, "", 2)
		if err != nil || len(page1) != 2 || next == "" {
			t.Fatalf("page1 %d %q %v", len(page1), next, err)
		}
		page2, next, err := f.svc.ListRules(f.ctx, f.viewer, f.est.ID, RuleListFilter{SeasonID: f.season.ID}, next, 2)
		if err != nil || len(page2) != 2 || next != "" || page2[0].ID == page1[1].ID {
			t.Fatalf("page2 %d %q %v", len(page2), next, err)
		}
	})
}

func TestFixedPricesAndAgreements_CRUD(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		code := "high"
		in := FixedPriceInput{SeasonID: f.season.ID, PackID: f.base.ID, Formula: pricing.FormulaWeek, PriceCategory: "front", RatePeriodCode: &code, Amount: 21000}
		fp, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, in); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate fixed: %v", err)
		}
		bad := in
		bad.Formula = pricing.FormulaDay
		missing := "nope"
		bad.RatePeriodCode = &missing
		if _, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, bad); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("bad fixed: %v", err)
		}
		in.Amount = 22000
		up, err := f.svc.UpdateFixedPrice(f.ctx, f.manager, f.est.ID, fp.ID, &fp.Version, in)
		if err != nil || up.Amount != 22000 || up.Version != 2 {
			t.Fatalf("update fixed: %+v %v", up, err)
		}
		items, _, err := f.svc.ListFixedPrices(f.ctx, f.cashier, f.est.ID, FixedPriceListFilter{PackID: f.base.ID}, "", 10)
		if err != nil || len(items) != 1 {
			t.Fatalf("list fixed: %d %v", len(items), err)
		}

		// Agreement: 15% off for hotel guests; usable in the simulator.
		ac := "HOTEL"
		ag, err := f.svc.CreateAgreement(f.ctx, f.manager, f.est.ID, AgreementInput{
			Code: &ac, Name: "Hotel Miramare", PartnerName: "Miramare srl", Active: true,
			Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("15")},
		})
		if err != nil {
			t.Fatal(err)
		}
		lower := "hotel"
		if _, err := f.svc.CreateAgreement(f.ctx, f.manager, f.est.ID, AgreementInput{
			Code: &lower, Name: "dup",
			Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("5")},
		}); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("duplicate agreement code: %v", err)
		}
		q := dayQuote(f, f.base.ID, f.back.ID, d("2027-07-05"), pricing.ChannelDesk)
		q.AgreementID = ag.ID
		res, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, q)
		if err != nil {
			t.Fatal(err)
		}
		if res.Breakdown.Total != 2125 {
			t.Fatalf("agreement total %d", res.Breakdown.Total)
		}
		inactive := AgreementInput{
			Code: &ac, Name: "Hotel Miramare", Active: false,
			Action: pricing.Action{Type: pricing.ActionPercentOff, Percent: decp("15")},
		}
		if _, err := f.svc.UpdateAgreement(f.ctx, f.manager, f.est.ID, ag.ID, nil, inactive); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Simulate(f.ctx, f.manager, f.est.ID, q); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("inactive agreement: %v", err)
		}
		if err := f.svc.DeleteAgreement(f.ctx, f.manager, f.est.ID, ag.ID, nil); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.DeleteFixedPrice(f.ctx, f.manager, f.est.ID, fp.ID, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPolicies_CRUDAndValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		in := PolicyInput{
			SeasonID: f.season.ID, Name: "Agosto solo settimane", Priority: 5, Active: true,
			Scope: policy.Scope{RatePeriods: []string{"high"}, Channels: []policy.Channel{policy.ChannelOnline}, Packs: []string{"base"}},
			Rules: policy.Rules{Duration: &policy.Duration{AllowedFormulas: []policy.Formula{policy.FormulaWeek}, FixedStartWeekday: policy.Ptr(6)}},
		}
		p, err := f.pol.Create(f.ctx, f.manager, f.est.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.pol.Get(f.ctx, f.viewer, f.est.ID, p.ID)
		if err != nil || *got.Rules.Duration.FixedStartWeekday != 6 || got.Scope.Packs[0] != "base" {
			t.Fatalf("get: %+v %v", got, err)
		}
		bad := in
		bad.Scope = policy.Scope{Zones: []string{"nowhere"}, RatePeriods: []string{"winter"}}
		bad.Rules = policy.Rules{MaxSpotsPerBooking: policy.Ptr(0), Payment: &policy.Payment{Mode: policy.Ptr(policy.PaymentDeposit)}}
		var ve *configerr.ValidationError
		if _, err := f.pol.Create(f.ctx, f.manager, f.est.ID, bad); !errors.As(err, &ve) || len(ve.Fields) != 2 {
			t.Fatalf("invalid rules: %v", err)
		}
		bad.Rules = policy.Rules{}
		if _, err := f.pol.Create(f.ctx, f.manager, f.est.ID, bad); !errors.As(err, &ve) || len(ve.Fields) != 2 {
			t.Fatalf("invalid scope: %v", err)
		}
		in.Priority = 7
		up, err := f.pol.Update(f.ctx, f.manager, f.est.ID, p.ID, &p.Version, in)
		if err != nil || up.Priority != 7 || up.Version != 2 {
			t.Fatalf("update: %+v %v", up, err)
		}
		list, _, err := f.pol.List(f.ctx, f.cashier, f.est.ID, f.season.ID, "", 10)
		if err != nil || len(list) != 1 {
			t.Fatalf("list: %d %v", len(list), err)
		}
		if err := f.pol.Delete(f.ctx, f.manager, f.est.ID, p.ID, &p.Version); !errors.Is(err, configerr.ErrVersionConflict) {
			t.Fatalf("stale delete: %v", err)
		}
		if err := f.pol.Delete(f.ctx, f.manager, f.est.ID, p.ID, &up.Version); err != nil {
			t.Fatal(err)
		}
		if n := f.countAudit("booking_policy.created"); n != 1 {
			t.Fatalf("audit: %d", n)
		}
		if n := f.countEvents(event.ConfigPricingChanged); n != 3 {
			t.Fatalf("events: %d", n)
		}
	})
}

func TestCoverage_GapsAndClosures(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		res, err := f.svc.Coverage(f.ctx, f.manager, f.est.ID, CoverageInput{SeasonID: f.season.ID, PackIDs: []string{f.base.ID, f.premium.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Complete() || res.From != f.season.OpensOn || res.To != f.season.ClosesOn {
			t.Fatalf("base/premium coverage: %+v", res.Gaps)
		}
		f.insert(&store.ClosureDay{
			ID: store.NewID(), EstablishmentID: f.est.ID, Date: d("2027-06-02"),
			Slots: []calendar.Slot{calendar.AM, calendar.PM}, Reason: "festa", Version: 1, CreatedAt: f.clock.Now(), UpdatedAt: f.clock.Now(),
		})
		res, err = f.svc.Coverage(f.ctx, f.manager, f.est.ID, CoverageInput{SeasonID: f.season.ID})
		if err != nil {
			t.Fatal(err)
		}
		// kayak has no price: 2 categories x 2 channels, each split by the closure.
		if len(res.Gaps) != 8 {
			t.Fatalf("gaps: %+v", res.Gaps)
		}
		for _, g := range res.Gaps {
			if g.PackID != f.kayak.ID {
				t.Fatalf("unexpected gap %+v", g)
			}
		}
		if g := res.Gaps[0]; g.From != d("2027-05-15") || g.To != d("2027-06-01") || res.Gaps[1].From != d("2027-06-03") || res.Gaps[1].To != d("2027-09-15") {
			t.Fatalf("split gaps: %+v %+v", res.Gaps[0], res.Gaps[1])
		}
		// Removing the mid base rule opens a July gap for base.
		rules, _, err := f.svc.ListRules(f.ctx, f.manager, f.est.ID, RuleListFilter{SeasonID: f.season.ID}, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rules {
			if r.Name == "base mid" {
				if err := f.svc.DeleteRule(f.ctx, f.manager, f.est.ID, r.ID, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		res, err = f.svc.Coverage(f.ctx, f.manager, f.est.ID, CoverageInput{
			SeasonID: f.season.ID, PackIDs: []string{f.base.ID},
			Channels: []pricing.Channel{pricing.ChannelOnline},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Gaps) != 2 || res.Gaps[0].From != d("2027-07-01") || res.Gaps[0].To != d("2027-07-31") {
			t.Fatalf("july gaps: %+v", res.Gaps)
		}
	})
}

func TestCopySeason_ToNextYear(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.priceList()
		code := "high"
		if _, err := f.svc.CreateFixedPrice(f.ctx, f.manager, f.est.ID, FixedPriceInput{
			SeasonID: f.season.ID, PackID: f.base.ID,
			Formula: pricing.FormulaWeek, PriceCategory: "front", RatePeriodCode: &code, Amount: 21000,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pol.Create(f.ctx, f.manager, f.est.ID, PolicyInput{
			SeasonID: f.season.ID, Name: "acconto", Active: true,
			Scope: policy.Scope{RatePeriods: []string{"high"}}, Rules: policy.Rules{ApprovalRequired: policy.Ptr(true)},
		}); err != nil {
			t.Fatal(err)
		}
		all := CopySeasonInput{TargetYear: 2028, IncludeRatePeriods: true, IncludePriceRules: true, IncludeFixedPrices: true, IncludePolicies: true}
		if _, err := f.svc.CopySeason(f.ctx, f.cashier, f.est.ID, f.season.ID, all); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier copy: %v", err)
		}
		res, err := f.svc.CopySeason(f.ctx, f.manager, f.est.ID, f.season.ID, all)
		if err != nil {
			t.Fatal(err)
		}
		if !res.SeasonCreated || res.Season.Year != 2028 || res.RatePeriods != 3 || res.PriceRules != 8 || res.FixedPrices != 1 || res.Policies != 1 {
			t.Fatalf("copy result %+v", res)
		}
		// 2027-05-15 is a Saturday: the copy opens on Saturday 2028-05-13.
		if res.Season.OpensOn != d("2028-05-13") || res.Season.Status != "draft" {
			t.Fatalf("target season %+v", res.Season)
		}
		periods, err := store.PricingListRatePeriods(f.ctx, db, mustScope(t, f), res.Season.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range periods {
			if p.Code == "high" && (p.Ranges[0].From != d("2028-08-01") || p.Ranges[0].To != d("2028-08-31")) {
				t.Fatalf("high copied as %+v", p.Ranges)
			}
		}
		rules, _, err := f.svc.ListRules(f.ctx, f.manager, f.est.ID, RuleListFilter{SeasonID: res.Season.ID}, "", 50)
		if err != nil || len(rules) != 8 {
			t.Fatalf("copied rules %d %v", len(rules), err)
		}
		for _, r := range rules {
			if r.Seq <= 8 {
				t.Fatalf("copied rule %q keeps old seq %d", r.Name, r.Seq)
			}
		}
		// The copied price list prices the same matrix.
		m, err := f.svc.Matrix(f.ctx, f.manager, f.est.ID, MatrixInput{SeasonID: res.Season.ID, PackIDs: []string{f.base.ID}})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range m.Cells {
			if c.PriceCategory == "front" && c.RatePeriod == "high" && c.Price != 4000 {
				t.Fatalf("copied matrix cell %+v", c)
			}
		}
		// A second copy onto the same year conflicts.
		if _, err := f.svc.CopySeason(f.ctx, f.manager, f.est.ID, f.season.ID, all); !errors.Is(err, configerr.ErrConflict) {
			t.Fatalf("second copy: %v", err)
		}
		if n := f.countAudit("season_copy.copied"); n != 1 {
			t.Fatalf("copy audit %d", n)
		}
		if n := f.countAudit("season.created"); n != 1 {
			t.Fatalf("season audit %d", n)
		}
		// Copying fixed prices without periods into a season lacking them fails.
		only := CopySeasonInput{TargetYear: 2029, IncludeFixedPrices: true}
		if _, err := f.svc.CopySeason(f.ctx, f.manager, f.est.ID, f.season.ID, only); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("fixed without periods: %v", err)
		}
		if _, err := store.PricingSeasonByYear(f.ctx, db, mustScope(t, f), 2029); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("failed copy must roll back the season: %v", err)
		}
	})
}

func mustScope(t *testing.T, f *fixture) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.ctx, f.db, f.est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestReplaceRules_ForSetup(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		first, err := f.svc.ReplaceRules(f.ctx, f.manager, f.est.ID, f.season.ID, nil, []RuleInput{
			setRule("front low", 2500, pricing.Conditions{RatePeriods: []string{"low"}, PriceCategories: []string{"front"}}),
			setRule("back low", 2000, pricing.Conditions{RatePeriods: []string{"low"}, PriceCategories: []string{"back"}}),
		})
		if err != nil || len(first.Rules) != 2 || first.Replaced != 0 {
			t.Fatalf("first %+v %v", first, err)
		}
		second, err := f.svc.ReplaceRules(f.ctx, f.manager, f.est.ID, f.season.ID,
			[]string{first.Rules[0].ID, first.Rules[1].ID},
			[]RuleInput{setRule("all low", 2200, pricing.Conditions{RatePeriods: []string{"low"}})})
		if err != nil || second.Replaced != 2 || len(second.Rules) != 1 {
			t.Fatalf("second %+v %v", second, err)
		}
		var ve *configerr.ValidationError
		_, err = f.svc.ReplaceRules(f.ctx, f.manager, f.est.ID, f.season.ID, []string{second.Rules[0].ID},
			[]RuleInput{{Name: "", Stage: pricing.StageBase}})
		if !errors.As(err, &ve) || ve.Fields[0].Field != "rules[0].name" {
			t.Fatalf("invalid replace: %v", err)
		}
		// Rolled back: the rule to replace is still there.
		if _, err := f.svc.GetRule(f.ctx, f.manager, f.est.ID, second.Rules[0].ID); err != nil {
			t.Fatalf("rollback: %v", err)
		}
	})
}
