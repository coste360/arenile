package availability

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/coste360/arenile/internal/calendar"
)

// umbrellas are the spots of rows A and B of the fixture.
func (f fixture) umbrellas() []Spot {
	var out []Spot
	for _, s := range f.spots {
		if s.Row == "A" || s.Row == "B" {
			out = append(out, s)
		}
	}
	return out
}

// describeProposals renders proposals as "A1" | "A2@1-3+A3@4-7", the lines
// of a proposal joined by ",".
func (f fixture) describeProposals(ps []Proposal) []string {
	out := []string{}
	for _, p := range ps {
		var lines []string
		for _, l := range p.Lines {
			if len(l.Segments) == 1 {
				lines = append(lines, f.label(l.Segments[0].SpotID))
				continue
			}
			var segs []string
			for _, sg := range l.Segments {
				segs = append(segs, fmt.Sprintf("%s@%d-%d", f.label(sg.SpotID), day1.DaysUntil(sg.Interval.From)+1, day1.DaysUntil(sg.Interval.To)+1))
			}
			lines = append(lines, strings.Join(segs, "+"))
		}
		out = append(out, strings.Join(lines, ","))
	}
	return out
}

func TestFindSpots_Ranking(t *testing.T) {
	fx := newFixture()
	allBut := func(keep ...string) []string {
		var out []string
		for _, s := range fx.umbrellas() {
			if !slices.Contains(keep, s.Label) {
				out = append(out, s.Label)
			}
		}
		return out
	}
	tests := []struct {
		name    string
		in      FindInput
		occupy  func(fs *FreeSpots)
		want    []string
		changes []int
	}{
		{
			name: "all free: whole-stay spots in row order",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), Limit: 3},
			want: []string{"A1", "A2", "A3"}, changes: []int{0, 0, 0},
		},
		{
			name: "continuity with the previous spot first",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), Limit: 2, PreviousSpotID: fx.id("B3")},
			want: []string{"B3", "A1"},
		},
		{
			name: "required attributes filter the spots",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), RequiredAttributes: []string{"accessible"}},
			want: []string{"B5"},
		},
		{
			name: "no change allowed and no spot free for the stay",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull)},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 4, 7, allBut("A3")...)
				fx.occupy(fs, 1, 3, "A3")
			},
			want: []string{},
		},
		{
			name: "one change: nearest spot before the free one",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), MaxSpotChanges: 1, Limit: 2},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 4, 7, allBut("A3")...)
				fx.occupy(fs, 1, 3, "A3")
			},
			want: []string{"A2@1-3+A3@4-7", "A4@1-3+A3@4-7"}, changes: []int{1, 1},
		},
		{
			name: "whole-stay spot before any split",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), MaxSpotChanges: 2, Limit: 2},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 4, 7, allBut("A3", "B5")...)
				fx.occupy(fs, 1, 3, "A3")
			},
			want: []string{"B5", "A2@1-3+A3@4-7"}, changes: []int{0, 1},
		},
		{
			name: "fewer changes first",
			in:   FindInput{Interval: iv(1, 6, calendar.SlotsFull), MaxSpotChanges: 3, Limit: 1},
			occupy: func(fs *FreeSpots) {
				// A1 free 1-2, A2 free 3-4, A3 free 5-6 (two changes);
				// B1 free 1-3, B2 free 4-6 (one change).
				fx.occupy(fs, 1, 6, allBut("A1", "A2", "A3", "B1", "B2")...)
				fx.occupy(fs, 3, 6, "A1")
				fx.occupy(fs, 1, 2, "A2")
				fx.occupy(fs, 5, 6, "A2")
				fx.occupy(fs, 1, 4, "A3")
				fx.occupy(fs, 4, 6, "B1")
				fx.occupy(fs, 1, 3, "B2")
			},
			want: []string{"B1@1-3+B2@4-6"}, changes: []int{1},
		},
		{
			name: "max changes bounds the segments",
			in:   FindInput{Interval: iv(1, 6, calendar.SlotsFull), MaxSpotChanges: 1},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 1, 6, allBut("A1", "A2", "A3")...)
				fx.occupy(fs, 3, 6, "A1")
				fx.occupy(fs, 1, 2, "A2")
				fx.occupy(fs, 5, 6, "A2")
				fx.occupy(fs, 1, 4, "A3")
			},
			want: []string{},
		},
		{
			name: "group: adjacent window of one row",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), Count: 3, Limit: 2},
			want: []string{"A1,A2,A3", "A2,A3,A4"},
		},
		{
			name: "group: window skips an occupied spot",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), Count: 2, Limit: 1},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 2, 2, "A2")
			},
			want: []string{"A3,A4"},
		},
		{
			name: "group: not adjacent when no window is free",
			in:   FindInput{Interval: iv(1, 7, calendar.SlotsFull), Count: 2, Limit: 1},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 1, 7, allBut("A1", "A3", "B5")...)
			},
			want: []string{"A1,A3"},
		},
		{
			name: "group with changes: lines never share a half-day",
			in:   FindInput{Interval: iv(1, 4, calendar.SlotsFull), Count: 2, MaxSpotChanges: 1, Limit: 1},
			occupy: func(fs *FreeSpots) {
				fx.occupy(fs, 1, 4, allBut("A1", "A2", "A3")...)
				fx.occupy(fs, 3, 4, "A1")
				fx.occupy(fs, 1, 2, "A3")
			},
			want: []string{"A1@1-2+A3@3-4,A2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			free := fx.allFree()
			if tt.occupy != nil {
				tt.occupy(free)
			}
			in := tt.in
			in.Spots, in.Free = fx.umbrellas(), free
			got := FindSpots(in)
			if d := fx.describeProposals(got); !slices.Equal(d, tt.want) {
				t.Fatalf("proposals = %v, want %v", d, tt.want)
			}
			for i, c := range tt.changes {
				if got[i].SpotChanges != c {
					t.Fatalf("proposal %d: changes = %d, want %d", i, got[i].SpotChanges, c)
				}
			}
			if tt.in.Count > 1 && len(got) > 0 && strings.Contains(tt.name, "adjacent window") && !got[0].Adjacent {
				t.Fatalf("first group proposal not adjacent")
			}
		})
	}
}

// The ranking does not depend on the order of the input spots.
func TestFindSpots_DeterministicOrder(t *testing.T) {
	fx := newFixture()
	free := fx.allFree()
	fx.occupy(free, 3, 5, "A1", "A2", "B1")
	in := FindInput{Interval: iv(1, 7, calendar.SlotsFull), MaxSpotChanges: 2, Count: 2, Free: free}
	in.Spots = fx.umbrellas()
	a := fx.describeProposals(FindSpots(in))
	slices.Reverse(in.Spots)
	b := fx.describeProposals(FindSpots(in))
	if !slices.Equal(a, b) {
		t.Fatalf("order dependent:\n%v\n%v", a, b)
	}
}

// TestFindSpots_SegmentsCoverStay is INV-102 for the pure search: every
// line of every proposal covers each half-day of the stay exactly once with
// consecutive segments on spots free on those half-days, never more than
// MaxSpotChanges changes, and the lines of a proposal never share a
// spot/half-day.
func TestFindSpots_SegmentsCoverStay(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nSpots := rapid.IntRange(1, 8).Draw(t, "spots")
		var spots []Spot
		for n := 1; n <= nSpots; n++ {
			spots = append(spots, Spot{ID: id(100 + n), ZoneID: zone1, Label: fmt.Sprintf("A%d", n), Row: "A", Number: n, X: float64(10 * n)})
		}
		days := rapid.IntRange(1, 9).Draw(t, "days")
		slots := rapid.SampledFrom([]calendar.Slots{calendar.SlotsAM, calendar.SlotsPM, calendar.SlotsFull}).Draw(t, "slots")
		stay := iv(1, days, slots)
		free := NewFreeSpots()
		for _, s := range spots {
			for _, ds := range iv(1, days, calendar.SlotsFull).DateSlots() {
				if rapid.IntRange(0, 3).Draw(t, "free") > 0 {
					free.Add(ds.Date, ds.Slot, s.ID)
				}
			}
		}
		in := FindInput{
			Spots: spots, Free: free, Interval: stay,
			Count:          rapid.IntRange(1, 3).Draw(t, "count"),
			MaxSpotChanges: rapid.IntRange(0, 3).Draw(t, "changes"),
			Limit:          rapid.IntRange(1, 20).Draw(t, "limit"),
		}
		props := FindSpots(in)
		if len(props) > in.Limit {
			t.Fatalf("%d proposals over limit %d", len(props), in.Limit)
		}
		keys := map[string]bool{}
		for _, p := range props {
			if keys[p.Key()] {
				t.Fatalf("duplicate proposal %s", p.Key())
			}
			keys[p.Key()] = true
			if len(p.Lines) != in.Count {
				t.Fatalf("%d lines, want %d", len(p.Lines), in.Count)
			}
			used := map[string]bool{}
			total := 0
			for _, l := range p.Lines {
				if l.Changes() > in.MaxSpotChanges {
					t.Fatalf("line with %d changes > %d", l.Changes(), in.MaxSpotChanges)
				}
				total += l.Changes()
				covered := map[string]int{}
				next := stay.From
				for j, sg := range l.Segments {
					if !sg.Interval.From.Equal(next) || sg.Interval.To.Before(sg.Interval.From) || sg.Interval.Slots != slots {
						t.Fatalf("segment %d not consecutive: %+v (expected from %s)", j, sg.Interval, next)
					}
					if j > 0 && l.Segments[j-1].SpotID == sg.SpotID {
						t.Fatalf("two consecutive segments on the same spot")
					}
					next = sg.Interval.To.AddDays(1)
					for _, ds := range sg.Interval.DateSlots() {
						if !free.IsFree(sg.SpotID, ds.Date, ds.Slot) {
							t.Fatalf("spot %s not free on %v", sg.SpotID, ds)
						}
						k := ds.key()
						covered[k]++
						sk := sg.SpotID.String() + k
						if used[sk] {
							t.Fatalf("spot %s used twice on %v", sg.SpotID, ds)
						}
						used[sk] = true
					}
				}
				for _, ds := range stay.DateSlots() {
					if covered[ds.key()] != 1 {
						t.Fatalf("half-day %v covered %d times", ds, covered[ds.key()])
					}
				}
			}
			if total != p.SpotChanges {
				t.Fatalf("spot changes %d, lines say %d", p.SpotChanges, total)
			}
		}
		// Completeness for a single line without changes: every spot free
		// for the whole stay is proposed (up to the limit).
		if in.Count == 1 {
			whole := 0
			for _, s := range spots {
				if free.FreeFor(s.ID, stay) {
					whole++
				}
			}
			zero := 0
			for _, p := range props {
				if p.SpotChanges == 0 {
					zero++
				}
			}
			if zero != min(whole, in.Limit) {
				t.Fatalf("%d whole-stay proposals, %d spots free for the stay (limit %d)", zero, whole, in.Limit)
			}
		}
	})
}
