package season

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the season service. Every field is required
// except Log.
type Deps struct {
	DB     bun.IDB
	Clock  clock.Clock
	Audit  *audit.Writer
	Outbox *event.Outbox
	Log    *slog.Logger
}

// Service implements the season, rate period and closure day use cases.
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service { return &Service{d: d} }

// WithDB returns a copy of the service that runs on db, typically the
// transaction of a composing use case (guided setup, season copy). Nested
// transactions become savepoints, so the caller's transaction commits or
// rolls back everything together.
func (s *Service) WithDB(db bun.IDB) *Service {
	c := *s
	c.d.DB = db
	return &c
}

// Errors of the season package. They wrap the configerr kinds, which the
// HTTP layer maps to problem types.
var (
	ErrSeasonYearTaken  = fmt.Errorf("%w: esiste già una stagione per questo anno", configerr.ErrConflict)
	ErrSeasonNotDraft   = fmt.Errorf("%w: si possono eliminare solo le stagioni in bozza", configerr.ErrConflict)
	ErrInvalidStatus    = fmt.Errorf("%w: cambio di stato non consentito", configerr.ErrConflict)
	ErrCodeTaken        = fmt.Errorf("%w: codice del periodo già usato in questa stagione", configerr.ErrConflict)
	ErrClosureDateTaken = fmt.Errorf("%w: esiste già una chiusura in questa data", configerr.ErrConflict)
	ErrTargetYearTaken  = fmt.Errorf("%w: esiste già una stagione per l'anno di destinazione", configerr.ErrConflict)
)

// Audit actions.
const (
	ActionSeasonCreated     = "season.created"
	ActionSeasonUpdated     = "season.updated"
	ActionSeasonDeleted     = "season.deleted"
	ActionSeasonCopied      = "season.copied"
	ActionRatePeriodCreated = "rate_period.created"
	ActionRatePeriodUpdated = "rate_period.updated"
	ActionRatePeriodDeleted = "rate_period.deleted"
	ActionClosureCreated    = "closure_day.created"
	ActionClosureUpdated    = "closure_day.updated"
	ActionClosureDeleted    = "closure_day.deleted"
)

// Page size bounds.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// authorize resolves the establishment in the principal's scope and checks
// perm (configuration for mutations, view for reads).
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if err != nil {
		return store.EstablishmentScope{}, err
	}
	return es, nil
}

// record writes an audit entry in tx.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, ip, action, entityType, entityID string, diff any) error {
	return s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: action, EntityType: entityType, EntityID: entityID, Diff: diff,
	})
}

// pricingChanged emits config.pricing_changed in tx with the shared
// event.ConfigChange payload (version 1): entity "season" or "rate_period",
// its id and season, change created | updated | deleted | copied.
func (s *Service) pricingChanged(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, resource, id, seasonID, action string) error {
	_, err := s.d.Outbox.Write(ctx, tx, event.ConfigPricingChanged, es.ID(), 1, event.ConfigChange{
		Entity: resource, EntityID: id, SeasonID: seasonID, Change: action,
	})
	return err
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageSize
	}
	return min(limit, MaxPageSize)
}

// passThrough reports whether err is a domain error to return unwrapped.
func passThrough(err error) bool {
	return errors.Is(err, configerr.ErrValidation) || errors.Is(err, configerr.ErrConflict) ||
		errors.Is(err, configerr.ErrVersionConflict) || errors.Is(err, store.ErrNotFound) ||
		errors.Is(err, auth.ErrForbidden) || errors.Is(err, store.ErrInvalidCursor)
}

func wrap(op string, err error) error {
	if err == nil || passThrough(err) {
		return err
	}
	return fmt.Errorf("season: %s: %w", op, err)
}
