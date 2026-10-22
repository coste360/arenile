package availability

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

// id builds a deterministic UUID whose string order matches n.
func id(n int) uuid.UUID {
	var u uuid.UUID
	u[14], u[15] = byte(n>>8), byte(n)
	return u
}

var (
	zone1 = id(9001)
	zone2 = id(9002)
	day1  = calendar.MustParse("2027-08-01")
)

func d(n int) calendar.Date { return day1.AddDays(n - 1) }

func iv(from, to int, s calendar.Slots) Interval { return Interval{From: d(from), To: d(to), Slots: s} }

// fixture is a beach with rows A and B of 5 umbrellas each (zone1), one gazebo
// G1 (zone1) and one umbrella Z1 in zone2. Spot IDs: A1=101.., B1=201.., G1=301, Z1=401.
type fixture struct {
	spots   []Spot
	byLabel map[string]Spot
}

func newFixture() fixture {
	f := fixture{byLabel: map[string]Spot{}}
	add := func(s Spot) {
		f.spots = append(f.spots, s)
		f.byLabel[s.Label] = s
	}
	for r, row := range []string{"A", "B"} {
		for n := 1; n <= 5; n++ {
			add(Spot{
				ID: id(100*(r+1) + n), ZoneID: zone1, Label: fmt.Sprintf("%s%d", row, n), Row: row, Number: n,
				Kind: "umbrella", X: float64(n * 10), Y: float64(r * 10),
			})
		}
	}
	add(Spot{ID: id(301), ZoneID: zone1, Label: "G1", Row: "G", Number: 1, Kind: "gazebo", X: 80, Y: 0})
	add(Spot{ID: id(401), ZoneID: zone2, Label: "Z1", Row: "Z", Number: 1, Kind: "umbrella", X: 0, Y: 50})
	f.byLabel["B5"] = withAttrs(&f, "B5", "accessible")
	return f
}

func withAttrs(f *fixture, label string, attrs ...string) Spot {
	for i := range f.spots {
		if f.spots[i].Label == label {
			f.spots[i].Attributes = attrs
			return f.spots[i]
		}
	}
	panic(label)
}

func (f fixture) id(label string) uuid.UUID { return f.byLabel[label].ID }

func (f fixture) label(u uuid.UUID) string {
	for _, s := range f.spots {
		if s.ID == u {
			return s.Label
		}
	}
	return u.String()
}

// allFree marks every spot free on days 1..7, both slots.
func (f fixture) allFree() *FreeSpots {
	fs := NewFreeSpots()
	for _, s := range f.spots {
		fs.AddInterval(iv(1, 7, calendar.SlotsFull), s.ID)
	}
	return fs
}

// occupy removes labels from the free set on days from..to (both slots).
func (f fixture) occupy(fs *FreeSpots, from, to int, labels ...string) {
	for _, ds := range iv(from, to, calendar.SlotsFull).DateSlots() {
		for _, l := range labels {
			fs.Remove(ds.Date, ds.Slot, f.id(l))
		}
	}
}

func line(n int, booking int, i Interval) Line {
	return Line{ID: id(1000 + n), BookingID: id(5000 + booking), CustomerID: id(7000 + booking), ZoneID: zone1, Interval: i}
}

// describe renders the result as {line#: "A1" | "A1@1-3+A2@4-7" | "!reason"}.
func (f fixture) describe(r AssignResult) map[int]string {
	out := map[int]string{}
	for _, a := range r.Assignments {
		n := int(a.LineID[14])<<8 | int(a.LineID[15]) - 1000
		if !a.Split() {
			out[n] = f.label(a.SpotID())
			continue
		}
		parts := make([]string, len(a.Segments))
		for i, s := range a.Segments {
			parts[i] = fmt.Sprintf("%s@%d-%d", f.label(s.SpotID), day1.DaysUntil(s.Interval.From)+1, day1.DaysUntil(s.Interval.To)+1)
		}
		out[n] = strings.Join(parts, "+")
	}
	for _, u := range r.Unassigned {
		n := int(u.LineID[14])<<8 | int(u.LineID[15]) - 1000
		out[n] = "!" + string(u.Reason)
	}
	return out
}

func TestAutoAssign_Table(t *testing.T) {
	full, am, pm := calendar.SlotsFull, calendar.SlotsAM, calendar.SlotsPM
	tests := []struct {
		name  string
		build func(f fixture) AssignInput
		want  map[int]string
	}{
		{"single line all free takes first by row and number", func(f fixture) AssignInput {
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 3, full))}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "A1"}},
		{"occupied on one date is skipped", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 2, 2, "A1")
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 3, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A2"}}, // A2 also earns the compact bonus (A1 occupied on day 2)
		{"continuity with previous spot", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 7, full))
			l.PreviousSpotID = f.id("B3")
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "B3"}},
		{"previous spot occupied falls back", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 4, 4, "B3")
			l := line(1, 1, iv(1, 7, full))
			l.PreviousSpotID = f.id("B3")
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "B2"}}, // next to the partially occupied B3
		{"preferred attribute", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.PreferredAttributes = []string{"accessible"}
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "B5"}},
		{"duplicate preferences count once", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.PreferredAttributes = []string{"accessible", "accessible", "accessible"}
			l.PreviousSpotID = f.id("A4")
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "A4"}},
		{"continuity beats attribute", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.PreferredAttributes = []string{"accessible"}
			l.PreviousSpotID = f.id("A2")
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "A2"}},
		{"sibling already placed attracts", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "B4")
			return AssignInput{
				Lines: []Line{line(1, 1, iv(1, 2, full))}, Spots: f.spots, Free: fs,
				Placed: []Placement{{BookingID: id(5001), LineID: id(1099), SpotID: f.id("B4")}},
			}
		}, map[int]string{1: "B3"}},
		{"sibling of another booking does not attract", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "B4")
			return AssignInput{
				Lines: []Line{line(1, 1, iv(1, 2, full))}, Spots: f.spots, Free: fs,
				Placed: []Placement{{BookingID: id(5999), LineID: id(1099), SpotID: f.id("B4")}},
			}
		}, map[int]string{1: "B3"}}, // still B3/B5 by compact filling (neighbour occupied): B3 first
		{"proximity beats attribute", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "A1")
			l := line(1, 1, iv(1, 2, full))
			l.PreferredAttributes = []string{"accessible"}
			return AssignInput{
				Lines: []Line{l}, Spots: f.spots, Free: fs,
				Placed: []Placement{{BookingID: id(5001), LineID: id(1099), SpotID: f.id("A1")}},
			}
		}, map[int]string{1: "A2"}},
		{"sibling in another row uses coordinates", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "A1", "A2", "A3", "A4", "A5", "G1")
			return AssignInput{
				Lines: []Line{line(1, 1, iv(1, 2, full))}, Spots: f.spots, Free: fs,
				Placed: []Placement{{BookingID: id(5001), LineID: id(1099), SpotID: f.id("A4")}},
			}
		}, map[int]string{1: "B4"}},
		{"two lines of the same booking sit together", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "A1", "A2", "A3")
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 3, full)), line(2, 1, iv(1, 3, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A4", 2: "A5"}},
		{"longest line first regardless of input order", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 7, full), f.id("A1"))
			fs.AddInterval(iv(1, 1, full), f.id("B1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, full)), line(2, 2, iv(1, 7, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "B1", 2: "A1"}},
		{"spot kind filter", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.SpotKinds = []string{"gazebo"}
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "G1"}},
		{"zone filter", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.ZoneID = zone2
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "Z1"}},
		{"nil zone means any spot", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.ZoneID = uuid.Nil
			l.PreviousSpotID = f.id("Z1")
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "Z1"}},
		{"no compatible spot", func(f fixture) AssignInput {
			l := line(1, 1, iv(1, 2, full))
			l.SpotKinds = []string{"tent"}
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "!no_compatible_spot"}},
		{"no spots at all", func(f fixture) AssignInput {
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 2, full))}, Free: f.allFree()}
		}, map[int]string{1: "!no_compatible_spot"}},
		{"zone full without split", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 3, full), f.id("A1"))
			fs.AddInterval(iv(4, 7, full), f.id("A2"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 7, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "!no_spot_for_whole_period"}},
		{"split allowed", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 3, full), f.id("A1"))
			fs.AddInterval(iv(4, 7, full), f.id("A2"))
			l := line(1, 1, iv(1, 7, full))
			l.AllowSpotChange = true
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A1@1-3+A2@4-7"}},
		{"split uses fewest segments", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 2, full), f.id("A1"))
			fs.AddInterval(iv(1, 6, full), f.id("A2"))
			fs.AddInterval(iv(3, 7, full), f.id("A3"))
			l := line(1, 1, iv(1, 7, full))
			l.AllowSpotChange = true
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A2@1-6+A3@7-7"}},
		{"split impossible on a date", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 4, full), f.id("A1"))
			fs.AddInterval(iv(6, 7, full), f.id("A2"))
			l := line(1, 1, iv(1, 7, full))
			l.AllowSpotChange = true
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "!no_spot_on_some_dates"}},
		{"split not applicable to one day", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.Add(d(1), calendar.AM, f.id("A1"))
			fs.Add(d(1), calendar.PM, f.id("A2"))
			l := line(1, 1, iv(1, 1, full))
			l.AllowSpotChange = true
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "!no_spot_for_whole_period"}},
		{"whole period preferred over split", func(f fixture) AssignInput {
			fs := f.allFree()
			l := line(1, 1, iv(1, 7, full))
			l.AllowSpotChange = true
			return AssignInput{Lines: []Line{l}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A1"}},
		{"invalid interval", func(f fixture) AssignInput {
			return AssignInput{Lines: []Line{line(1, 1, iv(3, 1, full))}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "!invalid_interval"}},
		{"invalid slots", func(f fixture) AssignInput {
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, calendar.Slots("XX")))}, Spots: f.spots, Free: f.allFree()}
		}, map[int]string{1: "!invalid_interval"}},
		{"AM line ignores PM occupancy", func(f fixture) AssignInput {
			fs := f.allFree()
			fs.Remove(d(1), calendar.PM, f.id("A1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, am))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A1"}},
		{"FULL line needs both slots", func(f fixture) AssignInput {
			fs := f.allFree()
			fs.Remove(d(1), calendar.PM, f.id("A1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A2"}},
		{"AM and PM lines share a spot", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 1, full), f.id("A1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, am)), line(2, 2, iv(1, 1, pm))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A1", 2: "A1"}},
		{"compact filling next to occupied", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "B2")
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 7, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "B1"}},
		{"partial neighbour occupancy weighs less", func(f fixture) AssignInput {
			fs := f.allFree()
			f.occupy(fs, 1, 7, "B2")
			f.occupy(fs, 1, 1, "A4")
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 7, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "B1"}},
		{"one spot two bookings", func(f fixture) AssignInput {
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 2, full), f.id("A1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 2, full)), line(2, 2, iv(2, 2, full))}, Spots: f.spots, Free: fs}
		}, map[int]string{1: "A1", 2: "!no_spot_for_whole_period"}},
		{"nil free set", func(f fixture) AssignInput {
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 2, full))}, Spots: f.spots}
		}, map[int]string{1: "!no_spot_for_whole_period"}},
		{"duplicate spot ids are ignored", func(f fixture) AssignInput {
			spots := append([]Spot{}, f.spots...)
			dup := f.byLabel["A1"]
			dup.Label = "A1-dup"
			spots = append(spots, dup)
			fs := NewFreeSpots()
			fs.AddInterval(iv(1, 1, full), f.id("A1"))
			return AssignInput{Lines: []Line{line(1, 1, iv(1, 1, full)), line(2, 2, iv(1, 1, full))}, Spots: spots, Free: fs}
		}, map[int]string{1: "A1", 2: "!no_spot_for_whole_period"}},
		{"no lines", func(f fixture) AssignInput {
			return AssignInput{Spots: f.spots, Free: f.allFree()}
		}, map[int]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			got := f.describe(AutoAssign(tc.build(f)))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAutoAssign_DoesNotMutateInput(t *testing.T) {
	f := newFixture()
	fs := f.allFree()
	before := fs.Clone()
	lines := []Line{line(2, 1, iv(1, 1, calendar.SlotsFull)), line(1, 1, iv(1, 7, calendar.SlotsFull))}
	linesCopy := append([]Line{}, lines...)
	AutoAssign(AssignInput{Lines: lines, Spots: f.spots, Free: fs})
	if !reflect.DeepEqual(fs, before) {
		t.Fatal("free set mutated")
	}
	if !reflect.DeepEqual(lines, linesCopy) {
		t.Fatal("lines reordered")
	}
}

func TestAssignment_Accessors(t *testing.T) {
	var empty Assignment
	if empty.SpotID() != uuid.Nil || empty.Split() {
		t.Fatal("empty assignment")
	}
	a := Assignment{Segments: []Segment{{SpotID: id(1)}, {SpotID: id(2)}}}
	if a.SpotID() != id(1) || !a.Split() {
		t.Fatal("split assignment")
	}
}
