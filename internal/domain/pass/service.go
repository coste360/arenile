package pass

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
	"github.com/coste360/arenile/internal/store"
)

// Errors of the pass domain. They wrap the configerr kinds so that the HTTP
// layer maps them to stable problem types (Italian text after the kind).
var (
	// ErrNotFound: unknown season pass (or a booking that is not a pass).
	ErrNotFound = fmt.Errorf("%w: season pass", configerr.ErrNotFound)
	// ErrReleaseNotFound: unknown release of the pass.
	ErrReleaseNotFound = fmt.Errorf("%w: season release", configerr.ErrNotFound)
	// ErrEstablishmentNotFound: unknown establishment or outside the scope.
	ErrEstablishmentNotFound = fmt.Errorf("%w: establishment", configerr.ErrNotFound)
	// ErrNotActive: the pass is not confirmed or checked in (cancelled,
	// still held...), so its days cannot be released or restored.
	ErrNotActive = fmt.Errorf("%w: abbonamento non attivo: i giorni non possono essere rilasciati o ripristinati", configerr.ErrConflict)
	// ErrAlreadyReleased: some requested half-days are already released.
	ErrAlreadyReleased = fmt.Errorf("%w: alcuni giorni sono già stati rilasciati", configerr.ErrConflict)
	// ErrReleaseResold: the released half-day was sold again (spec 7.7).
	ErrReleaseResold = fmt.Errorf("%w: il giorno rilasciato è già stato rivenduto e non può essere revocato", configerr.ErrConflict)
	// ErrReleaseRevoked: the release was already revoked.
	ErrReleaseRevoked = fmt.Errorf("%w: rilascio già revocato", configerr.ErrConflict)
	// ErrReleasePast: the released half-day is in the past.
	ErrReleasePast = fmt.Errorf("%w: il giorno rilasciato è già passato", configerr.ErrConflict)
)

// activeStatuses are the booking statuses in which a pass occupies its spot
// and may release days.
var activeStatuses = map[string]bool{"confirmed": true, "checked_in": true}

// Deps are the dependencies of the pass service. Every field is required
// except Log.
type Deps struct {
	DB        bun.IDB
	Clock     clock.Clock
	Audit     *audit.Writer
	Outbox    *event.Outbox
	Log       *slog.Logger
	Bookings  Bookings
	Inventory Inventory
	Credits   Credits
}

// Service implements the season pass use cases.
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

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perm auth.Permission) (store.EstablishmentScope, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if errors.Is(err, store.ErrNotFound) {
		return es, ErrEstablishmentNotFound
	}
	return es, err
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// today is the current date in the establishment timezone.
func (s *Service) today(ctx context.Context, db bun.IDB, es store.EstablishmentScope) (calendar.Date, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("pass: %w", err)
	}
	return todayIn(s.d.Clock, est), nil
}

func todayIn(c clock.Clock, est *store.Establishment) calendar.Date {
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return calendar.In(c.Now(), loc)
}

// record writes the audit entry and, when eventType is set, the outbox
// event of a pass change in tx (INV-18).
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, ip string, es store.EstablishmentScope, action, passID string, diff any, eventType string, payload any) error {
	err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: action,
		EntityType: "booking", EntityID: passID, Diff: diff,
	})
	if err != nil {
		return fmt.Errorf("pass: %w", err)
	}
	if eventType == "" {
		return nil
	}
	if _, err := s.d.Outbox.Write(ctx, tx, eventType, es.ID(), 1, payload); err != nil {
		return fmt.Errorf("pass: %w", err)
	}
	return nil
}

// loadPass returns the season pass id of es, mapping not found.
func loadPass(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string) (*store.SeasonPassView, error) {
	v, err := store.GetSeasonPass(ctx, db, es, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pass: %w", err)
	}
	return v, nil
}

// ResaleDeps are the dependencies of Resale.
type ResaleDeps struct {
	Clock   clock.Clock
	Credits Credits
	Log     *slog.Logger
}

// Resale marks released season half-days as resold. It implements
// booking.Releases and has no dependency on the booking service.
type Resale struct {
	d ResaleDeps
}

// NewResale returns a Resale.
func NewResale(d ResaleDeps) *Resale {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Resale{d: d}
}
