package booking

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// Duplicate (spec 30.9): a new booking with the same spots, customer,
// extras, agreement and group data on new dates, priced again and
// allocated through the normal create path (DB constraints, audit, outbox).
// Owner: the moves builder.

// DuplicateInput is api.BookingDuplicateRequest.
type DuplicateInput struct {
	StartsOn calendar.Date
	EndsOn   calendar.Date
	Slots    *calendar.Slots
	Formula  *string
	Status   *string // draft | held | confirmed (default confirmed)
	DryRun   bool
}

// DuplicateResult is api.BookingDuplicateResult.
type DuplicateResult struct {
	Applied   bool
	Detail    *Detail // nil with DryRun
	Total     money.Cents
	Conflicts []availability.Conflict
}

// Duplicate copies booking id on new dates. Requires auth.PermBookingsWrite.
func (s *Service) Duplicate(ctx context.Context, p *auth.Principal, establishmentID, id string, in DuplicateInput) (*DuplicateResult, error) {
	_, _, _, _, _ = ctx, p, establishmentID, id, in
	return nil, configerr.ErrNotImplemented
}
