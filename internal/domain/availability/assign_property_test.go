package availability

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
)

type genCase struct {
	in      AssignInput
	initial *FreeSpots
}

func genAssignInput(t *rapid.T) genCase {
	nRows := rapid.IntRange(1, 3).Draw(t, "rows")
	perRow := rapid.IntRange(1, 6).Draw(t, "perRow")
	attrs := []string{"accessible", "shade_morning", "front"}
	var spots []Spot
	for r := range nRows {
		for n := 1; n <= perRow; n++ {
			s := Spot{
				ID: id(100*(r+1) + n), ZoneID: zone1, Label: fmt.Sprintf("%c%d", 'A'+r, n),
				Row: string(rune('A' + r)), Number: n, Kind: rapid.SampledFrom([]string{"umbrella", "gazebo"}).Draw(t, "kind"),
				X: float64(n * 10), Y: float64(r * 10),
			}
			if rapid.Bool().Draw(t, "hasAttr") {
				s.Attributes = []string{rapid.SampledFrom(attrs).Draw(t, "attr")}
			}
			spots = append(spots, s)
		}
	}
	free := NewFreeSpots()
	for _, s := range spots {
		for _, ds := range iv(1, 7, calendar.SlotsFull).DateSlots() {
			if rapid.IntRange(0, 4).Draw(t, "free") > 0 {
				free.Add(ds.Date, ds.Slot, s.ID)
			}
		}
	}
	nLines := rapid.IntRange(0, 12).Draw(t, "lines")
	var lines []Line
	for i := range nLines {
		from := rapid.IntRange(1, 7).Draw(t, "from")
		to := rapid.IntRange(from, 7).Draw(t, "to")
		l := line(i+1, rapid.IntRange(1, 4).Draw(t, "booking"),
			iv(from, to, rapid.SampledFrom([]calendar.Slots{calendar.SlotsAM, calendar.SlotsPM, calendar.SlotsFull}).Draw(t, "slots")))
		l.AllowSpotChange = rapid.Bool().Draw(t, "split")
		if rapid.Bool().Draw(t, "pref") {
			l.PreferredAttributes = []string{rapid.SampledFrom(attrs).Draw(t, "prefAttr")}
		}
		if rapid.Bool().Draw(t, "prev") {
			l.PreviousSpotID = rapid.SampledFrom(spots).Draw(t, "prevSpot").ID
		}
		if rapid.IntRange(0, 3).Draw(t, "kindFilter") == 0 {
			l.SpotKinds = []string{"gazebo"}
		}
		lines = append(lines, l)
	}
	return genCase{in: AssignInput{Lines: lines, Spots: spots, Free: free}, initial: free.Clone()}
}

// TestAutoAssign_NeverExceedsZoneCapacity checks INV-08 for the pure
// assignment: assigned spots were free, no spot/half-day is used twice, every
// half-day of every assigned line is covered exactly once, and every line is
// either assigned or reported.
func TestAutoAssign_NeverExceedsZoneCapacity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := genAssignInput(t)
		res := AutoAssign(g.in)
		if len(res.Assignments)+len(res.Unassigned) != len(g.in.Lines) {
			t.Fatalf("lines lost: %d+%d != %d", len(res.Assignments), len(res.Unassigned), len(g.in.Lines))
		}
		byID := map[uuid.UUID]Line{}
		for _, l := range g.in.Lines {
			byID[l.ID] = l
		}
		used := map[string]uuid.UUID{}
		for _, a := range res.Assignments {
			l := byID[a.LineID]
			if a.Split() && !l.AllowSpotChange {
				t.Fatalf("line %s split without permission", a.LineID)
			}
			covered := 0
			prevTo := l.Interval.From.AddDays(-1)
			for _, sg := range a.Segments {
				if !sg.Interval.From.Equal(prevTo.AddDays(1)) {
					t.Fatalf("segments not contiguous for line %s", a.LineID)
				}
				prevTo = sg.Interval.To
				for _, ds := range sg.Interval.DateSlots() {
					if !g.initial.IsFree(sg.SpotID, ds.Date, ds.Slot) {
						t.Fatalf("spot %s not free on %v", sg.SpotID, ds)
					}
					k := sg.SpotID.String() + ds.key()
					if other, dup := used[k]; dup {
						t.Fatalf("spot %s on %v given to %s and %s", sg.SpotID, ds, other, a.LineID)
					}
					used[k] = a.LineID
					covered++
				}
			}
			if !prevTo.Equal(l.Interval.To) || covered != len(l.Interval.DateSlots()) {
				t.Fatalf("line %s not fully covered", a.LineID)
			}
		}
		// Capacity: per half-day, assignments never exceed initially free spots.
		perSlot := map[string]int{}
		for k := range used {
			perSlot[k[36:]]++
		}
		for _, ds := range iv(1, 7, calendar.SlotsFull).DateSlots() {
			if perSlot[ds.key()] > g.initial.Count(ds.Date, ds.Slot) {
				t.Fatalf("capacity exceeded on %v", ds)
			}
		}
		if !reflect.DeepEqual(g.in.Free, g.initial) {
			t.Fatal("input free set mutated")
		}
	})
}

func TestAutoAssign_PropertyDeterministicAndOrderIndependent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		g := genAssignInput(t)
		first := AutoAssign(g.in)
		again := AutoAssign(g.in)
		if !reflect.DeepEqual(first, again) {
			t.Fatal("not deterministic")
		}
		seed := rapid.Uint64().Draw(t, "seed")
		r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		shuffled := g.in
		shuffled.Lines = append([]Line{}, g.in.Lines...)
		shuffled.Spots = append([]Spot{}, g.in.Spots...)
		r.Shuffle(len(shuffled.Lines), func(i, j int) { shuffled.Lines[i], shuffled.Lines[j] = shuffled.Lines[j], shuffled.Lines[i] })
		r.Shuffle(len(shuffled.Spots), func(i, j int) { shuffled.Spots[i], shuffled.Spots[j] = shuffled.Spots[j], shuffled.Spots[i] })
		if got := AutoAssign(shuffled); !reflect.DeepEqual(first, got) {
			t.Fatalf("result depends on input order:\n%v\n%v", first, got)
		}
	})
}

func TestAutoAssign_PropertyAssignsWhenPossible(t *testing.T) {
	// A single line is left unassigned only if no compatible spot is free for
	// the whole stay (and, when splitting is allowed, some date has no spot).
	rapid.Check(t, func(t *rapid.T) {
		g := genAssignInput(t)
		if len(g.in.Lines) == 0 {
			return
		}
		l := g.in.Lines[0]
		in := g.in
		in.Lines = []Line{l}
		res := AutoAssign(in)
		if len(res.Unassigned) == 0 {
			return
		}
		for _, s := range in.Spots {
			if (len(l.SpotKinds) == 0 || s.Kind == l.SpotKinds[0]) && g.initial.FreeFor(s.ID, l.Interval) {
				t.Fatalf("line unassigned (%s) although %s is free", res.Unassigned[0].Reason, s.Label)
			}
		}
	})
}
