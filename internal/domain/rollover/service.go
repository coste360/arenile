package rollover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the service.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	Copier SeasonCopier
}

// Service implements the season rollover use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Audit == nil && d.Clock != nil {
		d.Audit = audit.NewWriter(d.Clock)
	}
	if d.Outbox == nil && d.Clock != nil {
		d.Outbox = event.NewOutbox(d.Clock)
	}
	return &Service{d: d}
}

// Input is api.SeasonRolloverRequest (the Include* flags default to true in
// the handler).
type Input struct {
	TargetYear         int
	OpensOn            *calendar.Date
	ClosesOn           *calendar.Date
	IncludeRatePeriods bool
	IncludeClosureDays bool
	IncludePriceRules  bool
	IncludeFixedPrices bool
	IncludePolicies    bool
	DryRun             bool
	IP                 string
}

// Counts are the copied rows per kind.
type Counts struct {
	RatePeriods, ClosureDays, PriceRules, FixedPrices, Policies int
}

// Result is api.SeasonRolloverResult.
type Result struct {
	DryRun         bool
	AlreadyApplied bool
	RolloverID     string // empty on dry run
	SourceSeasonID string
	TargetYear     int
	OpensOn        calendar.Date
	ClosesOn       calendar.Date
	Season         *store.Season // nil on dry run
	Counts         Counts
	SeasonPasses   int // confirmed passes of the source season (renewal candidates)
	Warnings       []Warning
}

// Rollover previews (in.DryRun) or applies the rollover of sourceSeasonID.
// Created reports whether this call wrote the rollover (HTTP 201 vs 200).
//
// The preview runs exactly the same writes as the apply inside a
// transaction that is always rolled back, so counts and warnings are the
// ones an apply would produce. An apply already recorded returns its result
// with AlreadyApplied (idempotent); concurrent applies serialise on the lock
// of the source season row and the loser finds the winner's row.
func (s *Service) Rollover(ctx context.Context, p *auth.Principal, establishmentID, sourceSeasonID string, in Input) (res *Result, created bool, err error) {
	if p == nil {
		return nil, false, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return nil, false, err
	}
	if in.TargetYear < 2000 || in.TargetYear > 2100 {
		return nil, false, configerr.Invalid("target_year", "Anno non valido (2000-2100).")
	}
	if s.d.Copier == nil {
		return nil, false, fmt.Errorf("rollover: %w: season copier not configured", configerr.ErrNotImplemented)
	}
	src, err := store.GetSeason(ctx, s.d.DB, es, sourceSeasonID)
	if err != nil {
		return nil, false, err
	}
	if _, _, err := TargetDates(*src, in.TargetYear, in.OpensOn, in.ClosesOn); err != nil {
		return nil, false, err
	}
	if prev, err := s.recorded(ctx, s.d.DB, es, src.ID, in.TargetYear); err != nil || prev != nil {
		return prev, false, err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.LockSeason(ctx, tx, es, src.ID); err != nil {
			return err
		}
		prev, err := s.recorded(ctx, tx, es, src.ID, in.TargetYear)
		if err != nil {
			return err
		}
		if prev != nil {
			res = prev
			return nil
		}
		res, err = s.apply(ctx, tx, p, es, src, in)
		if err != nil {
			return err
		}
		created = !in.DryRun
		if in.DryRun {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		err = nil
	}
	if err != nil {
		return nil, false, err
	}
	return res, created, nil
}

// errDryRun rolls back the transaction of a preview.
var errDryRun = errors.New("rollover: dry run")

// recordedResult is the JSON of season_rollovers.result.
type recordedResult struct {
	OpensOn      calendar.Date `json:"opens_on"`
	ClosesOn     calendar.Date `json:"closes_on"`
	RatePeriods  int           `json:"rate_periods"`
	ClosureDays  int           `json:"closure_days"`
	PriceRules   int           `json:"price_rules"`
	FixedPrices  int           `json:"fixed_prices"`
	Policies     int           `json:"policies"`
	SeasonPasses int           `json:"season_passes"`
	Warnings     []Warning     `json:"warnings"`
}

// recordedOptions is the JSON of season_rollovers.options.
type recordedOptions struct {
	IncludeRatePeriods bool `json:"include_rate_periods"`
	IncludeClosureDays bool `json:"include_closure_days"`
	IncludePriceRules  bool `json:"include_price_rules"`
	IncludeFixedPrices bool `json:"include_fixed_prices"`
	IncludePolicies    bool `json:"include_policies"`
}

// CountsOf decodes the counts recorded on a rollover row (zero on a
// malformed row).
func CountsOf(r *store.SeasonRollover) Counts {
	var rr recordedResult
	_ = json.Unmarshal(r.Result, &rr)
	return Counts{RatePeriods: rr.RatePeriods, ClosureDays: rr.ClosureDays, PriceRules: rr.PriceRules, FixedPrices: rr.FixedPrices, Policies: rr.Policies}
}

// recorded returns the result of an applied rollover, nil when none.
func (s *Service) recorded(ctx context.Context, db bun.IDB, es store.EstablishmentScope, sourceID string, year int) (*Result, error) {
	r, err := store.GetSeasonRollover(ctx, db, es, sourceID, year)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rr recordedResult
	if err := json.Unmarshal(r.Result, &rr); err != nil {
		return nil, fmt.Errorf("rollover: decode result: %w", err)
	}
	target, err := store.GetSeason(ctx, db, es, r.TargetSeasonID)
	if err != nil {
		return nil, fmt.Errorf("rollover: target season: %w", err)
	}
	if rr.Warnings == nil {
		rr.Warnings = []Warning{}
	}
	return &Result{
		AlreadyApplied: true, RolloverID: r.ID, SourceSeasonID: r.SourceSeasonID, TargetYear: r.TargetYear,
		OpensOn: target.OpensOn, ClosesOn: target.ClosesOn, Season: target, Counts: CountsOf(r),
		SeasonPasses: rr.SeasonPasses, Warnings: rr.Warnings,
	}, nil
}

// apply runs the rollover in tx: the target year must be free, the price
// list and the policies are copied by the SeasonCopier (which creates the
// draft season), closure days are shifted, the rollover row, the audit
// entry and season.rolled_over are written.
func (s *Service) apply(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, src *store.Season, in Input) (*Result, error) {
	switch _, err := store.GetSeasonByYear(ctx, tx, es, in.TargetYear); {
	case err == nil:
		return nil, ErrTargetExists
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	copied, err := s.d.Copier.CopySeasonInTx(ctx, tx, p, es, src.ID, pricingconfig.CopySeasonInput{
		TargetYear: in.TargetYear, OpensOn: in.OpensOn, ClosesOn: in.ClosesOn,
		IncludeRatePeriods: in.IncludeRatePeriods, IncludePriceRules: in.IncludePriceRules,
		IncludeFixedPrices: in.IncludeFixedPrices, IncludePolicies: in.IncludePolicies,
	})
	if err != nil {
		return nil, err
	}
	target := copied.Season
	res := &Result{
		DryRun: in.DryRun, SourceSeasonID: src.ID, TargetYear: in.TargetYear, OpensOn: target.OpensOn, ClosesOn: target.ClosesOn,
		Counts: Counts{
			RatePeriods: copied.RatePeriods, PriceRules: copied.PriceRules, FixedPrices: copied.FixedPrices, Policies: copied.Policies,
		},
		Warnings: []Warning{},
	}
	for _, w := range copied.Warnings {
		res.Warnings = append(res.Warnings, Warning{Code: WarnSeasonCopy, Text: w})
	}
	now := s.d.Clock.Now().UTC()
	if in.IncludeClosureDays {
		n, warnings, err := s.copyClosures(ctx, tx, es, src, &target, now)
		if err != nil {
			return nil, err
		}
		res.Counts.ClosureDays = n
		res.Warnings = append(res.Warnings, warnings...)
	}
	if res.SeasonPasses, err = store.CountRenewableSeasonPasses(ctx, tx, es, src.ID); err != nil {
		return nil, err
	}
	if in.DryRun {
		return res, nil
	}
	options, err := store.MarshalJSONValue(recordedOptions{
		IncludeRatePeriods: in.IncludeRatePeriods, IncludeClosureDays: in.IncludeClosureDays, IncludePriceRules: in.IncludePriceRules,
		IncludeFixedPrices: in.IncludeFixedPrices, IncludePolicies: in.IncludePolicies,
	})
	if err != nil {
		return nil, err
	}
	result, err := store.MarshalJSONValue(recordedResult{
		OpensOn: target.OpensOn, ClosesOn: target.ClosesOn, RatePeriods: res.Counts.RatePeriods, ClosureDays: res.Counts.ClosureDays,
		PriceRules: res.Counts.PriceRules, FixedPrices: res.Counts.FixedPrices, Policies: res.Counts.Policies,
		SeasonPasses: res.SeasonPasses, Warnings: res.Warnings,
	})
	if err != nil {
		return nil, err
	}
	row := &store.SeasonRollover{
		ID: store.NewID(), SourceSeasonID: src.ID, TargetSeasonID: target.ID, TargetYear: in.TargetYear,
		Options: options, Result: result, CreatedAt: now,
	}
	if p.Kind == auth.PrincipalUser && p.UserID != "" {
		u := p.UserID
		row.CreatedByUserID = &u
	}
	if err := store.InsertSeasonRollover(ctx, tx, es, row); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, ErrTargetExists
		}
		return nil, err
	}
	res.RolloverID, res.Season = row.ID, &target
	counts := map[string]int{
		"rate_periods": res.Counts.RatePeriods, "closure_days": res.Counts.ClosureDays, "price_rules": res.Counts.PriceRules,
		"fixed_prices": res.Counts.FixedPrices, "policies": res.Counts.Policies,
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: "season.rolled_over", EntityType: "season_rollover", EntityID: row.ID,
		Diff: map[string]any{
			"source_season_id": src.ID, "target_season_id": target.ID, "target_year": in.TargetYear,
			"opens_on": target.OpensOn.String(), "closes_on": target.ClosesOn.String(), "counts": counts,
			"season_passes": res.SeasonPasses, "warnings": len(res.Warnings),
		},
	}); err != nil {
		return nil, fmt.Errorf("rollover: audit: %w", err)
	}
	if _, err := s.d.Outbox.Write(ctx, tx, event.SeasonRolledOver, es.ID(), 1, map[string]any{
		"rollover_id": row.ID, "source_season_id": src.ID, "target_season_id": target.ID, "target_year": in.TargetYear, "counts": counts,
	}); err != nil {
		return nil, fmt.Errorf("rollover: outbox: %w", err)
	}
	s.d.Log.InfoContext(ctx, "season rolled over", slog.String("establishment_id", es.ID()), slog.String("rollover_id", row.ID),
		slog.Int("target_year", in.TargetYear))
	return res, nil
}

// copyClosures copies the closure days of the source season onto target.
func (s *Service) copyClosures(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, src, target *store.Season, now time.Time) (int, []Warning, error) {
	srcDays, err := store.RolloverClosureDays(ctx, tx, es, src.OpensOn, src.ClosesOn)
	if err != nil {
		return 0, nil, err
	}
	have, err := store.RolloverClosureDays(ctx, tx, es, target.OpensOn, target.ClosesOn)
	if err != nil {
		return 0, nil, err
	}
	existing := make(map[string]bool, len(have))
	for _, c := range have {
		existing[c.Date.String()] = true
	}
	plan, warnings := PlanClosureDays(srcDays, target.Year-src.Year, target.OpensOn, target.ClosesOn, existing)
	for _, c := range plan {
		row := &store.ClosureDay{
			ID: store.NewID(), Date: c.Date, Slots: c.Slots, Reason: c.Reason, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.InsertClosureDay(ctx, tx, es, row); err != nil {
			return 0, nil, fmt.Errorf("rollover: closure %s: %w", c.Date, err)
		}
	}
	return len(plan), warnings, nil
}

// List returns the applied rollovers, newest first.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[store.SeasonRollover], error) {
	if p == nil {
		return store.Page[store.SeasonRollover]{}, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermEstablishmentConfigure)
	if err != nil {
		return store.Page[store.SeasonRollover]{}, err
	}
	return store.ListSeasonRollovers(ctx, s.d.DB, es, cursor, limit)
}
