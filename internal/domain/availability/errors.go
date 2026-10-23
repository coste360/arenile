package availability

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

// Sentinel errors of the availability domain.
var (
	// ErrSpotUnavailable: the spot is already allocated on at least one of the
	// requested half-days. Returned as *SpotUnavailableError.
	ErrSpotUnavailable = errors.New("availability: spot unavailable")
	// ErrPoolExhausted: a pool (or the implicit pool of a zone-mode zone) has
	// not enough capacity on at least one requested half-day.
	ErrPoolExhausted = errors.New("availability: pool exhausted")
	// ErrHoldExpired: the hold of a booking expired before confirmation.
	ErrHoldExpired = errors.New("availability: hold expired")
	// ErrQuotaExceeded: the online quota of a zone (spec 7.6) does not allow
	// the requested quantity on at least one half-day. The desk ignores it.
	ErrQuotaExceeded = errors.New("availability: online quota exceeded")
)

// Conflict reasons (Conflict.Reason), mirrored by the API AllocationConflict.
const (
	ReasonSpotUnavailable = "spot_unavailable"
	ReasonPoolExhausted   = "pool_exhausted"
	ReasonQuotaExceeded   = "quota_exceeded"
	ReasonClosed          = "closed"
	ReasonInactive        = "inactive"
)

// Conflict is one half-day on which a resource cannot be allocated. Exactly
// one of SpotID, PoolID or ZoneID identifies the resource.
type Conflict struct {
	SpotID string
	PoolID string
	ZoneID string
	Date   calendar.Date
	Slot   calendar.Slot
	Reason string
}

// ConflictError is returned by the DB-backed allocation (internal/domain/
// inventory) when constraints refuse an allocation: Err is
// ErrSpotUnavailable, ErrPoolExhausted or ErrQuotaExceeded and Conflicts
// lists every refused half-day, so that the panel can propose alternatives
// (spec 7.2). errors.Is(err, Err) holds.
type ConflictError struct {
	Err       error
	Conflicts []Conflict
}

func (e *ConflictError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		id := c.SpotID + c.PoolID + c.ZoneID
		parts[i] = id + " " + c.Date.String() + " " + string(c.Slot)
	}
	return fmt.Sprintf("%v [%s]", e.Err, strings.Join(parts, ", "))
}

// Unwrap returns the sentinel kind.
func (e *ConflictError) Unwrap() error { return e.Err }

// SpotUnavailableError lists the half-days on which a spot is not available,
// so that the UI can propose alternatives (spec 7.2).
// errors.Is(err, ErrSpotUnavailable) is true.
type SpotUnavailableError struct {
	SpotID    uuid.UUID
	Conflicts []DateSlot
}

func (e *SpotUnavailableError) Error() string {
	parts := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		parts[i] = c.Date.String() + " " + string(c.Slot)
	}
	return fmt.Sprintf("availability: spot %s unavailable on [%s]", e.SpotID, strings.Join(parts, ", "))
}

// Is makes errors.Is(err, ErrSpotUnavailable) succeed.
func (e *SpotUnavailableError) Is(target error) bool { return target == ErrSpotUnavailable }
