// Package ticketassign places the zone-first tickets of the daily sale
// ("da assegnare", spec 14.3, ADR 0020) on a spot: manual assignment from the
// day map, later moves, and the automatic fill of the free spots in order.
//
// # Invariants
//
//   - Every placement is a booking Move (internal/domain/booking): release of
//     the zone pool usage (zone mode) or of the provisional spot (seat mode)
//     and allocation of the new spot in one transaction, audit log and outbox
//     event included. The unique key of spot_allocations decides who wins a
//     spot; a loser gets ticketing.ErrAssignmentConflict (409).
//   - The ticket row changes in the same transaction as the Move
//     (booking.MoveThen) with a conditional update, so a ticket is never
//     assigned twice and a voided ticket is never moved.
//   - Map and SSE never carry personal data: the outbox events of the Move
//     are the existing booking/allocation events (ids only).
package ticketassign

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/store"
)

// Bookings performs the Move of the ticket's line with the ticket update in
// the same transaction. Implemented by *booking.Service.
type Bookings interface {
	MoveThen(ctx context.Context, p *auth.Principal, establishmentID, id string, in booking.MoveInput, then booking.MoveHook) (*booking.ChangeResult, error)
}

// Deps are the collaborators of the service.
type Deps struct {
	DB       bun.IDB
	Clock    clock.Clock
	Bookings Bookings
}

// Service assigns and moves tickets.
type Service struct{ d Deps }

// NewService builds the service.
func NewService(d Deps) *Service { return &Service{d: d} }

// Input places one ticket on a spot. ExpectedVersion is the ticket version
// (optional optimistic check).
type Input struct {
	SpotID          string
	ExpectedVersion *int64
}

// AutoInput selects the tickets of an automatic assignment.
type AutoInput struct {
	Date   calendar.Date
	ZoneID string
	DryRun bool
}

// Unassigned reasons of AutoResult.
const (
	ReasonNoFreeSpot = "no_free_spot"
	ReasonConflict   = "conflict"
)

// AutoAssigned is one ticket placed by the automatic assignment.
type AutoAssigned struct {
	TicketID     string
	TicketNumber int64
	SpotID       string
	SpotLabel    string
}

// AutoUnassigned is one ticket left to assign, with the reason.
type AutoUnassigned struct {
	TicketID     string
	TicketNumber int64
	Reason       string
}

// AutoResult is the outcome of AutoAssign.
type AutoResult struct {
	Applied    bool
	Assigned   []AutoAssigned
	Unassigned []AutoUnassigned
}

func (s *Service) now() time.Time { return s.d.Clock.Now().UTC() }

// ListUnassigned returns the valid tickets of date still to assign, by
// number, optionally of one zone. Requires auth.PermBookingsRead.
func (s *Service) ListUnassigned(ctx context.Context, p *auth.Principal, establishmentID string, date calendar.Date, zoneID string) ([]ticketing.View, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return nil, err
	}
	if date.IsZero() {
		return nil, configerr.Invalid("date", "Campo obbligatorio.")
	}
	if zoneID != "" {
		if _, err := uuid.Parse(zoneID); err != nil {
			return nil, configerr.Invalid("zone_id", "Zona non valida.")
		}
	}
	ts, err := store.ListUnassignedTicketRows(ctx, s.d.DB, es, date, zoneID)
	if err != nil {
		return nil, err
	}
	return ticketing.LoadViews(ctx, s.d.DB, es, ts)
}

// Assign places a ticket still to assign on a free spot of its zone. A spot
// already occupied is spot_unavailable; a spot taken between the check and
// the allocation is ticketing.ErrAssignmentConflict. Requires
// auth.PermBookingsWrite.
func (s *Service) Assign(ctx context.Context, p *auth.Principal, establishmentID, ticketID string, in Input) (*ticketing.View, error) {
	return s.place(ctx, p, establishmentID, ticketID, in, true)
}

// Move places a ticket on another spot (any zone): for an assigned ticket it
// is a move, for one still to assign it is an assignment. Requires
// auth.PermBookingsWrite.
func (s *Service) Move(ctx context.Context, p *auth.Principal, establishmentID, ticketID string, in Input) (*ticketing.View, error) {
	return s.place(ctx, p, establishmentID, ticketID, in, false)
}

// target is a loaded ticket with its booking and line.
type target struct {
	es     store.EstablishmentScope
	ticket *store.Ticket
	bk     *store.Booking
	line   *store.BookingLine
}

func (s *Service) load(ctx context.Context, p *auth.Principal, establishmentID, ticketID string) (*target, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(ticketID); err != nil {
		return nil, ticketing.ErrNotFound
	}
	t, err := store.GetTicketForAssignment(ctx, s.d.DB, es, ticketID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ticketing.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.loadFor(ctx, es, t)
}

func (s *Service) loadFor(ctx context.Context, es store.EstablishmentScope, t *store.Ticket) (*target, error) {
	switch {
	case t.Status == store.TicketVoided:
		return nil, ticketing.ErrVoided
	case t.Kind != store.TicketKindSale:
		return nil, configerr.Invalid("ticket_id", "Un biglietto d'ingresso non occupa una postazione.")
	}
	bk, err := store.GetBooking(ctx, s.d.DB, es, t.BookingID)
	if err != nil {
		return nil, fmt.Errorf("ticketassign: booking: %w", err)
	}
	lines, err := store.ListBookingLines(ctx, s.d.DB, es, t.BookingID)
	if err != nil {
		return nil, fmt.Errorf("ticketassign: lines: %w", err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("ticketassign: ticket %s: booking without lines", t.ID)
	}
	// A daily sale has one line; prefer the one of the ticket's zone.
	line := &lines[0]
	for i := range lines {
		if lines[i].ZoneID == t.ZoneID {
			line = &lines[i]
			break
		}
	}
	return &target{es: es, ticket: t, bk: bk, line: line}, nil
}

func (s *Service) place(ctx context.Context, p *auth.Principal, establishmentID, ticketID string, in Input, assignOnly bool) (*ticketing.View, error) {
	if _, err := uuid.Parse(in.SpotID); err != nil {
		return nil, configerr.Invalid("spot_id", "Postazione non valida.")
	}
	tg, err := s.load(ctx, p, establishmentID, ticketID)
	if err != nil {
		return nil, err
	}
	if err := configerr.CheckVersion(in.ExpectedVersion, tg.ticket.Version); err != nil {
		return nil, err
	}
	if assignOnly && !tg.ticket.NeedsAssignment {
		return nil, fmt.Errorf("%w: ticket already assigned", ticketing.ErrAssignmentConflict)
	}
	sp, err := store.GetSpotForAssignment(ctx, s.d.DB, tg.es, in.SpotID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, configerr.Invalid("spot_id", "Postazione inesistente.")
	}
	if err != nil {
		return nil, err
	}
	if !sp.Active {
		return nil, configerr.Invalid("spot_id", "Postazione non attiva.")
	}
	if assignOnly && sp.ZoneID != tg.ticket.ZoneID {
		return nil, configerr.Invalid("spot_id", "La postazione non appartiene alla zona del biglietto.")
	}
	if err := s.checkFree(ctx, tg, sp.ID); err != nil {
		return nil, err
	}
	if err := s.apply(ctx, p, establishmentID, tg, sp); err != nil {
		return nil, err
	}
	t, err := store.GetTicketForAssignment(ctx, s.d.DB, tg.es, tg.ticket.ID)
	if err != nil {
		return nil, err
	}
	vs, err := ticketing.LoadViews(ctx, s.d.DB, tg.es, []store.Ticket{*t})
	if err != nil {
		return nil, err
	}
	return &vs[0], nil
}

// checkFree reports a spot already occupied (by someone else) on a half-day
// of the stay as *availability.SpotUnavailableError. It only classifies the
// error for the staff: the unique key of spot_allocations stays the guard.
func (s *Service) checkFree(ctx context.Context, tg *target, spotID string) error {
	occ, err := store.OccupiedHalfDays(ctx, s.d.DB, tg.es, []string{spotID}, tg.bk.StartsOn, tg.bk.EndsOn, s.now())
	if err != nil {
		return err
	}
	want := availability.Interval{From: tg.bk.StartsOn, To: tg.bk.EndsOn, Slots: tg.bk.Slots}.DateSlots()
	var conflicts []availability.DateSlot
	for _, o := range occ {
		if o.BookingID != nil && *o.BookingID == tg.bk.ID {
			continue
		}
		ds := availability.DateSlot{Date: o.Date, Slot: o.Slot}
		if slices.Contains(want, ds) {
			conflicts = append(conflicts, ds)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	id, _ := uuid.Parse(spotID)
	return &availability.SpotUnavailableError{SpotID: id, Conflicts: conflicts}
}

// apply runs the Move with the ticket update in the same transaction.
func (s *Service) apply(ctx context.Context, p *auth.Principal, establishmentID string, tg *target, sp *store.Spot) error {
	wasUnassigned := tg.ticket.NeedsAssignment
	// KeepPrice: the ticket was paid at the desk, so the placement never
	// changes the booking total (the engine price of the new spot is only
	// recorded in the audit diff, ADR 0020).
	_, err := s.d.Bookings.MoveThen(ctx, p, establishmentID, tg.bk.ID, booking.MoveInput{LineID: tg.line.ID, SpotID: sp.ID, KeepPrice: true},
		func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, _ *store.Booking) error {
			mark := store.MarkTicketMoved
			if wasUnassigned {
				mark = store.MarkTicketAssigned
			}
			ok, err := mark(ctx, tx, es, tg.ticket.ID, sp.ZoneID, s.now())
			if err != nil {
				return err
			}
			if !ok {
				cur, err := store.GetTicketForAssignment(ctx, tx, es, tg.ticket.ID)
				if err == nil && cur.Status == store.TicketVoided {
					return ticketing.ErrVoided
				}
				return fmt.Errorf("%w: ticket changed meanwhile", ticketing.ErrAssignmentConflict)
			}
			return nil
		})
	return classify(err)
}

// classify turns an allocation refused by the database into an assignment
// conflict (the spot was taken meanwhile); other errors pass through.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var ce *availability.ConflictError
	if errors.As(err, &ce) || errors.Is(err, availability.ErrSpotUnavailable) {
		return fmt.Errorf("%w: %w", ticketing.ErrAssignmentConflict, err)
	}
	// The booking was cancelled between the ticket read and the Move: the
	// ticket was voided meanwhile (the only cancellation of a ticket booking).
	if errors.Is(err, booking.ErrNotModifiable) {
		return fmt.Errorf("%w: %w", ticketing.ErrVoided, err)
	}
	return err
}
