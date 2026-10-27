package booking

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// Re-placement of several lines of one or more bookings in ONE transaction
// (swap, group move, undo; spec 30.7, ADR 0028, INV-101). The bookings are
// locked in id order (two operations on overlapping bookings serialize and
// never deadlock on the booking rows), every moved line releases its
// allocations from its date BEFORE any new allocation is inserted (so two
// lines can exchange their spots), then each booking re-allocates its lines
// segment by segment, is re-priced day by day (ADR 0028 decision 1) and
// writes its audit entry and booking.modified event (mutateInTx). The
// unique keys of spot_allocations refuse any half-day taken by someone else
// meanwhile: the whole transaction rolls back (INV-01, INV-102).

// lineChange re-places one line: its allocations from From on are released
// and Segments (covering From..EndsOn of the booking) are allocated.
type lineChange struct {
	BookingID string
	LineID    string
	From      calendar.Date
	Segments  []SegmentInput
}

// undoLine is one line of a move undo entry (store.MoveUndoEntry.Payload).
type undoLine struct {
	BookingID string        `json:"booking_id"`
	LineID    string        `json:"line_id"`
	From      calendar.Date `json:"from"`
	Before    []undoSegment `json:"before"`
	After     []undoSegment `json:"after"`
	SpotFrom  string        `json:"from_spot_id,omitempty"`
	SpotTo    string        `json:"to_spot_id,omitempty"`
}

type undoSegment struct {
	SpotID   string        `json:"spot_id"`
	StartsOn calendar.Date `json:"starts_on"`
	EndsOn   calendar.Date `json:"ends_on"`
}

func toUndoSegments(segs []SegmentInput) []undoSegment {
	out := make([]undoSegment, len(segs))
	for i, sg := range segs {
		out[i] = undoSegment(sg)
	}
	return out
}

func fromUndoSegments(segs []undoSegment) []SegmentInput {
	out := make([]SegmentInput, len(segs))
	for i, sg := range segs {
		out[i] = SegmentInput(sg)
	}
	return out
}

// reassignOutcome is what reassignInTx did.
type reassignOutcome struct {
	// Results by booking id: totals and detail after the change.
	Results map[string]*ChangeResult
	// Lines are the moved lines with their segments before and after.
	Lines []undoLine
	// Versions are the booking versions after the change.
	Versions map[string]int64
	// Order is the booking ids in lock order.
	Order []string
}

// reassignSpec describes one re-placement operation.
type reassignSpec struct {
	Changes  []lineChange
	Expected map[string]*int64 // If-Match per booking (nil = none)
	Action   string            // audit action
	// Diff adds operation fields to the audit diff of each booking.
	Diff map[string]any
	// Validate wraps the validation errors of the new spots (swap: 409).
	Validate func(error) error
}

// reassignInTx applies spec in tx with actor (see the file comment).
func (s *Service) reassignInTx(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, spec reassignSpec) (*reassignOutcome, error) {
	actor := p.Actor(reqctx.From(ctx).ClientIP)
	byBooking := map[string][]lineChange{}
	var ids []string
	for _, c := range spec.Changes {
		if _, ok := byBooking[c.BookingID]; !ok {
			ids = append(ids, c.BookingID)
		}
		byBooking[c.BookingID] = append(byBooking[c.BookingID], c)
	}
	slices.Sort(ids)
	if err := store.LockBookings(ctx, tx, es, ids); err != nil {
		return nil, err
	}
	out := &reassignOutcome{Results: map[string]*ChangeResult{}, Versions: map[string]int64{}, Order: ids}
	before := map[string][]SegmentInput{} // by line
	// 1. Check every booking and line, read the current segments.
	for _, id := range ids {
		b, err := store.GetBooking(ctx, tx, es, id)
		if err != nil {
			return nil, notFound(err)
		}
		if err := configerr.CheckVersion(spec.Expected[id], b.Version); err != nil {
			return nil, err
		}
		if st := Status(b.Status); !modifiable(st) || !st.OccupiesResources() {
			return nil, ErrNotModifiable
		}
		if err := refuseTicketOwned(ctx, tx, es, b); err != nil {
			return nil, err
		}
		rows, err := store.ListBookingLines(ctx, tx, es, id)
		if err != nil {
			return nil, err
		}
		cur, err := allocSegments(ctx, tx, es, b)
		if err != nil {
			return nil, err
		}
		for _, c := range byBooking[id] {
			if !slices.ContainsFunc(rows, func(r store.BookingLine) bool { return r.ID == c.LineID }) {
				return nil, ErrLineNotFound
			}
			if c.From.Before(b.StartsOn) || c.From.After(b.EndsOn) {
				return nil, configerr.Invalid("from", "La data deve rientrare nel periodo della prenotazione.")
			}
			if len(cur[c.LineID]) == 0 {
				return nil, configerr.Invalid("line_id", "La riga non ha una postazione assegnata.")
			}
			if msg := coverError(c.Segments, c.From, b.EndsOn); msg != "" {
				return nil, configerr.Invalid("segments", msg)
			}
			before[c.LineID] = cur[c.LineID]
		}
	}
	// 2. Release every moved line from its date before allocating anything.
	released := map[string]*inventory.Allocation{}
	for _, id := range ids {
		for _, c := range byBooking[id] {
			from := c.From
			a, err := s.d.Inventory.Release(ctx, tx, es, inventory.ReleaseRequest{BookingID: id, LineIDs: []string{c.LineID}, From: &from})
			if err != nil {
				return nil, err
			}
			released[id] = mergeAlloc(released[id], a)
			if err := s.unmarkResold(ctx, tx, es, []string{c.LineID}); err != nil {
				return nil, err
			}
		}
	}
	// 3. Each booking: allocate, re-price, save, audit and event.
	after := map[string][]SegmentInput{}
	for _, id := range ids {
		res := &ChangeResult{Conflicts: []availability.Conflict{}}
		d, err := s.mutateInTx(ctx, tx, actor, es, id, nil, func(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking) (*outcome, error) {
			return s.reassignLocked(ctx, tx, p, es, b, byBooking[id], before, after, released[id], res, spec)
		})
		if err != nil {
			return nil, err
		}
		res.Detail = d
		out.Results[id] = res
		out.Versions[id] = d.Booking.Version
	}
	for _, c := range spec.Changes {
		l := undoLine{
			BookingID: c.BookingID, LineID: c.LineID, From: c.From,
			Before: toUndoSegments(clipFrom(before[c.LineID], c.From)),
			After:  toUndoSegments(clipFrom(after[c.LineID], c.From)),
		}
		l.SpotFrom = spotOn(before[c.LineID], c.From)
		l.SpotTo = spotOn(after[c.LineID], c.From)
		out.Lines = append(out.Lines, l)
	}
	return out, nil
}

// reassignLocked re-places the lines of one locked booking whose moved
// allocations were already released.
func (s *Service) reassignLocked(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, b *store.Booking,
	changes []lineChange, before, after map[string][]SegmentInput, released *inventory.Allocation, res *ChangeResult, spec reassignSpec,
) (*outcome, error) {
	rows, extras, _, err := s.currentLines(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	lineIn, bookingExtras := linesToInput(rows, extras)
	spotsIn := slices.Clone(lineIn)
	idxOf := map[string]int{}
	for i := range rows {
		idxOf[rows[i].ID] = i
	}
	for _, c := range changes {
		i := idxOf[c.LineID]
		// The line takes the spot of its first new segment (as Move does).
		lineIn[i].SpotID, lineIn[i].ZoneID = c.Segments[0].SpotID, ""
		for _, sg := range c.Segments {
			spotsIn = append(spotsIn, LineInput{SpotID: sg.SpotID})
		}
	}
	cat, err := loadCatalog(ctx, tx, es, spotsIn, bookingExtras)
	if err != nil {
		return nil, err
	}
	lines, err := cat.resolve(lineIn, "lines")
	if err != nil {
		if spec.Validate != nil {
			return nil, spec.Validate(err)
		}
		return nil, err
	}
	// Segments after the change: the untouched lines as allocated, the
	// moved ones as before up to their date, then the new segments.
	segs, err := allocSegments(ctx, tx, es, b)
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		segs[c.LineID] = joinSegments(clipBefore(before[c.LineID], c.From), c.Segments)
	}
	withSegments(lines, rows, segs)
	st := stay{StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: b.Slots, Formula: b.Formula, Channel: b.Channel}
	if b.IsSeasonPass && b.SeasonID != nil {
		st.SeasonID = *b.SeasonID
	}
	pr, err := s.repriceAs(ctx, tx, es, b, st, lines, bookingExtras, cat)
	if err != nil {
		return nil, err
	}
	diff := map[string]any{}
	for k, v := range spec.Diff {
		diff[k] = v
	}
	if err := s.checkModifyApproval(ctx, tx, p, es, b, pr.approval, false, diff); err != nil {
		return nil, err
	}
	res.PreviousTotal, res.NewTotal = b.TotalAmount, pr.res.Breakdown.Total
	kind := allocationKind(b)
	var allocated *inventory.Allocation
	for _, c := range changes {
		i := idxOf[c.LineID]
		row := rows[i]
		row.SpotID, row.ZoneID = &lines[i].SpotID, lines[i].ZoneID
		// The first line also carries the booking-level components.
		base := allocRequest(b, kind, b.HoldExpiresAt, []store.BookingLine{row}, lineExtras(extras, row.ID, i == 0), cat)
		for _, sg := range c.Segments {
			a, err := s.d.Inventory.Allocate(ctx, tx, es, cat.segmentRequest(base, base.Lines[0], sg))
			if err != nil {
				return nil, err
			}
			allocated = mergeAlloc(allocated, a)
		}
	}
	now := s.now()
	if _, err := s.rewriteRows(ctx, tx, es, b, rows, pr, cat, now, false); err != nil {
		return nil, err
	}
	moved := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		after[c.LineID] = segs[c.LineID]
		if kind != inventory.KindSeason {
			if err := s.markSegmentsResold(ctx, tx, es, c.LineID, segs[c.LineID], b.Slots); err != nil {
				return nil, err
			}
		}
		moved = append(moved, map[string]any{
			"line_id": c.LineID, "from": c.From.String(),
			"from_spot_id": spotOn(before[c.LineID], c.From), "to_spot_id": spotOn(segs[c.LineID], c.From),
		})
	}
	diff["lines"] = moved
	if err := s.applyTotals(b, pr, now, diff); err != nil {
		return nil, err
	}
	res.NewTotal = b.TotalAmount
	diff["previous_total"], diff["new_total"] = int64(res.PreviousTotal), int64(res.NewTotal)
	return &outcome{action: spec.Action, event: eventModified, diff: diff, alloc: mergeAlloc(released, allocated)}, nil
}

// recordUndo writes the undo entry of an applied move in tx (users only:
// an API key cannot undo) and returns its info.
func (s *Service) recordUndo(ctx context.Context, tx bun.IDB, p *auth.Principal, es store.EstablishmentScope, kind string, lines []undoLine, versions map[string]int64) (*UndoInfo, error) {
	if p == nil || p.Kind != auth.PrincipalUser || p.UserID == "" || len(lines) == 0 {
		return nil, nil
	}
	payload := undoPayload{Lines: lines, Versions: versions}
	raw, err := marshalUndo(payload)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, l := range lines {
		if !slices.Contains(ids, l.BookingID) {
			ids = append(ids, l.BookingID)
		}
	}
	now := s.now()
	uid := p.UserID
	e := &store.MoveUndoEntry{
		ID: store.NewID(), UserID: &uid, Kind: kind, Payload: raw, BookingIDs: ids,
		CreatedAt: now, ExpiresAt: now.Add(store.MoveUndoWindow),
	}
	if err := store.InsertMoveUndo(ctx, tx, es, e); err != nil {
		return nil, fmt.Errorf("booking: %w", err)
	}
	return undoInfo(e), nil
}

func undoInfo(e *store.MoveUndoEntry) *UndoInfo {
	return &UndoInfo{MoveID: e.ID, Kind: e.Kind, ExpiresAt: e.ExpiresAt, BookingIDs: slices.Clone(e.BookingIDs)}
}

// conflictsOf extracts the conflicts of a refused allocation (nil, false
// for other errors).
func conflictsOf(err error) ([]availability.Conflict, bool) {
	var ce *availability.ConflictError
	if errors.As(err, &ce) {
		return ce.Conflicts, true
	}
	return nil, false
}

// runChange runs fn in one transaction, rolled back when dryRun; the
// conflicts of a dry run are returned instead of the error.
func (s *Service) runChange(ctx context.Context, dryRun bool, fn func(ctx context.Context, tx bun.Tx) error) ([]availability.Conflict, error) {
	var conflicts []availability.Conflict
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		err := fn(ctx, tx)
		if cs, ok := conflictsOf(err); ok && dryRun {
			conflicts = cs
			return errDryRun
		}
		if err != nil {
			return err
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		err = nil
	}
	if conflicts == nil {
		conflicts = []availability.Conflict{}
	}
	return conflicts, err
}
