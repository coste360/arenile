package mapping

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

// AllocationChecker tells which spots have allocations (bookings, holds,
// blocks, season passes) on or after a date. Spots with future allocations
// cannot be deleted from a map, only deactivated (spec 11.2).
//
// Allocations arrive with M3 (spot_allocations): until then the app wires
// NoAllocations. The M3 availability package will provide the real
// implementation backed by spot_allocations.
type AllocationChecker interface {
	SpotsWithFutureAllocations(ctx context.Context, es store.EstablishmentScope, spotIDs []string, from calendar.Date) ([]string, error)
}

// NoAllocations is the M1 AllocationChecker: no spot has allocations yet.
type NoAllocations struct{}

// SpotsWithFutureAllocations always returns no spots.
func (NoAllocations) SpotsWithFutureAllocations(context.Context, store.EstablishmentScope, []string, calendar.Date) ([]string, error) {
	return nil, nil
}
