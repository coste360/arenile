package booking

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
)

// Split stays (spec 30.6, 30.7, ADR 0028, INV-102). A line occupies one
// spot per half-day (unique index spot_allocations (booking_line_id, date,
// slot)); its segments are not stored but derived from its allocations.
// Find spot creates a line with segments (one allocation per segment, in
// the creation transaction); a move with from, a swap and a group move
// change the allocations from a date. Every re-pricing of a line passes its
// segments to the engine, which prices each day with the spot occupied that
// day (ADR 0028 decision 1).

// MaxSegments bounds the segments of a line (api.BookingLineInput.segments).
const MaxSegments = 4

// ErrTotalChanged: CreateInput.ExpectedTotal differs from the computed total
// (find spot maps it to 409 price_changed).
var ErrTotalChanged = errors.New("booking: total changed")

// SegmentInput is the part of a line's stay spent on one spot, StartsOn to
// EndsOn inclusive (api.BookingSegmentInput, api.BookingLineSegment).
type SegmentInput struct {
	SpotID   string
	StartsOn calendar.Date
	EndsOn   calendar.Date
}

// EngineSegments converts segments for the pricing engine (nil for none).
func EngineSegments(segs []SegmentInput) []pricing.LineSegment {
	if len(segs) == 0 {
		return nil
	}
	out := make([]pricing.LineSegment, len(segs))
	for i, sg := range segs {
		out[i] = pricing.LineSegment{SpotID: sg.SpotID, From: sg.StartsOn, To: sg.EndsOn}
	}
	return out
}

func hasSegments(lines []PricedLine) bool {
	for _, l := range lines {
		if len(l.Segments) > 0 {
			return true
		}
	}
	return false
}

// validateSegments checks that the segments of every line are consecutive
// and cover the stay exactly once (INV-102). Split lines need resources
// at once: not for drafts (confirmed later on one spot) nor panel holds.
func validateSegments(v *configerr.ValidationError, in *CreateInput) {
	for i, l := range in.Lines {
		if len(l.Segments) == 0 {
			continue
		}
		f := fmt.Sprintf("lines[%d].segments", i)
		switch {
		case len(l.Segments) > MaxSegments:
			v.Add(f, fmt.Sprintf("Al massimo %d segmenti.", MaxSegments))
			continue
		case in.Status == StatusDraft:
			v.Add(f, "Una bozza non può avere segmenti.")
			continue
		case in.HoldID != "":
			v.Add(f, "I segmenti non si usano con un hold del pannello.")
			continue
		case l.Quantity > 1:
			v.Add(f, "I segmenti valgono per una sola postazione.")
			continue
		}
		if msg := coverError(l.Segments, in.StartsOn, in.EndsOn); msg != "" {
			v.Add(f, msg)
		}
	}
}

// coverError is "" when segs are consecutive, on different spots in a row
// and cover from..to exactly once; the message otherwise.
func coverError(segs []SegmentInput, from, to calendar.Date) string {
	next := from
	for j, sg := range segs {
		switch {
		case sg.SpotID == "":
			return "Ogni segmento richiede la postazione."
		case sg.StartsOn.IsZero() || sg.EndsOn.IsZero() || sg.EndsOn.Before(sg.StartsOn):
			return "Date del segmento non valide."
		case !sg.StartsOn.Equal(next):
			return "I segmenti devono essere consecutivi e iniziare con la prenotazione."
		case j > 0 && segs[j-1].SpotID == sg.SpotID:
			return "Due segmenti consecutivi sulla stessa postazione."
		}
		next = sg.EndsOn.AddDays(1)
	}
	if !next.Equal(to.AddDays(1)) {
		return "I segmenti devono coprire tutte le date della prenotazione."
	}
	return ""
}

// resolveSegments validates the spots of the segments of l against the
// catalog: the line takes the spot of the first segment.
func (c *catalog) resolveSegments(v *configerr.ValidationError, f string, l LineInput, pl *PricedLine) {
	first := l.Segments[0].SpotID
	if l.SpotID != "" && l.SpotID != first {
		v.Add(f+".spot_id", "Deve essere la postazione del primo segmento.")
	}
	pl.SpotID = first
	pl.Segments = l.Segments
	pack := c.packs[l.PackID]
	for j, sg := range l.Segments {
		sf := fmt.Sprintf("%s.segments[%d].spot_id", f, j)
		sp, ok := c.spots[sg.SpotID]
		switch {
		case !ok:
			v.Add(sf, "Postazione inesistente.")
		case !sp.Active:
			v.Add(sf, "Postazione non attiva.")
		case c.zones[sp.ZoneID].AssignmentMode == modeQuantity:
			v.Add(sf, "Le zone a quantità non hanno postazioni.")
		case len(pack.AllowedZoneIDs) > 0 && !contains(pack.AllowedZoneIDs, sp.ZoneID):
			v.Add(sf, "Pack non disponibile nella zona di questa postazione.")
		}
	}
}

// segmentRequest is the allocation of one segment of a line: the line
// request on the segment spot (and its zone) over the segment dates.
func (c *catalog) segmentRequest(base inventory.AllocateRequest, lr inventory.LineRequest, sg SegmentInput) inventory.AllocateRequest {
	r := base
	r.StartsOn, r.EndsOn = sg.StartsOn, sg.EndsOn
	lr.SpotID = sg.SpotID
	if sp, ok := c.spots[sg.SpotID]; ok {
		lr.ZoneID = sp.ZoneID
	}
	r.Lines = []inventory.LineRequest{lr}
	return r
}

// allocateSplit allocates the lines of a new booking, one request per
// segment for split lines (the unique keys refuse any half-day taken
// meanwhile: 409 with the conflicts), and records the allocation on the
// rows like applyAllocation.
func (s *Service) allocateSplit(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, b *store.Booking, kind inventory.Kind, expiresAt *time.Time,
	rows []store.BookingLine, extras []store.BookingLineExtra, cat *catalog, lines []PricedLine, now time.Time, desk bool,
) (*inventory.Allocation, error) {
	req := allocRequest(b, kind, expiresAt, rows, extras, cat)
	req.ByDesk = desk
	plain := req
	plain.Lines = nil
	var plainRows []store.BookingLine
	var out *inventory.Allocation
	for i, lr := range req.Lines {
		if len(lines[i].Segments) == 0 {
			plain.Lines = append(plain.Lines, lr)
			plainRows = append(plainRows, rows[i])
			continue
		}
		for _, sg := range lines[i].Segments {
			a, err := s.d.Inventory.Allocate(ctx, tx, es, cat.segmentRequest(req, lr, sg))
			if err != nil {
				return nil, err
			}
			out = mergeAlloc(out, a)
		}
		if kind != inventory.KindSeason {
			if err := s.markSegmentsResold(ctx, tx, es, rows[i].ID, lines[i].Segments, b.Slots); err != nil {
				return nil, err
			}
		}
	}
	if len(plain.Lines) > 0 {
		a, err := s.d.Inventory.Allocate(ctx, tx, es, plain)
		if err != nil {
			return nil, err
		}
		if err := s.applyAllocation(ctx, tx, es, b, plainRows, a, kind, now); err != nil {
			return nil, err
		}
		out = mergeAlloc(out, a)
	}
	return out, nil
}

// markSegmentsResold tracks the resale of released season days segment by
// segment (applyAllocation does it for single-spot lines).
func (s *Service) markSegmentsResold(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, lineID string, segs []SegmentInput, slots calendar.Slots) error {
	if s.d.Releases == nil {
		return nil
	}
	for _, sg := range segs {
		if err := s.d.Releases.MarkResold(ctx, tx, es, lineID, sg.SpotID, calendar.Range(sg.StartsOn, sg.EndsOn), slots); err != nil {
			return err
		}
	}
	return nil
}

// checkSplit is Inventory.Check of a creation preview with split lines
// checked segment by segment.
func (s *Service) checkSplit(ctx context.Context, db bun.IDB, es store.EstablishmentScope, req inventory.AllocateRequest, lines []PricedLine, cat *catalog) ([]availability.Conflict, error) {
	if !hasSegments(lines) {
		return s.d.Inventory.Check(ctx, db, es, req)
	}
	plain := req
	plain.Lines = nil
	var out []availability.Conflict
	for i, lr := range req.Lines {
		if i >= len(lines) || len(lines[i].Segments) == 0 {
			plain.Lines = append(plain.Lines, lr)
			continue
		}
		for _, sg := range lines[i].Segments {
			cs, err := s.d.Inventory.Check(ctx, db, es, cat.segmentRequest(req, lr, sg))
			if err != nil {
				return nil, err
			}
			out = append(out, cs...)
		}
	}
	if len(plain.Lines) > 0 {
		cs, err := s.d.Inventory.Check(ctx, db, es, plain)
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	return out, nil
}

// SegmentsFromAllocations derives the segments of every line from its spot
// allocations: runs of dates on the same spot, the first one starting with
// the stay and each one ending the day before the next (closure days
// without allocations stay with the run before them), the last one ending
// with the stay. Lines without allocations are absent.
func SegmentsFromAllocations(allocs []store.SpotAllocation, startsOn, endsOn calendar.Date) map[string][]SegmentInput {
	type day struct {
		date calendar.Date
		spot string
	}
	byLine := map[string][]day{}
	for _, a := range allocs {
		if a.BookingLineID == nil {
			continue
		}
		ds := byLine[*a.BookingLineID]
		if n := len(ds); n > 0 && ds[n-1].date.Equal(a.Date) {
			continue // second slot of the same date (one spot per half-day)
		}
		byLine[*a.BookingLineID] = append(ds, day{a.Date, a.SpotID})
	}
	out := make(map[string][]SegmentInput, len(byLine))
	for line, ds := range byLine {
		slices.SortStableFunc(ds, func(a, b day) int { return a.date.Time().Compare(b.date.Time()) })
		var segs []SegmentInput
		for _, d := range ds {
			if n := len(segs); n > 0 && segs[n-1].SpotID == d.spot {
				continue
			}
			from := d.date
			if len(segs) == 0 && startsOn.Before(from) {
				from = startsOn
			}
			if n := len(segs); n > 0 {
				segs[n-1].EndsOn = from.AddDays(-1)
			}
			segs = append(segs, SegmentInput{SpotID: d.spot, StartsOn: from})
		}
		if n := len(segs); n > 0 {
			segs[n-1].EndsOn = endsOn
			if last := ds[len(ds)-1].date; last.After(endsOn) {
				segs[n-1].EndsOn = last
			}
		}
		out[line] = segs
	}
	return out
}

// LineSegments returns the segments of the lines of a booking detail (for
// api.BookingLine.segments): lines without spot allocations are absent.
func LineSegments(d *Detail) map[string][]SegmentInput {
	return SegmentsFromAllocations(d.SpotAllocations, d.Booking.StartsOn, d.Booking.EndsOn)
}

// allocSegments reads the current segments of the lines of b.
func allocSegments(ctx context.Context, db bun.IDB, es store.EstablishmentScope, b *store.Booking) (map[string][]SegmentInput, error) {
	allocs, err := store.ListBookingSpotAllocations(ctx, db, es, b.ID)
	if err != nil {
		return nil, err
	}
	return SegmentsFromAllocations(allocs, b.StartsOn, b.EndsOn), nil
}

// withSegments sets on lines (aligned with rows) the segments of the split
// lines, so that the engine prices each day with the spot of that day; a
// line on one spot is priced as before.
func withSegments(lines []PricedLine, rows []store.BookingLine, segs map[string][]SegmentInput) {
	for i := range lines {
		if i >= len(rows) {
			return
		}
		sg := segs[rows[i].ID]
		if len(sg) > 1 || (len(sg) == 1 && sg[0].SpotID != lines[i].SpotID) {
			lines[i].Segments = sg
		} else {
			lines[i].Segments = nil
		}
	}
}

// clipBefore keeps the part of segs before from (the days a move from a date
// leaves untouched).
func clipBefore(segs []SegmentInput, from calendar.Date) []SegmentInput {
	var out []SegmentInput
	for _, sg := range segs {
		if !sg.StartsOn.Before(from) {
			break
		}
		if !sg.EndsOn.Before(from) {
			sg.EndsOn = from.AddDays(-1)
		}
		out = append(out, sg)
	}
	return out
}

// clipFrom keeps the part of segs from from on.
func clipFrom(segs []SegmentInput, from calendar.Date) []SegmentInput {
	var out []SegmentInput
	for _, sg := range segs {
		if sg.EndsOn.Before(from) {
			continue
		}
		if sg.StartsOn.Before(from) {
			sg.StartsOn = from
		}
		out = append(out, sg)
	}
	return out
}

// joinSegments appends b to a, merging the junction when both sides are on
// the same spot.
func joinSegments(a, b []SegmentInput) []SegmentInput {
	out := append([]SegmentInput(nil), a...)
	for _, sg := range b {
		if n := len(out); n > 0 && out[n-1].SpotID == sg.SpotID && out[n-1].EndsOn.AddDays(1).Equal(sg.StartsOn) {
			out[n-1].EndsOn = sg.EndsOn
			continue
		}
		out = append(out, sg)
	}
	return out
}

// spotOn returns the spot of segs on date d ("" if none).
func spotOn(segs []SegmentInput, d calendar.Date) string {
	for _, sg := range segs {
		if !d.Before(sg.StartsOn) && !d.After(sg.EndsOn) {
			return sg.SpotID
		}
	}
	return ""
}
