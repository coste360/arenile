package booking

import (
	"errors"
	"fmt"
)

// Status is the lifecycle state of a booking (spec 10.2).
type Status string

// Booking statuses.
const (
	StatusDraft            Status = "draft"
	StatusHeld             Status = "held"
	StatusPendingPayment   Status = "pending_payment"
	StatusAwaitingApproval Status = "awaiting_approval"
	StatusConfirmed        Status = "confirmed"
	StatusCheckedIn        Status = "checked_in"
	StatusCompleted        Status = "completed"
	StatusCancelled        Status = "cancelled"
	StatusNoShow           Status = "no_show"
	StatusExpired          Status = "expired"
	StatusRejected         Status = "rejected"
)

// AllStatuses lists every status in lifecycle order.
func AllStatuses() []Status {
	return []Status{
		StatusDraft, StatusHeld, StatusPendingPayment, StatusAwaitingApproval, StatusConfirmed,
		StatusCheckedIn, StatusCompleted, StatusCancelled, StatusNoShow, StatusExpired, StatusRejected,
	}
}

// ErrInvalidTransition is returned when a status change is not allowed.
var ErrInvalidTransition = errors.New("booking: invalid status transition")

// ErrUnknownStatus is returned when parsing an unknown status string.
var ErrUnknownStatus = errors.New("booking: unknown status")

// TransitionError details a refused transition; errors.Is(err,
// ErrInvalidTransition) is true.
type TransitionError struct {
	From, To Status
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("booking: invalid status transition %s -> %s", e.From, e.To)
}

// Is makes errors.Is(err, ErrInvalidTransition) succeed.
func (e *TransitionError) Is(target error) bool { return target == ErrInvalidTransition }

// transitions is the allowed transition table. It contains the arrows of the
// diagram in spec 10.2 plus the edges implied by the surrounding text:
//
//   - draft -> held: the panel finalises a draft and occupies resources.
//   - draft -> cancelled: a draft is discarded (it holds no resources).
//   - held -> pending_payment: payment started.
//   - held -> confirmed: confirmation without payment (payment mode "none",
//     desk sale paid at the till, manual driver) — spec 10.1 step 5.
//   - held -> awaiting_approval: approval required and nothing to pay upfront.
//   - held -> expired: hold TTL elapsed.
//   - held -> cancelled: the customer or the operator abandons the hold.
//   - pending_payment -> confirmed: payment succeeded (webhook).
//   - pending_payment -> awaiting_approval: paid/authorised, approval needed.
//   - pending_payment -> expired: payment failed or timed out (max 30 min).
//   - awaiting_approval -> confirmed | rejected: manager decision.
//   - awaiting_approval -> expired: the long approval hold elapsed.
//   - awaiting_approval -> cancelled: the customer withdraws the request.
//   - confirmed -> checked_in | cancelled | no_show.
//   - confirmed -> completed: automatic closing at the end of the period when
//     no check-in was recorded (many establishments do not scan arrivals).
//   - checked_in -> completed: automatic closing at the end of the period.
//
// completed, cancelled, no_show, expired and rejected are terminal.
var transitions = map[Status][]Status{
	StatusDraft:            {StatusHeld, StatusCancelled},
	StatusHeld:             {StatusPendingPayment, StatusConfirmed, StatusAwaitingApproval, StatusExpired, StatusCancelled},
	StatusPendingPayment:   {StatusConfirmed, StatusAwaitingApproval, StatusExpired},
	StatusAwaitingApproval: {StatusConfirmed, StatusRejected, StatusExpired, StatusCancelled},
	StatusConfirmed:        {StatusCheckedIn, StatusCancelled, StatusNoShow, StatusCompleted},
	StatusCheckedIn:        {StatusCompleted},
}

// ParseStatus converts a string into a Status.
func ParseStatus(s string) (Status, error) {
	st := Status(s)
	if !st.Valid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownStatus, s)
	}
	return st, nil
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusHeld, StatusPendingPayment, StatusAwaitingApproval, StatusConfirmed,
		StatusCheckedIn, StatusCompleted, StatusCancelled, StatusNoShow, StatusExpired, StatusRejected:
		return true
	}
	return false
}

// IsTerminal reports whether no further transition is possible.
func (s Status) IsTerminal() bool {
	return s.Valid() && len(transitions[s]) == 0
}

// OccupiesResources reports whether bookings in this status hold allocations
// (spots or pool units), either temporarily (hold) or stably.
func (s Status) OccupiesResources() bool {
	switch s {
	case StatusHeld, StatusPendingPayment, StatusAwaitingApproval, StatusConfirmed, StatusCheckedIn:
		return true
	}
	return false
}

// IsHold reports whether the allocations of this status are temporary holds
// with an expiry (kind = 'hold').
func (s Status) IsHold() bool {
	return s == StatusHeld || s == StatusPendingPayment || s == StatusAwaitingApproval
}

// NextStatuses returns the statuses reachable from s in one step.
func (s Status) NextStatuses() []Status {
	return append([]Status(nil), transitions[s]...)
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to Status) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Transition returns nil if from -> to is allowed, otherwise a
// *TransitionError wrapping ErrInvalidTransition.
func Transition(from, to Status) error {
	if !CanTransition(from, to) {
		return &TransitionError{From: from, To: to}
	}
	return nil
}
