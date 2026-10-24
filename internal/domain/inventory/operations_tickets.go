package inventory

import (
	"context"
	"fmt"

	"github.com/coste360/arenile/internal/store"
)

// OperationalTicket is the daily-sale ticket of a booking on the day map
// (spec 14.3, ADR 0020). Admin API only: SSE events never carry it.
type OperationalTicket struct {
	// ID and Version let the map move the ticket with the ticket actions
	// (If-Match on the ticket version) instead of the generic booking move.
	ID     string
	Number int64
	// Label is the number label ("#037"), same format as
	// ticketing.NumberLabel.
	Label string
	// NeedsAssignment: zone-first ticket not yet placed (a seat-mode zone
	// shows it on its provisional spot).
	NeedsAssignment bool
	Version         int64
}

// TicketNumberLabel formats a daily ticket number ("#007", "#037", "#1204").
// It mirrors ticketing.NumberLabel (inventory cannot import ticketing).
func TicketNumberLabel(n int64) string { return fmt.Sprintf("#%03d", n) }

// TicketOccupantLabel is the map label of a ticket booking: the holder name
// when given, otherwise the number label. Walk-in sales have no customer
// name worth showing, so the ticket wins over the customer's surname.
func TicketOccupantLabel(holderName string, number int64) string {
	if holderName != "" {
		return holderName
	}
	return TicketNumberLabel(number)
}

// applyTicketMarks adds the ticket of each daily-sale booking to the spots
// and pending assignments of day and replaces the occupant label with the
// ticket label (holder name or "#037").
func (s *Service) applyTicketMarks(ctx context.Context, es store.EstablishmentScope, day *OperationsDay) error {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for i := range day.Spots {
		if b := day.Spots[i].Booking; b != nil {
			add(b.BookingID)
		}
	}
	for i := range day.PendingAssignments {
		add(day.PendingAssignments[i].BookingID)
	}
	if len(ids) == 0 {
		return nil
	}
	marks, err := store.TicketMapMarks(ctx, s.d.DB, es, ids)
	if err != nil {
		return err
	}
	if len(marks) == 0 {
		return nil
	}
	byBooking := make(map[string]store.TicketMapMark, len(marks))
	for _, m := range marks {
		byBooking[m.BookingID] = m
	}
	ticket := func(m store.TicketMapMark) *OperationalTicket {
		return &OperationalTicket{ID: m.ID, Number: m.TicketNumber, Label: TicketNumberLabel(m.TicketNumber), NeedsAssignment: m.NeedsAssignment, Version: m.Version}
	}
	for i := range day.Spots {
		b := day.Spots[i].Booking
		if b == nil {
			continue
		}
		if m, ok := byBooking[b.BookingID]; ok {
			b.Ticket = ticket(m)
			// An anonymised customer keeps only the number (no name).
			if b.CustomerAnonymized {
				b.OccupantLabel = TicketNumberLabel(m.TicketNumber)
			} else {
				b.OccupantLabel = TicketOccupantLabel(m.HolderName, m.TicketNumber)
			}
			// The walk-in customer ("Cliente al banco") is shared by every
			// sale: the accessible name and the tooltip show the ticket too.
			b.CustomerDisplayName = b.OccupantLabel
		}
	}
	for i := range day.PendingAssignments {
		pa := &day.PendingAssignments[i]
		if m, ok := byBooking[pa.BookingID]; ok {
			pa.Ticket = ticket(m)
			pa.CustomerDisplayName = TicketOccupantLabel(m.HolderName, m.TicketNumber)
		}
	}
	return nil
}
