package booking

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

// Swap and group move (spec 30.7, ADR 0028, INV-101). Owner: the moves
// builder. Both run in ONE transaction: the bookings are locked in id
// order, the allocations of the lines from From are released and
// re-created on the other spot (the unique keys of spot_allocations make a
// double allocation impossible at any instant), the price is re-quoted when
// the price category changes, and the audit entry and the booking.modified
// outbox event are written for every booking. A move_undo_log row is
// written for the 5 minute undo (moveundo.go). The re-placement itself is
// reassignInTx (reassign.go), shared with the group move and the undo.

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

// actionSwapped is the audit action of a swap (both bookings).
const actionSwapped = "booking.swapped"

// Swap exchanges the spots of two booking lines on their common interval
// from in.From: on every day from max(From, both starts) to the earlier of
// the two ends each line takes the spot the other one had that day; outside
// that interval each line stays where it was. One transaction, bookings
// locked in id order, both lines released before either is re-allocated,
// re-pricing and audit + booking.modified for both (INV-101). Requires
// auth.PermBookingsWrite.
func (s *Service) Swap(ctx context.Context, p *auth.Principal, establishmentID string, in SwapInput) (*SwapResult, error) {
	v := &configerr.ValidationError{}
	for side, x := range map[string]SwapSide{"a": in.A, "b": in.B} {
		if x.BookingID == "" {
			v.Add(side+".booking_id", "Campo obbligatorio.")
		}
		if x.LineID == "" {
			v.Add(side+".line_id", "Campo obbligatorio.")
		}
	}
	if in.From.IsZero() {
		v.Add("from", "Campo obbligatorio.")
	}
	if in.A.LineID != "" && in.A.LineID == in.B.LineID {
		v.Add("b.line_id", "Scegli due righe diverse.")
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	es, err := s.authorize(ctx, p, establishmentID, auth.PermBookingsWrite)
	if err != nil {
		return nil, err
	}
	out := &SwapResult{From: in.From}
	conflicts, err := s.runChange(ctx, in.DryRun, func(ctx context.Context, tx bun.Tx) error {
		return s.swapInTx(ctx, tx, p, es, in, out)
	})
	if err != nil {
		return nil, err
	}
	out.Conflicts = conflicts
	out.Applied = !in.DryRun
	if out.A.Detail == nil || out.B.Detail == nil { // dry run refused by a conflict
		if err := s.fillSwapDetails(ctx, es, in, out); err != nil {
			return nil, err
		}
	}
	if !out.Applied {
		out.Undo = nil
	}
	return out, nil
}

func (s *Service) swapInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, in SwapInput, out *SwapResult) error {
	if err := store.LockBookings(ctx, tx, es, []string{in.A.BookingID, in.B.BookingID}); err != nil {
		return err
	}
	ba, err := store.GetBooking(ctx, tx, es, in.A.BookingID)
	if err != nil {
		return notFound(err)
	}
	bb, err := store.GetBooking(ctx, tx, es, in.B.BookingID)
	if err != nil {
		return notFound(err)
	}
	sa, err := allocSegments(ctx, tx, es, ba)
	if err != nil {
		return err
	}
	sb, err := allocSegments(ctx, tx, es, bb)
	if err != nil {
		return err
	}
	today, err := s.today(ctx, tx, es)
	if err != nil {
		return err
	}
	if in.From.Before(today) {
		return configerr.Invalid("from", "La data non può essere nel passato.")
	}
	from := laterDate(in.From, laterDate(ba.StartsOn, bb.StartsOn))
	to := ba.EndsOn
	if bb.EndsOn.Before(to) {
		to = bb.EndsOn
	}
	out.From, out.To = from, to
	segA, segB := sa[in.A.LineID], sb[in.B.LineID]
	switch {
	case to.Before(from):
		return fmt.Errorf("%w: nessun giorno in comune dal %s", ErrSwapConflict, in.From)
	case len(segA) == 0 || len(segB) == 0:
		return fmt.Errorf("%w: riga senza postazione", ErrSwapConflict)
	}
	newA := swapSegments(segA, segB, from, to, ba.EndsOn)
	newB := swapSegments(segB, segA, from, to, bb.EndsOn)
	if sameSegments(newA, clipFrom(segA, from)) {
		return fmt.Errorf("%w: le due righe sono sulla stessa postazione", ErrSwapConflict)
	}
	res, err := s.reassignInTx(ctx, tx, p, es, reassignSpec{
		Changes: []lineChange{
			{BookingID: ba.ID, LineID: in.A.LineID, From: from, Segments: newA},
			{BookingID: bb.ID, LineID: in.B.LineID, From: from, Segments: newB},
		},
		Expected: addExpected(addExpected(map[string]*int64{}, ba.ID, in.A.ExpectedVersion), bb.ID, in.B.ExpectedVersion),
		Action:   actionSwapped,
		Diff: map[string]any{
			"swap": map[string]any{"a_booking_id": ba.ID, "b_booking_id": bb.ID, "from": from.String(), "to": to.String()},
		},
		Validate: func(err error) error { return fmt.Errorf("%w: %w", ErrSwapConflict, err) },
	})
	if err != nil {
		return err
	}
	out.A = swapOutcome(res.Results[ba.ID], spotOn(segA, from), spotOn(segB, from))
	out.B = swapOutcome(res.Results[bb.ID], spotOn(segB, from), spotOn(segA, from))
	if ba.ID == bb.ID {
		// Two lines of the same booking: one re-pricing for both.
		out.B.PreviousTotal, out.B.NewTotal = out.A.PreviousTotal, out.A.NewTotal
	}
	out.Undo, err = s.recordUndo(ctx, tx, p, es, store.MoveUndoSwap, res.Lines, res.Versions)
	return err
}

// fillSwapDetails loads the current bookings for a dry run refused by a
// conflict (nothing was re-priced).
func (s *Service) fillSwapDetails(ctx context.Context, es store.EstablishmentScope, in SwapInput, out *SwapResult) error {
	for _, side := range []struct {
		in  SwapSide
		out *SwapOutcome
	}{{in.A, &out.A}, {in.B, &out.B}} {
		b, err := store.GetBooking(ctx, s.d.DB, es, side.in.BookingID)
		if err != nil {
			return notFound(err)
		}
		d, err := s.loadDetail(ctx, s.d.DB, es, b)
		if err != nil {
			return err
		}
		side.out.Detail = d
		side.out.PreviousTotal, side.out.NewTotal = b.TotalAmount, b.TotalAmount
	}
	return nil
}

func swapOutcome(r *ChangeResult, fromSpot, toSpot string) SwapOutcome {
	return SwapOutcome{Detail: r.Detail, FromSpotID: fromSpot, ToSpotID: toSpot, PreviousTotal: r.PreviousTotal, NewTotal: r.NewTotal}
}

// swapSegments are the segments of a line from from to end after a swap:
// the spot of other on every day of from..to, its own spot after to.
func swapSegments(own, other []SegmentInput, from, to, end calendar.Date) []SegmentInput {
	var out []SegmentInput
	for _, d := range calendar.Range(from, end) {
		spot := spotOn(own, d)
		if !d.After(to) {
			spot = spotOn(other, d)
		}
		if n := len(out); n > 0 && out[n-1].SpotID == spot {
			out[n-1].EndsOn = d
			continue
		}
		out = append(out, SegmentInput{SpotID: spot, StartsOn: d, EndsOn: d})
	}
	return out
}

func sameSegments(a, b []SegmentInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].SpotID != b[i].SpotID || !a[i].StartsOn.Equal(b[i].StartsOn) || !a[i].EndsOn.Equal(b[i].EndsOn) {
			return false
		}
	}
	return true
}

func laterDate(a, b calendar.Date) calendar.Date {
	if a.After(b) {
		return a
	}
	return b
}

// addExpected records the expected version of booking id in m (nil: no
// precondition). Two different versions for one booking can never both
// match: the booking then gets an impossible version (412).
func addExpected(m map[string]*int64, id string, v *int64) map[string]*int64 {
	if v == nil {
		return m
	}
	if cur, ok := m[id]; ok && cur != nil && *cur != *v {
		bad := int64(-1)
		m[id] = &bad
		return m
	}
	m[id] = v
	return m
}
