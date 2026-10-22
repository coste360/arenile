package pricingconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// PriceList is everything the pure engines need to price and validate a
// booking in one season, assembled from the database.
type PriceList struct {
	Season   store.Season
	Location *time.Location
	Settings establishment.Settings
	Catalog  pricing.Catalog
	Rules    pricing.RuleSet
	// RatePeriods are the season's rate period rows (names, colors).
	RatePeriods []store.RatePeriod
	// Zones and Packs are the catalog rows (codes are used by policy scopes).
	Zones []store.Zone
	Packs []store.Pack
	// Policies are the active booking policies of the season; Settings.DefaultPolicy is the default they inherit from.
	Policies []policy.Policy
}

// LoadPriceList assembles the price list of a season in a scope that the
// caller has already authorized (booking flows of M3 call it with the scope
// of their own authorization). spotIDs selects the spots to include in the
// catalog (spot-specific conditions and price category overrides); unknown
// IDs are ignored.
func LoadPriceList(ctx context.Context, db bun.IDB, es store.EstablishmentScope, seasonID string, spotIDs []string) (*PriceList, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	settings, err := establishment.ParseSettings(est.Settings, est.Currency)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: timezone %q: %w", est.Timezone, err)
	}
	season, err := store.PricingGetSeason(ctx, db, es, seasonID)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	pl := &PriceList{Season: *season, Location: loc, Settings: settings}
	pl.Catalog.Settings = settings.Pricing

	if pl.Packs, err = store.PricingListPacks(ctx, db, es); err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for _, p := range pl.Packs {
		pl.Catalog.Packs = append(pl.Catalog.Packs, pricing.Pack{ID: p.ID, Code: p.Code, VATRateBP: p.VATRateBP, Active: p.Active})
	}
	extras, err := store.PricingListExtras(ctx, db, es)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for _, x := range extras {
		pl.Catalog.Extras = append(pl.Catalog.Extras, pricing.Extra{
			ID: x.ID, Code: x.Code, PricingUnit: pricing.PricingUnit(x.PricingUnit), VATRateBP: x.VATRateBP, Active: x.Active,
		})
	}
	if pl.Zones, err = store.PricingListZones(ctx, db, es); err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for _, z := range pl.Zones {
		pl.Catalog.Zones = append(pl.Catalog.Zones, pricing.Zone{ID: z.ID, Code: z.Code, PriceCategory: z.PriceCategory})
	}
	spots, err := store.PricingListSpotsByID(ctx, db, es, cleanIDs(spotIDs))
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for _, sp := range spots {
		ps := pricing.Spot{ID: sp.ID, Label: sp.Label, ZoneID: sp.ZoneID, Attributes: sp.Attributes}
		if sp.PriceCategoryOverride != nil {
			ps.PriceCategoryOverride = *sp.PriceCategoryOverride
		}
		pl.Catalog.Spots = append(pl.Catalog.Spots, ps)
	}

	if pl.RatePeriods, err = store.PricingListRatePeriods(ctx, db, es, seasonID); err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	pl.Rules.RatePeriods = ratePeriodsToEngine(pl.RatePeriods)
	rules, err := store.ListSeasonPriceRules(ctx, db, es, seasonID)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for i := range rules {
		r, err := ruleFromRow(&rules[i])
		if err != nil {
			return nil, err
		}
		pl.Rules.Rules = append(pl.Rules.Rules, r.Engine())
	}
	fixed, err := store.ListSeasonFixedPrices(ctx, db, es, seasonID)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	pl.Rules.FixedPrices = fixedToEngine(fixed)

	policies, err := store.ListSeasonBookingPolicies(ctx, db, es, seasonID)
	if err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	for i := range policies {
		if !policies[i].Active {
			continue
		}
		bp, err := policyFromRow(&policies[i])
		if err != nil {
			return nil, err
		}
		pl.Policies = append(pl.Policies, bp.Engine())
	}
	return pl, nil
}

func ratePeriodsToEngine(rows []store.RatePeriod) []pricing.RatePeriod {
	out := make([]pricing.RatePeriod, 0, len(rows))
	for _, rp := range rows {
		ranges := make([]pricing.DateRange, len(rp.Ranges))
		for i, r := range rp.Ranges {
			ranges[i] = pricing.DateRange{From: r.From, To: r.To}
		}
		out = append(out, pricing.RatePeriod{ID: rp.ID, Code: rp.Code, Ranges: ranges})
	}
	return out
}

func fixedToEngine(rows []store.FixedDurationPrice) []pricing.FixedDurationPrice {
	out := make([]pricing.FixedDurationPrice, 0, len(rows))
	for _, fp := range rows {
		out = append(out, pricing.FixedDurationPrice{
			ID: fp.ID, PackID: fp.PackID, Formula: pricing.Formula(fp.Formula), PriceCategory: fp.PriceCategory,
			RatePeriodCode: fp.RatePeriodCode, Amount: money.Cents(fp.Amount),
		})
	}
	return out
}

// categories returns the distinct zone price categories in zone order (the
// default category axis of Matrix and Coverage).
func (pl *PriceList) categories() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, z := range pl.Catalog.Zones {
		if !seen[z.PriceCategory] {
			seen[z.PriceCategory] = true
			out = append(out, z.PriceCategory)
		}
	}
	return out
}
