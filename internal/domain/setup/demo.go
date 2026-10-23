package setup

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// DemoResult reports what SeedDemo did.
type DemoResult struct {
	// Applied is false when the demo establishment was already configured
	// (it has maps): nothing changed.
	Applied      bool
	SeasonID     string
	SeasonYear   int
	SpotsCreated int
	RulesCreated int
	// MapsUpgraded counts the maps of an already configured demo that got
	// the map kit illustrations (docs/adr/0012-kit-mappa.md), with what
	// changed in them: spots given an illustration, pre-kit template
	// decorations removed and kit decorations added.
	MapsUpgraded       int
	SpotsIllustrated   int
	DecorationsRemoved int
	DecorationsAdded   int
	// MapsReoriented counts the template maps of an already configured demo
	// moved to the sea at the top (upgradeDemoSeaTop, spec 11 default).
	MapsReoriented int
}

// DemoDayPrices are the day prices of the demo price list (cents) by price
// category and rate period.
var DemoDayPrices = map[string]map[string]money.Cents{
	CategoryRow1:     {PeriodLow: 2500, PeriodMid: 3000, PeriodHigh: 3800, PeriodPeak: 4500},
	CategoryRow2:     {PeriodLow: 2200, PeriodMid: 2700, PeriodHigh: 3400, PeriodPeak: 4000},
	CategoryStandard: {PeriodLow: 1800, PeriodMid: 2200, PeriodHigh: 2800, PeriodPeak: 3300},
	CategoryGazebo:   {PeriodLow: 6000, PeriodMid: 7500, PeriodHigh: 9000, PeriodPeak: 11000},
}

// demoHalfDayFactor is the half-day factor of the demo quick price list.
var demoHalfDayFactor = pricing.NewDecimal(6, 1) // 0.6

// demoExtraPrices are the base prices of the demo extras (cents per unit).
var demoExtraPrices = map[string]money.Cents{"extra_sunbed": 600, "cabin": 1000, "towel": 300, "parking": 800}

// demoSeasonPrices are the season pass prices of the base pack by category.
var demoSeasonPrices = map[string]money.Cents{CategoryRow1: 210000, CategoryRow2: 185000, CategoryStandard: 150000}

// demoWeekPeakPrices are the August week prices of the base pack by category.
var demoWeekPeakPrices = map[string]money.Cents{CategoryRow1: 27000, CategoryRow2: 24000, CategoryStandard: 20000}

// policyAugustWeeks is the spec 9 example: in August, online only weeks from
// Saturday to Saturday.
var policyAugustWeeks = PolicySpec{
	Name:  "Agosto online: settimane da sabato a sabato",
	Scope: policy.Scope{Channels: []policy.Channel{policy.ChannelOnline}, RatePeriods: []string{PeriodPeak}},
	Rules: policy.Rules{
		Duration: &policy.Duration{AllowedFormulas: []policy.Formula{policy.FormulaWeek}, FixedStartWeekday: policy.Ptr(6)},
		Payment:  &policy.Payment{Mode: policy.Ptr(policy.PaymentFull)},
	},
	Priority: 30,
}

// SeedDemo configures the demo establishment (spec 27.10): the classic
// template (8 rows x 15 spots, rows A-H, gazebos) for the current or next
// season, activated, with the full quick price list of every category and
// period (half days at 0.6), extra prices, fixed week/season prices, the
// template policies plus "August online weeks Saturday to Saturday", a promo
// code and an agreement. It is idempotent: when the establishment already
// has maps it only gives the map kit illustrations to maps seeded before
// the kit (upgradeDemoMapKit: spots without illustration and the four plain
// template decorations; hand-made decorations are kept) and moves the sea of
// the template maps to the top (upgradeDemoSeaTop, geometry only), both
// idempotent and audited. It runs as the system actor (CLI).
// Customers and bookings arrive with M3.
func (s *Service) SeedDemo(ctx context.Context, establishmentID string) (*DemoResult, error) {
	res := &DemoResult{}
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := store.SystemEstablishmentScope(ctx, tx, establishmentID)
		if err != nil {
			return err
		}
		n, err := tx.NewSelect().Model((*store.Map)(nil)).Where("m.establishment_id = ?", es.ID()).Count(ctx)
		if err != nil {
			return fmt.Errorf("setup: demo: %w", err)
		}
		if n > 0 {
			// Already configured: only bring the maps up to the map kit.
			up, err := s.upgradeDemoMapKit(ctx, tx, es, s.d.Clock.Now())
			res.MapsUpgraded, res.SpotsIllustrated = up.Maps, up.SpotsIllustrated
			res.DecorationsRemoved, res.DecorationsAdded = up.DecorationsRemoved, up.DecorationsAdded
			if err != nil {
				return err
			}
			// Then move the sea of the template maps to the top.
			res.MapsReoriented, err = s.upgradeDemoSeaTop(ctx, tx, es, s.d.Clock.Now())
			return err
		}
		e, err := store.GetEstablishment(ctx, tx, es)
		if err != nil {
			return err
		}
		year := DefaultSeasonYear(s.today(e))
		qp := &QuickPrices{HalfDayFactor: &demoHalfDayFactor}
		for _, cat := range []string{CategoryRow1, CategoryRow2, CategoryStandard, CategoryGazebo} {
			for _, p := range Periods {
				qp.Prices = append(qp.Prices, QuickPrice{PriceCategory: cat, RatePeriodCode: p.Code, Amount: DemoDayPrices[cat][p.Code]})
			}
		}
		ar, err := s.applyTx(ctx, tx, es, audit.SystemActor, ApplyInput{
			Template: CodeClassicRows, SeasonYear: &year, QuickPrices: qp, seasonStatus: SeasonActive,
		})
		if err != nil {
			return err
		}
		if err := openDemoBookingWindow(ctx, tx, e, s.d.Clock.Now()); err != nil {
			return err
		}
		extraRules, err := s.seedDemoExtras(ctx, tx, es, ar)
		if err != nil {
			return err
		}
		res.Applied, res.SeasonID, res.SeasonYear = true, ar.SeasonID, ar.SeasonYear
		res.SpotsCreated, res.RulesCreated = ar.SpotsCreated, len(ar.QuickRules)+extraRules
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: audit.SystemActor, Action: "seed.demo_configured",
			EntityType: "season", EntityID: ar.SeasonID,
			Diff: map[string]any{"year": ar.SeasonYear, "spots": ar.SpotsCreated, "rules": res.RulesCreated},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("setup: seed demo: %w", err)
	}
	return res, nil
}

// demoMaxLeadTimeDays lets the demo be booked online right after `seed
// demo`, also off season: the seeded season can be the next one, beyond the
// 120 days of the default policy.
const demoMaxLeadTimeDays = 366

// openDemoBookingWindow widens the maximum lead time of the default policy
// of the demo establishment (settings document, version bumped).
func openDemoBookingWindow(ctx context.Context, tx bun.Tx, e *store.Establishment, now time.Time) error {
	settings, err := establishment.ParseSettings(e.Settings, e.Currency)
	if err != nil {
		return err
	}
	days := demoMaxLeadTimeDays
	settings.DefaultPolicy.MaxLeadTimeDays = &days
	raw, err := store.MarshalJSONValue(settings)
	if err != nil {
		return err
	}
	if _, err := tx.NewUpdate().Model((*store.Establishment)(nil)).
		Set("settings = ?", raw).Set("version = version + 1").Set("updated_at = ?", now).
		Where("id = ?", e.ID).Exec(ctx); err != nil {
		return fmt.Errorf("demo booking window: %w", err)
	}
	return nil
}

// seedDemoExtras adds what the quick price list does not cover: extra
// prices, fixed duration prices, the August policy, a promo code and an
// agreement. It returns the number of price rules created.
func (s *Service) seedDemoExtras(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, ar *ApplyResult) (int, error) {
	now := s.d.Clock.Now()
	var extras []store.Extra
	if err := tx.NewSelect().Model(&extras).Where("ex.establishment_id = ?", es.ID()).Scan(ctx); err != nil {
		return 0, err
	}
	var maxSeq int64
	if err := tx.NewSelect().Model((*store.PriceRule)(nil)).ColumnExpr("COALESCE(MAX(pr.seq), 0)").
		Where("pr.establishment_id = ?", es.ID()).Scan(ctx, &maxSeq); err != nil {
		return 0, err
	}
	var rules []store.PriceRule
	for _, x := range extras {
		amount, ok := demoExtraPrices[x.Code]
		if !ok {
			continue
		}
		maxSeq++
		act, err := store.MarshalJSONValue(pricing.Action{Type: pricing.ActionSet, Amount: amount})
		if err != nil {
			return 0, err
		}
		rules = append(rules, store.PriceRule{
			ID: store.NewID(), EstablishmentID: es.ID(), SeasonID: ar.SeasonID, Name: "Extra · " + x.Name["it"],
			TargetType: string(pricing.TargetExtra), TargetIDs: []string{x.ID}, Conditions: store.JSON(`{}`), Action: act,
			Stage: string(pricing.StageBase), Active: true, Seq: maxSeq, Version: 1, CreatedAt: now, UpdatedAt: now,
		})
	}
	// Weekend surcharge on standard rows in high season (a modifier example).
	maxSeq++
	rules = append(rules, store.PriceRule{
		ID: store.NewID(), EstablishmentID: es.ID(), SeasonID: ar.SeasonID, Name: "Weekend alta stagione +10%",
		TargetType: string(pricing.TargetPack), Conditions: store.JSON(`{"rate_periods":["high"],"weekdays":[6,7]}`),
		Action: store.JSON(`{"type":"multiply","factor":1.1}`), Priority: 10, Stage: string(pricing.StageModifier),
		Active: true, Seq: maxSeq, Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	if _, err := tx.NewInsert().Model(&rules).Exec(ctx); err != nil {
		return 0, fmt.Errorf("insert demo rules: %w", err)
	}

	var base store.Pack
	if err := tx.NewSelect().Model(&base).Where("pa.establishment_id = ?", es.ID()).Where("pa.code = ?", PackBase).Scan(ctx); err != nil {
		return 0, err
	}
	peak := PeriodPeak
	var fixed []store.FixedDurationPrice
	for _, cat := range []string{CategoryRow1, CategoryRow2, CategoryStandard} {
		fixed = append(fixed,
			store.FixedDurationPrice{
				ID: store.NewID(), EstablishmentID: es.ID(), SeasonID: ar.SeasonID, PackID: base.ID, Formula: string(pricing.FormulaSeason),
				PriceCategory: cat, Amount: int64(demoSeasonPrices[cat]), Version: 1, CreatedAt: now, UpdatedAt: now,
			},
			store.FixedDurationPrice{
				ID: store.NewID(), EstablishmentID: es.ID(), SeasonID: ar.SeasonID, PackID: base.ID, Formula: string(pricing.FormulaWeek),
				PriceCategory: cat, RatePeriodCode: &peak, Amount: int64(demoWeekPeakPrices[cat]), Version: 1, CreatedAt: now, UpdatedAt: now,
			},
		)
	}
	if _, err := tx.NewInsert().Model(&fixed).Exec(ctx); err != nil {
		return 0, fmt.Errorf("insert demo fixed prices: %w", err)
	}

	if _, err := s.insertPolicy(ctx, tx, es, ar.SeasonID, policyAugustWeeks); err != nil {
		return 0, err
	}

	opens := calendar.New(ar.SeasonYear, 5, 1)
	closes := calendar.New(ar.SeasonYear, 6, 30)
	maxUses := 200
	if _, err := tx.NewInsert().Model(&store.PromoCode{
		ID: store.NewID(), EstablishmentID: es.ID(), Code: "BENVENUTO10", Description: "10% di sconto a maggio e giugno",
		Action: store.JSON(`{"type":"percent_off","percent":10}`), Conditions: store.JSON(`{"rate_periods":["low","mid"]}`),
		ValidFrom: &opens, ValidTo: &closes, MaxUses: &maxUses, Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Exec(ctx); err != nil {
		return 0, fmt.Errorf("insert demo promo: %w", err)
	}
	code := "HOTELMARE"
	if _, err := tx.NewInsert().Model(&store.Agreement{
		ID: store.NewID(), EstablishmentID: es.ID(), Code: &code, Name: "Hotel Mare", PartnerName: "Hotel Mare Srl",
		Notes: "Convenzione per gli ospiti dell'hotel", Action: store.JSON(`{"type":"percent_off","percent":15}`),
		Conditions: store.JSON(`{}`), Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Exec(ctx); err != nil {
		return 0, fmt.Errorf("insert demo agreement: %w", err)
	}
	return len(rules), nil
}
