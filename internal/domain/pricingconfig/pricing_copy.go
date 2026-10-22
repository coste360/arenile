package pricingconfig

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// CopySeasonInput selects what CopySeason duplicates.
type CopySeasonInput struct {
	TargetYear int
	// OpensOn/ClosesOn are used when the target season must be created;
	// nil means the source dates shifted with pricing.ShiftDate.
	OpensOn            *calendar.Date
	ClosesOn           *calendar.Date
	IncludeRatePeriods bool
	IncludePriceRules  bool
	IncludeFixedPrices bool
	IncludePolicies    bool
}

// CopySeasonResult is the outcome of CopySeason.
type CopySeasonResult struct {
	Season         store.Season
	SeasonCreated  bool
	RatePeriods    int
	PriceRules     int
	FixedPrices    int
	Policies       int
	Warnings       []string
	SourceSeasonID string
}

// Season statuses (spec 6.1) used when the copy creates the target season.
const seasonDraft = "draft"

// CopySeason duplicates the price list of a season onto TargetYear (spec
// 8.7): rate periods, price rules and fixed prices through pricing.CopySeason
// (dates shifted keeping the weekday where it makes sense), and booking
// policies. The target season is created (status draft) when the
// establishment has none for that year; an existing target season must not
// already contain objects of the included kinds (ErrTargetSeasonInUse).
// Copied rules get fresh seq values above every existing rule, preserving
// their relative order. Everything happens in one transaction with one audit
// entry and one config.pricing_changed event.
func (s *Service) CopySeason(ctx context.Context, p *auth.Principal, establishmentID, sourceSeasonID string, in CopySeasonInput) (*CopySeasonResult, error) {
	var out *CopySeasonResult
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		es, err := authorize(ctx, tx, p, establishmentID, auth.PermEstablishmentConfigure)
		if err != nil {
			return err
		}
		src, err := store.PricingGetSeason(ctx, tx, es, sourceSeasonID)
		if err != nil {
			return mapStoreErr(err, nil)
		}
		if in.TargetYear < 2000 || in.TargetYear > 2200 {
			return configerr.Invalid("target_year", "Anno non valido.")
		}
		if in.TargetYear == src.Year {
			return configerr.Invalid("target_year", "Deve essere diverso dall'anno della stagione di origine.")
		}
		res := &CopySeasonResult{SourceSeasonID: src.ID, Warnings: []string{}}
		target, err := s.targetSeason(ctx, tx, p, es, src, in, res)
		if err != nil {
			return err
		}
		if err := ensureEmptyTarget(ctx, tx, es, target.ID, in); err != nil {
			return err
		}
		if err := s.copyInto(ctx, tx, es, src, target, in, res); err != nil {
			return err
		}
		res.Season = *target
		out = res
		diff := map[string]any{
			"source_season_id": src.ID, "target_season_id": target.ID, "target_year": target.Year,
			"season_created": res.SeasonCreated, "rate_periods": res.RatePeriods, "price_rules": res.PriceRules,
			"fixed_prices": res.FixedPrices, "policies": res.Policies, "warnings": res.Warnings,
		}
		return change(ctx, tx, s.d, p, es, EntitySeasonCopy, target.ID, ChangeCopied, target.ID, diff)
	})
	return out, err
}

func (s *Service) targetSeason(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, src *store.Season, in CopySeasonInput, res *CopySeasonResult) (*store.Season, error) {
	target, err := store.PricingSeasonByYear(ctx, tx, es, in.TargetYear)
	if err == nil {
		return target, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	years := in.TargetYear - src.Year
	opens, closes := pricing.ShiftDate(src.OpensOn, years, false), pricing.ShiftDate(src.ClosesOn, years, true)
	if in.OpensOn != nil {
		opens = *in.OpensOn
	}
	if in.ClosesOn != nil {
		closes = *in.ClosesOn
	}
	v := &configerr.ValidationError{}
	if opens.Year() != in.TargetYear {
		v.Add("opens_on", "Deve cadere nell'anno di destinazione.")
	}
	if closes.Before(opens) {
		v.Add("closes_on", "Deve essere successiva o uguale a opens_on.")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	now := s.now()
	target = &store.Season{
		ID: store.NewID(), Year: in.TargetYear, OpensOn: opens, ClosesOn: closes, Status: seasonDraft,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.PricingInsertSeason(ctx, tx, es, target); err != nil {
		return nil, mapStoreErr(err, ErrTargetSeasonInUse)
	}
	res.SeasonCreated = true
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actorOf(ctx, p), Action: "season.created", EntityType: "season",
		EntityID: target.ID, Diff: map[string]any{"after": target, "copied_from": src.ID},
	}); err != nil {
		return nil, fmt.Errorf("pricingconfig: %w", err)
	}
	return target, nil
}

func ensureEmptyTarget(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, seasonID string, in CopySeasonInput) error {
	if in.IncludeRatePeriods {
		rps, err := store.PricingListRatePeriods(ctx, tx, es, seasonID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		if len(rps) > 0 {
			return ErrTargetSeasonInUse
		}
	}
	if in.IncludePriceRules {
		n, err := store.CountSeasonPriceRules(ctx, tx, es, seasonID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		if n > 0 {
			return ErrTargetSeasonInUse
		}
	}
	if in.IncludeFixedPrices {
		n, err := store.CountSeasonFixedPrices(ctx, tx, es, seasonID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		if n > 0 {
			return ErrTargetSeasonInUse
		}
	}
	if in.IncludePolicies {
		ps, err := store.ListSeasonBookingPolicies(ctx, tx, es, seasonID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		if len(ps) > 0 {
			return ErrTargetSeasonInUse
		}
	}
	return nil
}

func (s *Service) copyInto(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, src, target *store.Season, in CopySeasonInput, res *CopySeasonResult) error {
	srcPeriods, err := store.PricingListRatePeriods(ctx, tx, es, src.ID)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	ruleRows, err := store.ListSeasonPriceRules(ctx, tx, es, src.ID)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	fixedRows, err := store.ListSeasonFixedPrices(ctx, tx, es, src.ID)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	cin := pricing.CopySeasonInput{
		FromYear: src.Year, ToYear: target.Year,
		NewID: func(string, string) string { return store.NewID() },
	}
	if in.IncludeRatePeriods {
		cin.RatePeriods = ratePeriodsToEngine(srcPeriods)
	}
	if in.IncludePriceRules {
		for i := range ruleRows {
			r, err := ruleFromRow(&ruleRows[i])
			if err != nil {
				return err
			}
			cin.Rules = append(cin.Rules, r.Engine())
		}
	}
	if in.IncludeFixedPrices {
		cin.FixedPrices = fixedToEngine(fixedRows)
	}
	copied, err := pricing.CopySeason(cin)
	if err != nil {
		return fromEngine(err, "target_year")
	}
	res.Warnings = append(res.Warnings, copied.Warnings...)
	now := s.now()

	// Rate periods: keep name, color and order of the source; clip ranges to
	// the target season.
	byCode := map[string]store.RatePeriod{}
	for _, rp := range srcPeriods {
		byCode[rp.Code] = rp
	}
	targetCodes := map[string]bool{}
	if !in.IncludeRatePeriods {
		existing, err := store.PricingListRatePeriods(ctx, tx, es, target.ID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		for _, rp := range existing {
			targetCodes[rp.Code] = true
		}
	}
	for _, rp := range copied.RatePeriods {
		ranges, clipped := clipRanges(rp.Ranges, target.OpensOn, target.ClosesOn)
		if clipped {
			res.Warnings = append(res.Warnings, fmt.Sprintf("period %q was trimmed to the season dates %s - %s", rp.Code, target.OpensOn, target.ClosesOn))
		}
		if len(ranges) == 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("period %q falls outside the target season and was not copied", rp.Code))
			continue
		}
		srcRow := byCode[rp.Code]
		row := &store.RatePeriod{
			ID: rp.ID, SeasonID: target.ID, Code: rp.Code, Name: srcRow.Name, Color: srcRow.Color, Ranges: ranges,
			SortOrder: srcRow.SortOrder, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if row.Name == nil {
			row.Name = store.I18nText{}
		}
		if err := store.PricingInsertRatePeriod(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, ErrTargetSeasonInUse)
		}
		targetCodes[rp.Code] = true
		res.RatePeriods++
	}

	// Price rules: fresh seq above every rule of the establishment, same
	// relative order as in the source.
	sort.SliceStable(copied.Rules, func(i, j int) bool { return copied.Rules[i].Seq < copied.Rules[j].Seq })
	seq, err := store.NextPriceRuleSeq(ctx, tx, es)
	if err != nil {
		return fmt.Errorf("pricingconfig: %w", err)
	}
	for _, r := range copied.Rules {
		cond, err := jsonOf(r.Conditions)
		if err != nil {
			return err
		}
		act, err := jsonOf(r.Action)
		if err != nil {
			return err
		}
		row := &store.PriceRule{
			ID: r.ID, SeasonID: target.ID, Name: r.Name, TargetType: string(r.TargetType), TargetIDs: r.TargetIDs,
			Conditions: cond, Action: act, Priority: r.Priority, Stage: string(r.Stage), Stackable: r.Stackable,
			Active: r.Active, Seq: seq, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		seq++
		if err := store.InsertPriceRule(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, nil)
		}
		res.PriceRules++
	}

	for _, fp := range copied.FixedPrices {
		if fp.RatePeriodCode != nil && !targetCodes[*fp.RatePeriodCode] {
			return configerr.Invalid("include_rate_periods",
				fmt.Sprintf("Il prezzo fisso usa il periodo %q, assente nella stagione di destinazione: copia anche i periodi.", *fp.RatePeriodCode))
		}
		row := &store.FixedDurationPrice{
			ID: fp.ID, SeasonID: target.ID, PackID: fp.PackID, Formula: string(fp.Formula), PriceCategory: fp.PriceCategory,
			RatePeriodCode: fp.RatePeriodCode, Amount: int64(fp.Amount), Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertFixedPrice(ctx, tx, es, row); err != nil {
			return mapStoreErr(err, ErrDuplicateFixed)
		}
		res.FixedPrices++
	}

	if in.IncludePolicies {
		pols, err := store.ListSeasonBookingPolicies(ctx, tx, es, src.ID)
		if err != nil {
			return fmt.Errorf("pricingconfig: %w", err)
		}
		for i := range pols {
			row := pols[i]
			row.ID, row.SeasonID, row.Version, row.CreatedAt, row.UpdatedAt = store.NewID(), target.ID, 1, now, now
			if err := store.InsertBookingPolicy(ctx, tx, es, &row); err != nil {
				return mapStoreErr(err, nil)
			}
			res.Policies++
		}
	}
	return nil
}

// clipRanges intersects ranges with [from, to]; clipped reports whether any
// range changed or was dropped.
func clipRanges(rs []pricing.DateRange, from, to calendar.Date) ([]store.DateRange, bool) {
	out := []store.DateRange{}
	clipped := false
	for _, r := range rs {
		f, t := r.From, r.To
		if f.Before(from) {
			f, clipped = from, true
		}
		if t.After(to) {
			t, clipped = to, true
		}
		if t.Before(f) {
			clipped = true
			continue
		}
		out = append(out, store.DateRange{From: f, To: t})
	}
	return out, clipped
}
