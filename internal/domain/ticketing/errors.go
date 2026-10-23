package ticketing

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// Sentinel errors of the ticketing and daily sale services; internal/http/problem
// maps them to the problem types of api/openapi.yaml.
var (
	// ErrNotFound: unknown ticket or outside the caller's establishment.
	ErrNotFound = fmt.Errorf("%w: ticket", configerr.ErrNotFound)
	// ErrEntryTypeNotFound: unknown entry ticket type.
	ErrEntryTypeNotFound = fmt.Errorf("%w: entry ticket type", configerr.ErrNotFound)
	// ErrZoneFull: no capacity left in the zone for the slot (409 zone_full).
	ErrZoneFull = errors.New("ticketing: zone full")
	// ErrAssignmentConflict: the spot was taken meanwhile (409 assignment_conflict).
	ErrAssignmentConflict = errors.New("ticketing: assignment conflict")
	// ErrNumberConflict: the unique daily number was hit; never expected
	// with the atomic counter (409 ticket_number_conflict, retryable).
	ErrNumberConflict = errors.New("ticketing: ticket number conflict")
	// ErrVoided: the ticket is already voided (409 ticket_voided).
	ErrVoided = errors.New("ticketing: ticket voided")
	// ErrAlreadyEntered: the entry ticket was already used (409 ticket_already_entered).
	ErrAlreadyEntered = errors.New("ticketing: ticket already entered")
	// ErrPriceChanged: expected_total differs from the computed price (409 price_changed).
	ErrPriceChanged = errors.New("ticketing: price changed")
)
