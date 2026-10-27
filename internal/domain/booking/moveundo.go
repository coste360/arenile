package booking

import (
	"context"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Undo of the last move (spec 30.7): store.MoveUndoEntry rows written by
// Move/MoveThen, Swap and GroupMove in their transaction, undoable by the
// same user for store.MoveUndoWindow if the original spots are still free
// and the bookings did not change since (ErrMoveUndoConflict otherwise,
// ErrMoveUndoExpired after the window). Owner: the moves builder.

// UndoInfo is api.MoveUndoInfo.
type UndoInfo struct {
	MoveID     string
	Kind       string // store.MoveUndoMove | MoveUndoSwap | MoveUndoGroup
	ExpiresAt  time.Time
	BookingIDs []string
}

// LastMove returns the last undoable move of the principal (nil if none).
// Requires auth.PermBookingsWrite.
func (s *Service) LastMove(ctx context.Context, p *auth.Principal, establishmentID string) (*UndoInfo, error) {
	_, _, _ = ctx, p, establishmentID
	return nil, configerr.ErrNotImplemented
}

// UndoMove puts the lines of a move back on their original spots in one
// transaction (audit and event per booking) and returns the bookings.
// Requires auth.PermBookingsWrite.
func (s *Service) UndoMove(ctx context.Context, p *auth.Principal, establishmentID, moveID string) ([]*Detail, error) {
	_, _, _, _ = ctx, p, establishmentID, moveID
	return nil, configerr.ErrNotImplemented
}
