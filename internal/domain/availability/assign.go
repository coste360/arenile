package availability

import (
	"cmp"
	"math"
	"slices"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

// Spot is the metadata of an active spot used by the auto-assignment. Callers
// pass only active spots (inactive spots are never bookable).
type Spot struct {
	ID         uuid.UUID
	ZoneID     uuid.UUID
	Label      string
	Row        string
	Number     int
	Kind       string
	Attributes []string
	X, Y       float64
	// OnlineExcluded: not on sale online (spec 30.11, INV-105); a line of a
	// self-service booking (Line.SelfService) is never placed on it.
	OnlineExcluded bool
}

// Line is a booking line of a zone-mode zone waiting for a spot.
type Line struct {
	ID         uuid.UUID
	BookingID  uuid.UUID
	CustomerID uuid.UUID
	// ZoneID restricts candidates to the spots of this zone (uuid.Nil = any
	// spot passed in Input.Spots).
	ZoneID   uuid.UUID
	Interval Interval
	// PreferredAttributes are spot attributes requested by the customer
	// (e.g. "accessible", "shade_morning").
	PreferredAttributes []string
	// SpotKinds lists compatible spot kinds (pack.requires_spot_kind); empty
	// means any kind.
	SpotKinds []string
	// SelfService marks a line of an online or api booking: spots with
	// OnlineExcluded are not candidates (INV-105).
	SelfService bool
	// PreviousSpotID is the spot the same customer had in a previous booking
	// (continuity), uuid.Nil if none.
	PreviousSpotID uuid.UUID
	// AllowSpotChange is the effective allow_spot_change_during_stay policy:
	// when no spot is free for the whole stay, the line may be split across
	// spots by date.
	AllowSpotChange bool
}

// Placement is a line already on a spot (for instance another line of the same
// booking assigned manually): it attracts the other lines of its booking.
type Placement struct {
	BookingID uuid.UUID
	LineID    uuid.UUID
	SpotID    uuid.UUID
}

// AssignInput is the input of AutoAssign.
type AssignInput struct {
	Lines  []Line
	Spots  []Spot
	Free   *FreeSpots // free spots per half-day; not modified
	Placed []Placement
}

// Segment is a part of a line's stay on one spot.
type Segment struct {
	SpotID   uuid.UUID
	Interval Interval
}

// Assignment places a line on one spot (one segment) or, when split, on
// several spots by consecutive dates.
type Assignment struct {
	LineID   uuid.UUID
	Segments []Segment
}

// SpotID returns the spot of the first segment.
func (a Assignment) SpotID() uuid.UUID {
	if len(a.Segments) == 0 {
		return uuid.Nil
	}
	return a.Segments[0].SpotID
}

// Split reports whether the line changes spot during the stay.
func (a Assignment) Split() bool { return len(a.Segments) > 1 }

// UnassignedReason explains why a line could not be placed. The line stays
// pending and is reported to the manager.
type UnassignedReason string

// Unassigned reasons.
const (
	ReasonInvalidInterval      UnassignedReason = "invalid_interval"
	ReasonNoCompatibleSpot     UnassignedReason = "no_compatible_spot"
	ReasonNoSpotForWholePeriod UnassignedReason = "no_spot_for_whole_period"
	ReasonNoSpotOnSomeDates    UnassignedReason = "no_spot_on_some_dates"
)

// Unassigned is a line left pending.
type Unassigned struct {
	LineID uuid.UUID
	Reason UnassignedReason
}

// AssignResult is the output of AutoAssign. Both slices follow the processing
// order (longest stays first).
type AssignResult struct {
	Assignments []Assignment
	Unassigned  []Unassigned
}

// Score weights. Continuity dominates, then proximity to the other lines of
// the same booking, then customer preferences, then compact row filling.
const (
	scoreContinuity      = 10000
	scoreSameRowAdjacent = 3000
	scoreSameRowStep     = 100
	scoreSameRowFloor    = 1000
	scoreOtherRowMax     = 1000
	scoreOtherRowPerUnit = 10
	scoreAttribute       = 500
	scoreNeighbourMax    = 100
)

// AutoAssign is the greedy auto-assignment of spec 7.5.
//
//  1. Lines are processed by decreasing number of half-days (the longest are
//     the hardest to place); ties by start date, booking ID, line ID.
//  2. For each line, among the compatible spots free on every half-day of the
//     stay, the best by score wins: continuity with the customer's previous
//     spot, proximity to the other lines of the same booking, matched
//     preferred attributes, occupied neighbours in the same row (compact
//     filling). Ties are broken by row, number, label and ID, so the result is
//     deterministic and independent of the input order.
//  3. If no spot is free for the whole stay, the line is split across spots
//     only when AllowSpotChange is set, using the fewest segments (each
//     segment takes the spot free for the longest run of dates); otherwise it
//     is left unassigned.
//
// A spot is never given to two lines on the same half-day, and only spots
// free in Free are used.
func AutoAssign(in AssignInput) AssignResult {
	a := newAssigner(in)
	lines := slices.Clone(in.Lines)
	slices.SortStableFunc(lines, func(x, y Line) int {
		if c := cmp.Compare(len(y.Interval.DateSlots()), len(x.Interval.DateSlots())); c != 0 {
			return c
		}
		if c := x.Interval.From.Time().Compare(y.Interval.From.Time()); c != 0 {
			return c
		}
		if c := cmp.Compare(x.BookingID.String(), y.BookingID.String()); c != 0 {
			return c
		}
		return cmp.Compare(x.ID.String(), y.ID.String())
	})
	var res AssignResult
	for _, l := range lines {
		segs, reason := a.place(l)
		if reason != "" {
			res.Unassigned = append(res.Unassigned, Unassigned{LineID: l.ID, Reason: reason})
			continue
		}
		res.Assignments = append(res.Assignments, Assignment{LineID: l.ID, Segments: segs})
	}
	return res
}

type rowPos struct {
	row    string
	number int
}

type assigner struct {
	spots    []Spot // deduplicated, sorted by tie-break order
	byPos    map[rowPos]uuid.UUID
	spotByID map[uuid.UUID]Spot
	free     *FreeSpots
	siblings map[uuid.UUID][]uuid.UUID // booking -> spots
}

func newAssigner(in AssignInput) *assigner {
	a := &assigner{
		byPos:    map[rowPos]uuid.UUID{},
		spotByID: map[uuid.UUID]Spot{},
		free:     in.Free.Clone(),
		siblings: map[uuid.UUID][]uuid.UUID{},
	}
	for _, s := range in.Spots {
		if _, dup := a.spotByID[s.ID]; dup {
			continue
		}
		a.spotByID[s.ID] = s
		a.spots = append(a.spots, s)
		if s.Row != "" {
			if _, ok := a.byPos[rowPos{s.Row, s.Number}]; !ok {
				a.byPos[rowPos{s.Row, s.Number}] = s.ID
			}
		}
	}
	slices.SortFunc(a.spots, tieBreak)
	for _, p := range in.Placed {
		a.siblings[p.BookingID] = append(a.siblings[p.BookingID], p.SpotID)
	}
	return a
}

func tieBreak(x, y Spot) int {
	if c := cmp.Compare(x.Row, y.Row); c != 0 {
		return c
	}
	if c := cmp.Compare(x.Number, y.Number); c != 0 {
		return c
	}
	if c := cmp.Compare(x.Label, y.Label); c != 0 {
		return c
	}
	return cmp.Compare(x.ID.String(), y.ID.String())
}

func (a *assigner) compatible(l Line) []Spot {
	var out []Spot
	for _, s := range a.spots {
		if l.ZoneID != uuid.Nil && s.ZoneID != l.ZoneID {
			continue
		}
		if len(l.SpotKinds) > 0 && !slices.Contains(l.SpotKinds, s.Kind) {
			continue
		}
		if l.SelfService && s.OnlineExcluded {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (a *assigner) place(l Line) ([]Segment, UnassignedReason) {
	if len(l.Interval.DateSlots()) == 0 {
		return nil, ReasonInvalidInterval
	}
	cands := a.compatible(l)
	if len(cands) == 0 {
		return nil, ReasonNoCompatibleSpot
	}
	if best, ok := a.best(l, cands, l.Interval); ok {
		segs := []Segment{{SpotID: best.ID, Interval: l.Interval}}
		a.commit(l, segs)
		return segs, ""
	}
	if !l.AllowSpotChange || l.Interval.Days() < 2 {
		return nil, ReasonNoSpotForWholePeriod
	}
	segs, ok := a.split(l, cands)
	if !ok {
		return nil, ReasonNoSpotOnSomeDates
	}
	a.commit(l, segs)
	return segs, ""
}

// best returns the highest-scoring candidate free for the whole interval.
func (a *assigner) best(l Line, cands []Spot, iv Interval) (Spot, bool) {
	var (
		best      Spot
		bestScore = math.MinInt
		found     bool
	)
	for _, s := range cands { // cands are in tie-break order: strict > keeps the first
		if !a.free.FreeFor(s.ID, iv) {
			continue
		}
		if sc := a.score(l, s, iv); sc > bestScore {
			best, bestScore, found = s, sc, true
		}
	}
	return best, found
}

// split covers the stay with the fewest segments: from the first uncovered
// date, take the spot free for the longest run of consecutive dates (ties by
// score, then tie-break order).
func (a *assigner) split(l Line, cands []Spot) ([]Segment, bool) {
	dates := calendar.Range(l.Interval.From, l.Interval.To)
	var segs []Segment
	for pos := 0; pos < len(dates); {
		var (
			bestSpot  Spot
			bestRun   int
			bestScore = math.MinInt
		)
		for _, s := range cands {
			run := 0
			for pos+run < len(dates) && a.free.FreeFor(s.ID, Interval{From: dates[pos+run], To: dates[pos+run], Slots: l.Interval.Slots}) {
				run++
			}
			if run == 0 {
				continue
			}
			iv := Interval{From: dates[pos], To: dates[pos+run-1], Slots: l.Interval.Slots}
			sc := a.score(l, s, iv)
			if run > bestRun || (run == bestRun && sc > bestScore) {
				bestSpot, bestRun, bestScore = s, run, sc
			}
		}
		if bestRun == 0 {
			return nil, false
		}
		segs = append(segs, Segment{
			SpotID:   bestSpot.ID,
			Interval: Interval{From: dates[pos], To: dates[pos+bestRun-1], Slots: l.Interval.Slots},
		})
		pos += bestRun
	}
	return segs, true
}

func (a *assigner) commit(l Line, segs []Segment) {
	for _, sg := range segs {
		for _, ds := range sg.Interval.DateSlots() {
			a.free.Remove(ds.Date, ds.Slot, sg.SpotID)
		}
		a.siblings[l.BookingID] = append(a.siblings[l.BookingID], sg.SpotID)
	}
}

func (a *assigner) score(l Line, s Spot, iv Interval) int {
	sc := 0
	if l.PreviousSpotID != uuid.Nil && s.ID == l.PreviousSpotID {
		sc += scoreContinuity
	}
	prox := 0
	for _, sib := range a.siblings[l.BookingID] {
		if o, ok := a.spotByID[sib]; ok {
			prox = max(prox, proximity(s, o))
		}
	}
	sc += prox
	seen := map[string]bool{}
	for _, p := range l.PreferredAttributes {
		if !seen[p] && slices.Contains(s.Attributes, p) {
			sc += scoreAttribute
		}
		seen[p] = true
	}
	if s.Row != "" {
		total := len(iv.DateSlots())
		for _, dn := range []int{-1, 1} {
			nb, ok := a.byPos[rowPos{s.Row, s.Number + dn}]
			if !ok {
				continue
			}
			occupied := 0
			for _, ds := range iv.DateSlots() {
				if !a.free.IsFree(nb, ds.Date, ds.Slot) {
					occupied++
				}
			}
			sc += scoreNeighbourMax * occupied / total
		}
	}
	return sc
}

// proximity scores how close s is to o (another spot of the same booking).
func proximity(s, o Spot) int {
	if s.Row != "" && s.Row == o.Row {
		d := s.Number - o.Number
		if d < 0 {
			d = -d
		}
		if d <= 1 {
			return scoreSameRowAdjacent
		}
		return max(scoreSameRowAdjacent-scoreSameRowStep*(d-1), scoreSameRowFloor)
	}
	dist := math.Hypot(s.X-o.X, s.Y-o.Y)
	return max(scoreOtherRowMax-int(math.Round(dist*scoreOtherRowPerUnit)), 0)
}
