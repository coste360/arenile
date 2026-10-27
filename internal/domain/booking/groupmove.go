package booking

import (
	"context"
	"fmt"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Group move (spec 30.7): several lines, of one or more bookings, moved
// together to the spots chosen by the panel (which keeps the relative
// layout), all or nothing, in one transaction (reassign.go, INV-101).

// MaxGroupMoves bounds the lines of one group move (api.GroupMoveRequest).
const MaxGroupMoves = 50

// actionGroupMoved is the audit action of a group move (every booking).
const actionGroupMoved = "booking.group_moved"

// GroupMove moves several lines at once (all or nothing): each line goes to
// its spot from in.From (default today), or from the first day of its stay
// when that is later. Requires auth.PermBookingsWrite.
func (s *Service) GroupMove(ctx context.Context, p *auth.Principal, establishmentID string, in GroupMoveInput) (*GroupMoveResult, error) {
	v := &configerr.ValidationError{}
	switch {
	case len(in.Moves) == 0:
		v.Add("moves", "Almeno uno spostamento.")
	case len(in.Moves) > MaxGroupMoves:
		v.Add("moves", fmt.Sprintf("Al massimo %d spostamenti.", MaxGroupMoves))
	}
	seenLine := map[string]bool{}
	for i, m := range in.Moves {
		f := fmt.Sprintf("moves[%d]", i)
		if m.BookingID == "" {
			v.Add(f+".booking_id", "Campo obbligatorio.")
		}
		if m.LineID == "" {
			v.Add(f+".line_id", "Campo obbligatorio.")
		} else if seenLine[m.LineID] {
			v.Add(f+".line_id", "Riga ripetuta.")
		}
		if m.SpotID == "" {
			v.Add(f+".spot_id", "Campo obbligatorio.")
		}
		seenLine[m.LineID] = true
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	out := &GroupMoveResult{Results: []ChangeResult{}}
	conflicts, err := s.runChange(ctx, in.DryRun, func(ctx context.Context, tx bun.Tx) error {
		return s.groupMoveInTx(ctx, tx, p, es, in, out)
	})
	if err != nil {
		return nil, err
	}
	out.Conflicts = conflicts
	out.Applied = !in.DryRun
	if !out.Applied {
		out.Undo = nil
		for i := range out.Results {
			out.Results[i].Applied = false
		}
	}
	return out, nil
}

func (s *Service) groupMoveInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in GroupMoveInput, out *GroupMoveResult) error {
	var ids []string
	for _, m := range in.Moves {
		ids = append(ids, m.BookingID)
	}
	if err := store.LockBookings(ctx, tx, es, ids); err != nil {
		return err
	}
	today, err := s.today(ctx, tx, es)
	if err != nil {
		return err
	}
	from := in.From
	if from == nil || from.IsZero() {
		from = &today
	} else if from.Before(today) {
		return configerr.Invalid("from", "La data non può essere nel passato.")
	}
	spec := reassignSpec{Expected: map[string]*int64{}, Action: actionGroupMoved, Diff: map[string]any{"group_move": len(in.Moves)}}
	for i, m := range in.Moves {
		b, err := store.GetBooking(ctx, tx, es, m.BookingID)
		if err != nil {
			return notFound(err)
		}
		start := laterDate(*from, b.StartsOn)
		if start.After(b.EndsOn) {
			return configerr.Invalid(fmt.Sprintf("moves[%d].booking_id", i), "La prenotazione è già terminata alla data dello spostamento.")
		}
		spec.Changes = append(spec.Changes, lineChange{
			BookingID: m.BookingID, LineID: m.LineID, From: start,
			Segments: []SegmentInput{{SpotID: m.SpotID, StartsOn: start, EndsOn: b.EndsOn}},
		})
		addExpected(spec.Expected, m.BookingID, m.ExpectedVersion)
	}
	res, err := s.reassignInTx(ctx, tx, p, es, spec)
	if err != nil {
		return err
	}
	// One result per booking, in the order of the request.
	var order []string
	for _, m := range in.Moves {
		if !slices.Contains(order, m.BookingID) {
			order = append(order, m.BookingID)
		}
	}
	for _, id := range order {
		r := res.Results[id]
		r.Applied = true
		out.Results = append(out.Results, *r)
	}
	out.Undo, err = s.recordUndo(ctx, tx, p, es, store.MoveUndoGroup, res.Lines, res.Versions)
	return err
}
