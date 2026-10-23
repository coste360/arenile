package booking

import (
	"errors"
	"fmt"

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
)

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
