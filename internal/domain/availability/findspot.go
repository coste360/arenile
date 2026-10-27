package availability

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

// Find spot (spec 30.6, ADR 0028): the pure search behind "Trova posto".
// It extends the segment algorithm of AutoAssign (spec 7.5) to seat zones
// and to groups: given the free half-days of the candidate spots it returns
// proposals ranked by number of spot changes (0 first), then continuity and
// proximity. No I/O and no clock: internal/domain/findspot loads the
// occupancy, prices the proposals and books the chosen one.

// FindInput is the input of FindSpots.
type FindInput struct {
	// Spots are the active candidate spots (already filtered by zone, kind
	// and pack compatibility by the caller).
	Spots []Spot
	// Free are the free half-days of the candidate spots; not modified.
	Free *FreeSpots
	// Interval is the requested stay.
	Interval Interval
	// Count is the number of spots requested (groups); >= 1.
	Count int
	// MaxSpotChanges is the maximum number of spot changes per line (0..3).
	MaxSpotChanges int
	// RequiredAttributes must all be present on every spot of a proposal.
	RequiredAttributes []string
	// PreviousSpotID is the spot of the same customer in a previous booking
	// (continuity), uuid.Nil if none.
	PreviousSpotID uuid.UUID
	// Limit caps the number of proposals (default 10).
	Limit int
}

// ProposalLine is one requested spot of a proposal: its segments cover the
// stay exactly once, in date order (INV-102).
type ProposalLine struct {
	Segments []Segment
}

// Changes is the number of spot changes of the line.
func (l ProposalLine) Changes() int { return max(len(l.Segments)-1, 0) }

// Proposal is a ranked solution of FindSpots.
type Proposal struct {
	Lines []ProposalLine
	// SpotChanges is the total number of changes of the proposal.
	SpotChanges int
	// Adjacent reports that the spots of a group proposal are neighbours.
	Adjacent bool

	score int
}

// Key identifies the proposal by its segments, stable across searches on
// the same data: "spot:from:to" joined by "|" within a line, lines joined by
// ";" (lines in the order of the proposal).
func (p Proposal) Key() string {
	var b strings.Builder
	for i, l := range p.Lines {
		if i > 0 {
			b.WriteByte(';')
		}
		for j, sg := range l.Segments {
			if j > 0 {
				b.WriteByte('|')
			}
			b.WriteString(sg.SpotID.String())
			b.WriteByte(':')
			b.WriteString(sg.Interval.From.String())
			b.WriteByte(':')
			b.WriteString(sg.Interval.To.String())
		}
	}
	return b.String()
}

// SpotIDs lists the distinct spots of the proposal in order of appearance.
func (p Proposal) SpotIDs() []uuid.UUID {
	var out []uuid.UUID
	for _, l := range p.Lines {
		for _, sg := range l.Segments {
			if !slices.Contains(out, sg.SpotID) {
				out = append(out, sg.SpotID)
			}
		}
	}
	return out
}

// Search bounds and score weights of FindSpots.
const (
	defaultFindLimit = 10
	maxFindChanges   = 3
	// splitBranching is the number of spots tried at each segment start of
	// a split line (the ones free for the longest run, nearest first).
	splitBranching = 6
	// splitBudget bounds the complete split coverings explored per line.
	splitBudget = 500
	// groupSeeds is the number of first-line placements tried for a group.
	groupSeeds = 12
	// scoreLongestDay rewards each day of the longest segment of a line
	// (fewer days away from the main spot).
	scoreLongestDay = 50
	// scoreGroupDivisor scales the proximity between the spots of a group.
	scoreGroupDivisor = 10
)

// FindSpots returns the proposals for in, best first (at most Limit).
//
// Ranking: fewer spot changes first; then, for groups, adjacent spots;
// then the score: continuity with PreviousSpotID, the longest stay on one
// spot, the proximity of consecutive segments (moving to the next umbrella
// rather than across the beach) and of the spots of a group; ties by row,
// number, label and ID, so the result is deterministic and independent of
// the input order.
//
// Every proposal is valid on Free: the segments of each line cover the stay
// exactly once, in date order, with at most MaxSpotChanges changes, and
// only use spots free on every half-day of their segment; the lines of a
// group never share a half-day of a spot. The database still decides at
// booking time (INV-01, INV-102).
func FindSpots(in FindInput) []Proposal {
	f := newFinder(in)
	if f == nil {
		return nil
	}
	var props []Proposal
	if f.count == 1 {
		props = f.single()
	} else {
		props = f.group()
	}
	seen := map[string]bool{}
	out := make([]Proposal, 0, len(props))
	for _, p := range props {
		k := p.Key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	slices.SortStableFunc(out, f.compare)
	if len(out) > f.limit {
		out = out[:f.limit]
	}
	return out
}

type finder struct {
	in      FindInput
	spots   []Spot // filtered, in tie-break order
	byID    map[uuid.UUID]Spot
	idx     map[uuid.UUID]int
	dates   []calendar.Date
	count   int
	changes int
	limit   int
	// free[i][k] reports that spots[i] is free on every slot of dates[k].
	free [][]bool
}

func newFinder(in FindInput) *finder {
	if in.Free == nil || !in.Interval.Slots.Valid() {
		return nil
	}
	dates := calendar.Range(in.Interval.From, in.Interval.To)
	if len(dates) == 0 {
		return nil
	}
	f := &finder{
		in: in, byID: map[uuid.UUID]Spot{}, idx: map[uuid.UUID]int{}, dates: dates,
		count: max(in.Count, 1), changes: min(max(in.MaxSpotChanges, 0), maxFindChanges), limit: in.Limit,
	}
	if f.limit <= 0 {
		f.limit = defaultFindLimit
	}
	for _, s := range in.Spots {
		if _, dup := f.byID[s.ID]; dup || !hasAll(s.Attributes, in.RequiredAttributes) {
			continue
		}
		f.byID[s.ID] = s
		f.spots = append(f.spots, s)
	}
	slices.SortFunc(f.spots, tieBreak)
	f.free = make([][]bool, len(f.spots))
	for i, s := range f.spots {
		f.idx[s.ID] = i
		row := make([]bool, len(dates))
		for k, d := range dates {
			row[k] = in.Free.FreeFor(s.ID, Interval{From: d, To: d, Slots: in.Interval.Slots})
		}
		f.free[i] = row
	}
	return f
}

func hasAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

// run is the number of consecutive days from day k on which spot i is free.
func (f *finder) run(free [][]bool, i, k int) int {
	n := 0
	for k+n < len(f.dates) && free[i][k+n] {
		n++
	}
	return n
}

// path is a split line under construction: spot indices and start days.
type path struct {
	spot  []int
	start []int
}

func (f *finder) segments(p path) []Segment {
	out := make([]Segment, len(p.spot))
	for j := range p.spot {
		end := len(f.dates) - 1
		if j+1 < len(p.spot) {
			end = p.start[j+1] - 1
		}
		out[j] = Segment{
			SpotID:   f.spots[p.spot[j]].ID,
			Interval: Interval{From: f.dates[p.start[j]], To: f.dates[end], Slots: f.in.Interval.Slots},
		}
	}
	return out
}

// placements returns the candidate placements of one line on free, best
// first: every spot free for the whole stay and, when changes are allowed
// and fewer than wantWhole spots are free for the whole stay, the split
// coverings found by a bounded depth-first search that always extends a
// segment as far as its spot is free (the farthest reach never needs more
// segments than a shorter one).
func (f *finder) placements(free [][]bool, wantWhole int) [][]Segment {
	var out [][]Segment
	for i := range f.spots {
		if f.run(free, i, 0) == len(f.dates) {
			out = append(out, f.segments(path{spot: []int{i}, start: []int{0}}))
		}
	}
	if f.changes > 0 && len(f.dates) > 1 && len(out) < wantWhole {
		budget := splitBudget
		var dfs func(p path, k int)
		dfs = func(p path, k int) {
			if budget <= 0 {
				return
			}
			if k == len(f.dates) {
				budget--
				if len(p.spot) > 1 {
					out = append(out, f.segments(path{spot: slices.Clone(p.spot), start: slices.Clone(p.start)}))
				}
				return
			}
			if len(p.spot) > f.changes {
				return
			}
			type opt struct{ i, run int }
			var opts []opt
			for i := range f.spots {
				if r := f.run(free, i, k); r > 0 && (len(p.spot) == 0 || p.spot[len(p.spot)-1] != i) {
					opts = append(opts, opt{i, r})
				}
			}
			slices.SortStableFunc(opts, func(a, b opt) int {
				if c := cmp.Compare(b.run, a.run); c != 0 {
					return c
				}
				if len(p.spot) > 0 {
					prev := f.spots[p.spot[len(p.spot)-1]]
					return cmp.Compare(proximity(f.spots[b.i], prev), proximity(f.spots[a.i], prev))
				}
				return 0
			})
			for _, o := range opts[:min(len(opts), splitBranching)] {
				p.spot, p.start = append(p.spot, o.i), append(p.start, k)
				dfs(p, k+o.run)
				p.spot, p.start = p.spot[:len(p.spot)-1], p.start[:len(p.start)-1]
			}
		}
		dfs(path{}, 0)
	}
	slices.SortStableFunc(out, func(a, b []Segment) int {
		if c := cmp.Compare(len(a), len(b)); c != 0 {
			return c
		}
		return cmp.Compare(f.lineScore(b), f.lineScore(a))
	})
	return out
}

// lineScore rates one placement: continuity with the previous spot, the
// longest segment, the proximity of consecutive segments.
func (f *finder) lineScore(segs []Segment) int {
	sc, longest := 0, 0
	for j, sg := range segs {
		days := sg.Interval.Days()
		longest = max(longest, days)
		if f.in.PreviousSpotID != uuid.Nil && sg.SpotID == f.in.PreviousSpotID {
			sc += scoreContinuity * days / len(f.dates)
		}
		if j > 0 {
			sc += proximity(f.byID[sg.SpotID], f.byID[segs[j-1].SpotID])
		}
	}
	return sc + scoreLongestDay*longest
}

func (f *finder) single() []Proposal {
	cands := f.placements(f.free, f.limit+1)
	out := make([]Proposal, 0, len(cands))
	for _, segs := range cands {
		l := ProposalLine{Segments: segs}
		out = append(out, Proposal{Lines: []ProposalLine{l}, SpotChanges: l.Changes(), score: f.lineScore(segs)})
	}
	return out
}

// group builds the proposals for Count lines: the windows of consecutive
// spots of one row free for the whole stay (adjacent), then, from several
// seeds, line by line the placement nearest to the spots already chosen on
// the half-days the previous lines left free (with changes when allowed).
func (f *finder) group() []Proposal {
	var out []Proposal
	for i := 0; i+f.count <= len(f.spots) && len(out) <= f.limit; i++ {
		ok := true
		for j := i; j < i+f.count && ok; j++ {
			s := f.spots[j]
			ok = s.Row != "" && f.run(f.free, j, 0) == len(f.dates)
			if ok && j > i {
				prev := f.spots[j-1]
				ok = prev.Row == s.Row && s.Number-prev.Number == 1
			}
		}
		if !ok {
			continue
		}
		p := Proposal{Adjacent: true}
		for j := i; j < i+f.count; j++ {
			p.Lines = append(p.Lines, ProposalLine{Segments: f.segments(path{spot: []int{j}, start: []int{0}})})
		}
		p.score = f.groupScore(p)
		out = append(out, p)
	}
	seeds := f.placements(f.free, groupSeeds)
	for _, seed := range seeds[:min(len(seeds), groupSeeds)] {
		free := cloneFree(f.free)
		f.take(free, seed)
		p := Proposal{Lines: []ProposalLine{{Segments: seed}}}
		for len(p.Lines) < f.count {
			cands := f.placements(free, f.count)
			if len(cands) == 0 {
				break
			}
			best, bestScore := 0, 0
			for c, segs := range cands {
				sc := f.nearness(p, segs) - scoreSameRowAdjacent*len(segs)
				if c == 0 || sc > bestScore {
					best, bestScore = c, sc
				}
			}
			f.take(free, cands[best])
			p.Lines = append(p.Lines, ProposalLine{Segments: cands[best]})
		}
		if len(p.Lines) < f.count {
			continue
		}
		slices.SortStableFunc(p.Lines, func(a, b ProposalLine) int {
			return tieBreak(f.byID[a.Segments[0].SpotID], f.byID[b.Segments[0].SpotID])
		})
		for _, l := range p.Lines {
			p.SpotChanges += l.Changes()
		}
		p.Adjacent = p.SpotChanges == 0 && f.adjacent(p)
		p.score = f.groupScore(p)
		out = append(out, p)
	}
	return out
}

// nearness is the best proximity between the spots of segs and those of p.
func (f *finder) nearness(p Proposal, segs []Segment) int {
	best := 0
	for _, sg := range segs {
		for _, l := range p.Lines {
			for _, o := range l.Segments {
				best = max(best, proximity(f.byID[sg.SpotID], f.byID[o.SpotID]))
			}
		}
	}
	return best
}

// adjacent reports that the spots of p (one segment per line) form one run
// of consecutive numbers in one row.
func (f *finder) adjacent(p Proposal) bool {
	spots := make([]Spot, 0, len(p.Lines))
	for _, l := range p.Lines {
		if len(l.Segments) != 1 {
			return false
		}
		spots = append(spots, f.byID[l.Segments[0].SpotID])
	}
	slices.SortFunc(spots, tieBreak)
	for i := 1; i < len(spots); i++ {
		if spots[i].Row == "" || spots[i].Row != spots[0].Row || spots[i].Number-spots[i-1].Number != 1 {
			return false
		}
	}
	return true
}

func (f *finder) groupScore(p Proposal) int {
	sc := 0
	for i, l := range p.Lines {
		sc += f.lineScore(l.Segments)
		for j := range i {
			sc += proximity(f.byID[l.Segments[0].SpotID], f.byID[p.Lines[j].Segments[0].SpotID]) / scoreGroupDivisor
		}
	}
	return sc
}

// take marks the days of segs as used on their spots.
func (f *finder) take(free [][]bool, segs []Segment) {
	for _, sg := range segs {
		i := f.idx[sg.SpotID]
		for k, d := range f.dates {
			if !d.Before(sg.Interval.From) && !d.After(sg.Interval.To) {
				free[i][k] = false
			}
		}
	}
}

func cloneFree(free [][]bool) [][]bool {
	out := make([][]bool, len(free))
	for i := range free {
		out[i] = slices.Clone(free[i])
	}
	return out
}

// compare orders proposals: fewer changes, adjacent groups, higher score,
// then the spots in tie-break order.
func (f *finder) compare(a, b Proposal) int {
	if c := cmp.Compare(a.SpotChanges, b.SpotChanges); c != 0 {
		return c
	}
	if a.Adjacent != b.Adjacent {
		if a.Adjacent {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(b.score, a.score); c != 0 {
		return c
	}
	as, bs := a.SpotIDs(), b.SpotIDs()
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return tieBreak(f.byID[as[i]], f.byID[bs[i]])
		}
	}
	return cmp.Compare(len(as), len(bs))
}
