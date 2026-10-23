package ticketing

import (
	"fmt"

	"github.com/coste360/arenile/internal/store"
)

// State is the ticket state shown to the staff (api.TicketState).
type State string

const (
	StateAssigned   State = "assigned"
	StateUnassigned State = "unassigned"
	StateIssued     State = "issued"
	StateEntered    State = "entered"
	StateVoided     State = "voided"
)

// NumberLabel formats a daily number for the map and the receipt: at least
// three digits with a leading hash ("#007", "#037", "#1204").
func NumberLabel(n int64) string { return fmt.Sprintf("#%03d", n) }

// StateOf derives the shown state of a ticket: voided wins, then entered,
// then the assignment of a sale; an entry not yet used is issued.
func StateOf(t *store.Ticket) State {
	switch {
	case t.Status == store.TicketVoided:
		return StateVoided
	case t.EnteredAt != nil:
		return StateEntered
	case t.Kind == store.TicketKindEntry:
		return StateIssued
	case t.NeedsAssignment:
		return StateUnassigned
	default:
		return StateAssigned
	}
}

// MapLabel is the occupant label of a sold spot on the day map: the holder
// name when given, otherwise the number label.
func MapLabel(t *store.Ticket) string {
	if t.HolderName != "" {
		return t.HolderName
	}
	return NumberLabel(t.TicketNumber)
}
