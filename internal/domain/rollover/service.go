package rollover

import (
	"context"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
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
	Warnings       []string
}

// Rollover previews (in.DryRun) or applies the rollover of sourceSeasonID.
// Created reports whether this call wrote the rollover (HTTP 201 vs 200).
func (s *Service) Rollover(ctx context.Context, p *auth.Principal, establishmentID, sourceSeasonID string, in Input) (res *Result, created bool, err error) {
	return nil, false, configerr.ErrNotImplemented
}

// List returns the applied rollovers, newest first.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (store.Page[store.SeasonRollover], error) {
	return store.Page[store.SeasonRollover]{}, configerr.ErrNotImplemented
}
