package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
)

// Ticket assignment queries (zone-first sales, ADR 0020). The ticket row is
// only changed together with the booking Move that places its line, in the
// same transaction (internal/domain/ticketassign); the conditional updates
// below are the final guard against assigning a ticket twice or touching a
// voided one.

// TicketMapMark is what the day map shows of the ticket of a booking: the
// daily number, the optional holder name and whether the ticket is still to
// be assigned (seat-mode provisional spot).
type TicketMapMark struct {
	ID              string `bun:"id"`
	BookingID       string `bun:"booking_id"`
	TicketNumber    int64  `bun:"ticket_number"`
	HolderName      string `bun:"holder_name"`
	NeedsAssignment bool   `bun:"needs_assignment"`
	Version         int64  `bun:"version"`
}

// TicketMapMarks returns the marks of the valid tickets owning bookingIDs.
func TicketMapMarks(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) ([]TicketMapMark, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []TicketMapMark{}
	if len(bookingIDs) == 0 {
		return out, nil
	}
	err := db.NewSelect().TableExpr("tickets AS tk").
		Column("tk.id", "tk.booking_id", "tk.ticket_number", "tk.holder_name", "tk.needs_assignment", "tk.version").
		Where("tk.establishment_id = ?", es.ID()).Where("tk.status = ?", TicketValid).
		Where("tk.booking_id IN (?)", bun.List(bookingIDs)).
		Scan(ctx, &out)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: ticket map marks: %w", err)
	}
	return out, nil
}

// TicketOwnsBooking reports whether a ticket (valid or voided) owns the
// booking: its generic cancel, modify and move are refused by the booking
// service (booking.ErrTicketOwned, ADR 0020).
func TicketOwnsBooking(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingID string) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	ok, err := db.NewSelect().Model((*Ticket)(nil)).
		Where("tk.establishment_id = ?", es.ID()).Where("tk.booking_id = ?", bookingID).Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("store: ticket owns booking: %w", err)
	}
	return ok, nil
}

// TicketOwnedBookingIDs returns, among bookingIDs, those owned by a ticket.
func TicketOwnedBookingIDs(ctx context.Context, db bun.IDB, es EstablishmentScope, bookingIDs []string) (map[string]bool, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	if len(bookingIDs) == 0 {
		return out, nil
	}
	var ids []string
	err := db.NewSelect().Model((*Ticket)(nil)).Column("tk.booking_id").
		Where("tk.establishment_id = ?", es.ID()).Where("tk.booking_id IN (?)", bun.List(bookingIDs)).
		Scan(ctx, &ids)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: ticket owned bookings: %w", err)
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ListUnassignedTicketRows returns the valid tickets of date still to be
// assigned, optionally of one zone, by ascending number.
func ListUnassignedTicketRows(ctx context.Context, db bun.IDB, es EstablishmentScope, date calendar.Date, zoneID string) ([]Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	out := []Ticket{}
	q := db.NewSelect().Model(&out).
		Where("tk.establishment_id = ?", es.ID()).Where("tk.ticket_date = ?", date).
		Where("tk.status = ?", TicketValid).Where("tk.needs_assignment = ?", true).
		OrderExpr("tk.ticket_number ASC")
	if zoneID != "" {
		q = q.Where("tk.zone_id = ?", zoneID)
	}
	if err := q.Scan(ctx); err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("store: unassigned tickets: %w", err)
	}
	return out, nil
}

// GetTicketForAssignment returns ticket id of es (ErrNotFound otherwise).
func GetTicketForAssignment(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Ticket, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	t := new(Ticket)
	err := db.NewSelect().Model(t).Where("tk.id = ?", id).Where("tk.establishment_id = ?", es.ID()).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: ticket: %w", err)
	}
	return t, nil
}

// MarkTicketAssigned clears the "da assegnare" state of a valid ticket and
// records the zone of its new spot. It reports false when the ticket is no
// longer valid and unassigned (assigned or voided meanwhile).
func MarkTicketAssigned(ctx context.Context, db bun.IDB, es EstablishmentScope, id, zoneID string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*Ticket)(nil)).
		Set("needs_assignment = ?", false).Set("provisional_spot_id = NULL").
		Set("zone_id = ?", zoneID).Set("version = version + 1").Set("updated_at = ?", now.UTC()).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("status = ?", TicketValid).Where("needs_assignment = ?", true).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: mark ticket assigned: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// MarkTicketMoved records the zone of the new spot of an assigned valid
// ticket. It reports false when the ticket is voided or still unassigned.
func MarkTicketMoved(ctx context.Context, db bun.IDB, es EstablishmentScope, id, zoneID string, now time.Time) (bool, error) {
	if err := es.mustBeValid(); err != nil {
		return false, err
	}
	res, err := db.NewUpdate().Model((*Ticket)(nil)).
		Set("zone_id = ?", zoneID).Set("version = version + 1").Set("updated_at = ?", now.UTC()).
		Where("id = ?", id).Where("establishment_id = ?", es.ID()).
		Where("status = ?", TicketValid).Where("needs_assignment = ?", false).
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("store: mark ticket moved: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// GetSpotForAssignment returns spot id of es (ErrNotFound otherwise).
func GetSpotForAssignment(ctx context.Context, db bun.IDB, es EstablishmentScope, id string) (*Spot, error) {
	if err := es.mustBeValid(); err != nil {
		return nil, err
	}
	sp := new(Spot)
	err := db.NewSelect().Model(sp).Where("sp.id = ?", id).Where("sp.establishment_id = ?", es.ID()).Scan(ctx)
	if isNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: spot: %w", err)
	}
	return sp, nil
}
