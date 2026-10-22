package mapping

import (
	"context"
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
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the mapping service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
	// Allocations reports future allocations of spots; nil means NoAllocations.
	Allocations AllocationChecker
}

// Service implements the mapping use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Allocations == nil {
		d.Allocations = NoAllocations{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db. Callers that compose
// several services in one transaction (the guided setup) pass their bun.Tx:
// the service then runs its own transactions as savepoints of it.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if err != nil {
		return store.EstablishmentScope{}, err
	}
	return es, nil
}

func (s *Service) configure(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, error) {
	return s.authorize(ctx, p, establishmentID, auth.PermEstablishmentConfigure)
}

func (s *Service) view(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, error) {
	return s.authorize(ctx, p, establishmentID, auth.PermBookingsRead)
}

// record writes the audit entry and the config.map_changed event of a
// change, in the transaction of the change.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, action string, data event.ConfigChange, diff any) error {
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(),
		Actor:           p.Actor(reqctx.From(ctx).ClientIP),
		Action:          action,
		EntityType:      data.Entity,
		EntityID:        data.EntityID,
		Diff:            diff,
	}); err != nil {
		return err
	}
	if _, err := s.d.Outbox.Write(ctx, tx, event.ConfigMapChanged, es.ID(), 1, data); err != nil {
		return err
	}
	return nil
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// today is the current calendar date in the establishment timezone. An
// establishment whose timezone does not load is an error: the future
// allocation check must never run on the wrong day (same rule as the policy
// evaluation).
func (s *Service) today(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (calendar.Date, error) {
	e, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("mapping: load establishment: %w", err)
	}
	loc, err := time.LoadLocation(e.Timezone)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("mapping: establishment timezone %q: %w", e.Timezone, err)
	}
	return calendar.In(s.d.Clock.Now(), loc), nil
}

// checkAllocations rejects the deletion of spots with future allocations.
func (s *Service) checkAllocations(ctx context.Context, db bun.IDB, es store.EstablishmentScope, spotIDs []string) error {
	if len(spotIDs) == 0 {
		return nil
	}
	today, err := s.today(ctx, db, es)
	if err != nil {
		return err
	}
	busy, err := s.d.Allocations.SpotsWithFutureAllocations(ctx, db, es, spotIDs, today)
	if err != nil {
		return fmt.Errorf("mapping: check allocations: %w", err)
	}
	if len(busy) > 0 {
		return &configerr.SpotsAllocatedError{SpotIDs: busy}
	}
	return nil
}

// notFound maps store.ErrNotFound to the area sentinel.
func notFound(err, sentinel error) error {
	if errors.Is(err, store.ErrNotFound) {
		return sentinel
	}
	return err
}

// inTx runs fn in a transaction of the service database.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, tx bun.Tx) error) error {
	return store.RunInTx(ctx, s.d.DB, fn)
}
