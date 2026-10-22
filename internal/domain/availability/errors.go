package availability

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
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
)

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
