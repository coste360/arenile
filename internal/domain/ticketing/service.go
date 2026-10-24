package ticketing

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
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Deps are the dependencies of the ticketing service. DB, Clock and Audit
// are required; BookingsIn is required by Void (the cancellation of the
// ticket's booking runs in the void transaction).
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
	// Bookings performs the booking state changes of a ticket (see Bookings).
	Bookings Bookings
	// BookingsIn binds Bookings to a transaction: every state change it
	// makes (audit and outbox included) joins the caller's transaction as a
	// savepoint. internal/app wires (*booking.Service).WithDB.
	BookingsIn func(db bun.IDB) Bookings
}

// Service implements the ticketing use cases (spec 14.3, ADR 0020).
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// WithDB returns a copy of the service bound to db.
func (s *Service) WithDB(db bun.IDB) *Service {
	d := s.d
	d.DB = db
	return &Service{d: d}
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// authorize resolves the establishment in the principal's scope and checks perm.
func (s *Service) authorize(ctx context.Context, p *auth.Principal, establishmentID string, perms ...auth.Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, auth.ErrUnauthenticated
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perms[0])
	if err != nil {
		return es, err
	}
	for _, perm := range perms[1:] {
		if !p.Can(es, perm) {
			return es, auth.ErrForbidden
		}
	}
	return es, nil
}

// Today is the business date of the establishment (its timezone).
func Today(ctx context.Context, db bun.IDB, es store.EstablishmentScope, c clock.Clock) (calendar.Date, error) {
	est, err := store.GetEstablishment(ctx, db, es)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("ticketing: establishment: %w", err)
	}
	loc, err := time.LoadLocation(est.Timezone)
	if err != nil {
		return calendar.Date{}, fmt.Errorf("ticketing: timezone %q: %w", est.Timezone, err)
	}
	return calendar.In(c.Now(), loc), nil
}

// record writes one audit entry of a ticket.
func (s *Service) record(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, action string, t *store.Ticket, diff map[string]any) error {
	if diff == nil {
		diff = map[string]any{}
	}
	diff["ticket_date"] = t.TicketDate.String()
	diff["ticket_number"] = t.TicketNumber
	diff["booking_id"] = t.BookingID
	var actor audit.Actor
	if p != nil {
		actor = p.Actor(reqctx.From(ctx).ClientIP)
	}
	if err := s.d.Audit.Record(ctx, tx, audit.Entry{
		EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: "ticket", EntityID: t.ID, Diff: diff,
	}); err != nil {
		return fmt.Errorf("ticketing: audit: %w", err)
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func userIDOf(p *auth.Principal) *string {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil
	}
	id := p.UserID
	return &id
}

// auditEntry is an audit entry of es by p.
func auditEntry(ctx context.Context, p *auth.Principal, es store.EstablishmentScope, action, entity, id string, diff map[string]any) audit.Entry {
	var actor audit.Actor
	if p != nil {
		actor = p.Actor(reqctx.From(ctx).ClientIP)
	}
	return audit.Entry{EstablishmentID: es.ID(), Actor: actor, Action: action, EntityType: entity, EntityID: id, Diff: diff}
}
