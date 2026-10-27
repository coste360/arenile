package booking

import (
	"context"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
)

// Swap and group move (spec 30.7, ADR 0028, INV-101). Owner: the moves
// builder. Both run in ONE transaction: the bookings are locked in id
// order, the allocations of the lines from From are released and
// re-created on the other spot (the unique keys of spot_allocations make a
// double allocation impossible at any instant), the price is re-quoted when
// the price category changes, and the audit entry and the booking.modified
// outbox event are written for every booking. A move_undo_log row is
// written for the 5 minute undo (moveundo.go).

// SwapSide is one line of a swap.
type SwapSide struct {
	BookingID       string
	LineID          string
	ExpectedVersion *int64
}

// SwapInput is api.BookingSwapRequest.
type SwapInput struct {
	A, B   SwapSide
	From   calendar.Date
	DryRun bool
}

// SwapOutcome is the result of a swap for one booking.
type SwapOutcome struct {
	Detail        *Detail
	FromSpotID    string
	ToSpotID      string
	PreviousTotal money.Cents
	NewTotal      money.Cents
}

// SwapResult is api.BookingSwapResult.
type SwapResult struct {
	Applied   bool
	From, To  calendar.Date
	A, B      SwapOutcome
	Conflicts []availability.Conflict
	Undo      *UndoInfo
}

// GroupMoveItem moves one line to a spot.
type GroupMoveItem struct {
	BookingID       string
	LineID          string
	SpotID          string
	ExpectedVersion *int64
}

// GroupMoveInput is api.GroupMoveRequest.
type GroupMoveInput struct {
	Moves  []GroupMoveItem
	From   *calendar.Date
	DryRun bool
}

// GroupMoveResult is api.GroupMoveResult.
type GroupMoveResult struct {
	Applied   bool
	Results   []ChangeResult
	Conflicts []availability.Conflict
	Undo      *UndoInfo
}

// Swap exchanges the spots of two booking lines on their common interval
// from in.From. Requires auth.PermBookingsWrite.
func (s *Service) Swap(ctx context.Context, p *auth.Principal, establishmentID string, in SwapInput) (*SwapResult, error) {
	_, _, _, _ = ctx, p, establishmentID, in
	return nil, configerr.ErrNotImplemented
}

// GroupMove moves several lines at once (all or nothing). Requires
// auth.PermBookingsWrite.
func (s *Service) GroupMove(ctx context.Context, p *auth.Principal, establishmentID string, in GroupMoveInput) (*GroupMoveResult, error) {
	_, _, _, _ = ctx, p, establishmentID, in
	return nil, configerr.ErrNotImplemented
}
