package booking

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
)

// Sentinel errors of the booking service. They wrap the configerr kinds so
// that internal/http/problem maps them without knowing this package.
var (
	// ErrNotFound: unknown booking or outside the caller's establishment.
	ErrNotFound = fmt.Errorf("%w: booking", configerr.ErrNotFound)
	// ErrLineNotFound: the line does not belong to the booking.
	ErrLineNotFound = fmt.Errorf("%w: booking line", configerr.ErrNotFound)
	// ErrInvalidInitialStatus: a creation may start only as draft, held or confirmed.
	ErrInvalidInitialStatus = errors.New("booking: invalid initial status")
	// ErrNotModifiable: the booking is in a terminal status (cancelled,
	// completed...) and its dates, lines or customer cannot change.
	ErrNotModifiable = fmt.Errorf("%w: booking not modifiable in its status", configerr.ErrConflict)
	// ErrCodeExhausted: no free booking code after several attempts
	// (pathological contention); the request can be retried.
	ErrCodeExhausted = fmt.Errorf("%w: booking code contention, retry", configerr.ErrConflict)
	// ErrOutsidePeriod: a check-in or no-show requested on a day outside the
	// stay (establishment timezone); see OutsidePeriodError.
	ErrOutsidePeriod = errors.New("booking: outside the stay period")
)

// OutsidePeriodError reports a check-in (or no-show) refused because today,
// in the establishment timezone, is not a day of the stay: a check-in before
// starts_on would leave the booking impossible to cancel or mark as no-show
// (checked_in only moves to completed), a no-show before starts_on would
// release the whole stay and skip the policy refund of a cancellation.
// errors.Is(err, ErrOutsidePeriod) holds; the HTTP layer answers 422
// booking_outside_period with the period.
type OutsidePeriodError struct {
	Action   string // "check_in" or "no_show"
	Today    calendar.Date
	StartsOn calendar.Date
	EndsOn   calendar.Date
}

func (e *OutsidePeriodError) Error() string {
	return fmt.Sprintf("booking: %s on %s outside the stay %s..%s", e.Action, e.Today, e.StartsOn, e.EndsOn)
}

// Is makes errors.Is(err, ErrOutsidePeriod) succeed.
func (e *OutsidePeriodError) Is(target error) bool { return target == ErrOutsidePeriod }

// Outbox events written by the service, besides those of event (spec 18.1
// and ADR 0010).
const (
	eventCreated           = event.BookingCreated
	eventHeld              = event.BookingHeld
	eventConfirmed         = event.BookingConfirmed
	eventModified          = event.BookingModified
	eventCancelled         = event.BookingCancelled
	eventCheckedIn         = event.BookingCheckedIn
	eventNoShow            = event.BookingNoShow
	eventCompleted         = event.BookingCompleted
	eventApprovalRequested = event.BookingApprovalRequested
	eventRejected          = event.BookingRejected
	eventExpired           = event.BookingExpired
)

// Audit actions that are not status changes.
const (
	actionNoteAdded = "booking.note_added"
	actionMoved     = "booking.moved"
)
