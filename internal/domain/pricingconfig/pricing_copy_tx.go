package pricingconfig

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// CopySeasonInTx is the body of CopySeason inside the caller's transaction
// (no transaction of its own): the season rollover (internal/domain/
// rollover) copies the price list and the policies together with the
// closure days and its season_rollovers row, so that the whole passage
// commits or rolls back at once. es must come from p (the caller resolved
// it); the permission establishment.configure is checked again here.
// Implements rollover.SeasonCopier.
func (s *Service) CopySeasonInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, sourceSeasonID string, in CopySeasonInput) (*CopySeasonResult, error) {
	if p == nil {
		return nil, auth.ErrUnauthenticated
	}
	if !p.Can(es, auth.PermEstablishmentConfigure) {
		return nil, auth.ErrForbidden
	}
	src, err := store.PricingGetSeason(ctx, tx, es, sourceSeasonID)
	if err != nil {
		return nil, mapStoreErr(err, nil)
	}
	if in.TargetYear < 2000 || in.TargetYear > 2200 {
		return nil, configerr.Invalid("target_year", "Anno non valido.")
	}
	if in.TargetYear == src.Year {
		return nil, configerr.Invalid("target_year", "Deve essere diverso dall'anno della stagione di origine.")
	}
	res := &CopySeasonResult{SourceSeasonID: src.ID, Warnings: []string{}}
	target, err := s.targetSeason(ctx, tx, p, es, src, in, res)
	if err != nil {
		return nil, err
	}
	if err := ensureEmptyTarget(ctx, tx, es, target.ID, in); err != nil {
		return nil, err
	}
	if err := s.copyInto(ctx, tx, es, src, target, in, res); err != nil {
		return nil, err
	}
	res.Season = *target
	diff := map[string]any{
		"source_season_id": src.ID, "target_season_id": target.ID, "target_year": target.Year,
		"season_created": res.SeasonCreated, "rate_periods": res.RatePeriods, "price_rules": res.PriceRules,
		"fixed_prices": res.FixedPrices, "policies": res.Policies, "warnings": res.Warnings,
	}
	if err := change(ctx, tx, s.d, p, es, EntitySeasonCopy, target.ID, ChangeCopied, target.ID, diff); err != nil {
		return nil, err
	}
	return res, nil
}
