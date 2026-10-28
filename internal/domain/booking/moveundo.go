package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Undo of the last move (spec 30.7): store.MoveUndoEntry rows written by
// Move, Swap and GroupMove in their transaction, undoable by the same user
// for store.MoveUndoWindow if the original spots are still free and the
// bookings did not change since (ErrMoveUndoConflict otherwise,
// ErrMoveUndoExpired after the window). The undo is itself a re-placement
// (reassign.go): one transaction, bookings locked in id order, release
// then allocation of the original segments; the unique keys of
// spot_allocations decide whether the original spots are still free.

// UndoInfo is api.MoveUndoInfo.
type UndoInfo struct {
	MoveID     string
	Kind       string // store.MoveUndoMove | MoveUndoSwap | MoveUndoGroup
	ExpiresAt  time.Time
	BookingIDs []string
}

// actionMoveUndone is the audit action of an undo.
const actionMoveUndone = "booking.move_undone"

// undoPayload is the JSON of store.MoveUndoEntry.Payload.
type undoPayload struct {
	Lines []undoLine `json:"lines"`
	// Versions are the booking versions right after the move: an undo is
	// refused if a booking changed since.
	Versions map[string]int64 `json:"versions"`
}

func marshalUndo(p undoPayload) (store.JSON, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("booking: undo payload: %w", err)
	}
	return store.JSON(raw), nil
}

// LastMove returns the last undoable move of the principal (nil if none).
// Requires auth.PermBookingsWrite.
func (s *Service) LastMove(ctx context.Context, p *auth.Principal, establishmentID string) (*UndoInfo, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	if p.Kind != auth.PrincipalUser || p.UserID == "" {
		return nil, nil
	}
	e, err := store.LastMoveUndo(ctx, s.d.DB, es, p.UserID, s.now())
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return undoInfo(e), nil
}

// UndoMove puts the lines of a move back on their original spots in one
// transaction (audit and event per booking) and returns the bookings.
// Requires auth.PermBookingsWrite.
func (s *Service) UndoMove(ctx context.Context, p *auth.Principal, establishmentID, moveID string) ([]*Detail, error) {
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	e, err := store.GetMoveUndo(ctx, s.d.DB, es, moveID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Only the user who moved can undo: for anyone else the entry does not
	// exist.
	if p.Kind != auth.PrincipalUser || e.UserID == nil || *e.UserID != p.UserID {
		return nil, ErrNotFound
	}
	if e.UndoneAt != nil {
		return nil, ErrMoveUndoConflict
	}
	if !s.now().Before(e.ExpiresAt) {
		return nil, ErrMoveUndoExpired
	}
	var pl undoPayload
	if err := json.Unmarshal(e.Payload, &pl); err != nil {
		return nil, fmt.Errorf("booking: undo payload: %w", err)
	}
	spec := reassignSpec{Expected: map[string]*int64{}, Action: actionMoveUndone, Diff: map[string]any{"move_id": e.ID, "kind": e.Kind}}
	for id, v := range pl.Versions {
		spec.Expected[id] = &v
	}
	for _, l := range pl.Lines {
		if len(l.Before) == 0 {
			return nil, ErrMoveUndoConflict
		}
		spec.Changes = append(spec.Changes, lineChange{BookingID: l.BookingID, LineID: l.LineID, From: l.From, Segments: fromUndoSegments(l.Before)})
	}
	var out []*Detail
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		ok, err := store.MarkMoveUndone(ctx, tx, es, e.ID, s.now())
		if err != nil {
			return err
		}
		if !ok { // expired or undone meanwhile
			if !s.now().Before(e.ExpiresAt) {
				return ErrMoveUndoExpired
			}
			return ErrMoveUndoConflict
		}
		res, err := s.reassignInTx(ctx, tx, p, es, spec)
		if err != nil {
			return err
		}
		for _, id := range res.Order {
			out = append(out, res.Results[id].Detail)
		}
		return nil
	})
	switch {
	case err == nil:
		return out, nil
	case errors.Is(err, ErrMoveUndoExpired):
		return nil, err
	case errors.Is(err, configerr.ErrVersionConflict), errors.Is(err, ErrNotModifiable), errors.Is(err, ErrLineNotFound):
		return nil, fmt.Errorf("%w: %w", ErrMoveUndoConflict, err)
	}
	if _, ok := conflictsOf(err); ok {
		return nil, fmt.Errorf("%w: %w", ErrMoveUndoConflict, err)
	}
	return nil, err
}

// moveRecorded is moveLocked with the undo entry of the move written in
// the same transaction (Move from the panel; the ticket placement and the
// auto-assignment are not undoable). A line without a spot before the move
// (an assignment) gets no undo entry.
func (s *Service) moveRecorded(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking, in MoveInput, res *ChangeResult) (*outcome, error) {
	before, err := allocSegments(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	from := b.StartsOn
	if in.From != nil && in.From.After(b.StartsOn) {
		from = *in.From
	}
	o, err := s.moveLocked(ctx, tx, p, es, b, in, res)
	if err != nil || len(before[in.LineID]) == 0 {
		return o, err
	}
	prev := o.after
	o.after = func(ctx context.Context, tx bun.IDB, b *store.Booking) error {
		if prev != nil {
			if err := prev(ctx, tx, b); err != nil {
				return err
			}
		}
		after, err := allocSegments(ctx, tx, es, b)
		if err != nil {
			return err
		}
		l := undoLine{
			BookingID: b.ID, LineID: in.LineID, From: from,
			Before: toUndoSegments(clipFrom(before[in.LineID], from)), After: toUndoSegments(clipFrom(after[in.LineID], from)),
			SpotFrom: spotOn(before[in.LineID], from), SpotTo: in.SpotID,
		}
		res.Undo, err = s.recordUndo(ctx, tx, p, es, store.MoveUndoMove, []undoLine{l}, map[string]int64{b.ID: b.Version})
		return err
	}
	return o, nil
}

// lastMoveOf is the most recent undoable move touching booking id (any
// user; only its author can undo it), nil if none.
func lastMoveOf(ctx context.Context, db bun.IDB, es store.EstablishmentScope, id string, now time.Time) (*UndoInfo, error) {
	entries, err := store.ListActiveMoveUndos(ctx, db, es, now)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if slices.Contains(entries[i].BookingIDs, id) {
			return undoInfo(&entries[i]), nil
		}
	}
	return nil, nil
}
