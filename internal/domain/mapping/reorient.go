package mapping

import (
	"context"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Transform is the geometry change that moves the sea to another side
// (API enum MapTransform).
type Transform string

// Transforms of ReorientTransform.
const (
	// TransformNone: same side, nothing to do.
	TransformNone Transform = "none"
	// TransformMirrorY: top <-> bottom, y' = height - y, rotation r' = -r;
	// the order along the shore is unchanged.
	TransformMirrorY Transform = "mirror_y"
	// TransformMirrorX: left <-> right, x' = width - x, rotation r' = -r.
	TransformMirrorX Transform = "mirror_x"
	// TransformRotateCW: 90 degrees clockwise (top -> right -> bottom ->
	// left): (x, y) -> (height - y, x), width and height swapped, r' = r + 90.
	TransformRotateCW Transform = "rotate_cw"
	// TransformRotateCCW: 90 degrees counter-clockwise: (x, y) -> (y,
	// width - x), width and height swapped, r' = r - 90.
	TransformRotateCCW Transform = "rotate_ccw"
)

// ReorientTransform returns the transform that moves the sea from one side
// to another (pure). Opposite sides mirror, adjacent sides rotate.
func ReorientTransform(from, to string) Transform {
	if from == to {
		return TransformNone
	}
	order := map[string]int{"top": 0, "right": 1, "bottom": 2, "left": 3}
	f, okF := order[from]
	t, okT := order[to]
	if !okF || !okT {
		return TransformNone
	}
	switch (t - f + 4) % 4 {
	case 1:
		return TransformRotateCW
	case 3:
		return TransformRotateCCW
	}
	if from == "top" || from == "bottom" {
		return TransformMirrorY
	}
	return TransformMirrorX
}

// ReorientInput asks to move the sea side of a map.
type ReorientInput struct {
	SeaSide string
	// DryRun computes the preview without saving (no 409, counts only).
	DryRun bool
	// AcknowledgeFutureAllocations confirms the change when spots of the
	// map have allocations from today on (otherwise
	// configerr.SpotsAllocatedError -> 409 spot_has_future_allocations).
	AcknowledgeFutureAllocations bool
	IP                           string
}

// ReorientResult is the transformed document and what was done.
type ReorientResult struct {
	// Document is the saved document (or the preview with DryRun).
	Document  *Document
	From, To  string
	Transform Transform
	Applied   bool
	// FutureAllocations counts booking, season and block allocations from
	// today on (establishment timezone) on the spots of the map.
	FutureAllocations          int
	SpotsWithFutureAllocations int
}

// ReorientMap moves the sea side of a map and transforms the whole layout
// (spots, decorations, walkway tiles) so that every row keeps its distance
// from the sea. Geometry only: ids, labels, rows, zones and allocations are
// unchanged. One versioned save: expected must match the map version
// (configerr.VersionConflictError otherwise); the version is bumped once,
// the audit log records "map.reoriented" and the outbox gets
// config.map_changed with Change event.ChangeReoriented, in one
// transaction. Permission auth.PermEstablishmentConfigure.
//
// Implementation note: apply the transform to the SaveInput built by
// currentInput and persist it through applyDocument, so that validation
// (inside the map bounds, kit kinds) is the same as SaveMap. Spot x/y are
// the centre of the shape (web/shared/src/map/types.ts; check decorations
// too), so the formulas above apply to the centre; normalise rotations to
// [0, 360) and snap to the grid only if the result is off-grid by rounding.
//
// Owner: map sea-side builder (M5A).
func (s *Service) ReorientMap(ctx context.Context, p *auth.Principal, establishmentID, mapID string, expected int64, in ReorientInput) (*ReorientResult, error) {
	return nil, configerr.ErrNotImplemented
}
