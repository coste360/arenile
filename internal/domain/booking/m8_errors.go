package booking

import (
	"errors"

	"github.com/coste360/arenile/internal/domain/pricingconfig"
)

// M8 sentinel errors (spec 30, ADR 0028), mapped to problem types in
// internal/http/problem/m8.go.
var (
	// ErrSwapConflict: the two lines cannot be swapped (no common interval
	// from the chosen date, incompatible spots or packs) -> 409 swap_conflict.
	ErrSwapConflict = errors.New("booking: swap not possible")
	// ErrMoveUndoExpired: the 5 minute undo window is over -> 410.
	ErrMoveUndoExpired = errors.New("booking: move undo expired")
	// ErrMoveUndoConflict: the original spots are no longer free or the
	// bookings changed after the move -> 409 move_undo_conflict.
	ErrMoveUndoConflict = errors.New("booking: move cannot be undone")
	// ErrAgreementNotApplicable: the agreement is inactive, outside its
	// validity, not for this channel or customer -> 422.
	ErrAgreementNotApplicable = pricingconfig.ErrAgreementNotApplicable
	// ErrRequirementNotVerified: the agreement requires a verification that
	// was not recorded (only where a flow demands it) -> 422.
	ErrRequirementNotVerified = errors.New("booking: agreement requirement not verified")
)
