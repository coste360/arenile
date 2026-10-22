package availability

import (
	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

// DateSlot identifies a half-day: a calendar date and a slot.
type DateSlot struct {
	Date calendar.Date `json:"date"`
	Slot calendar.Slot `json:"slot"`
}

func (k DateSlot) key() string { return k.Date.String() + "/" + string(k.Slot) }

// Interval is a booked span: dates from From to To inclusive, with Slots on
// each date.
type Interval struct {
	From  calendar.Date
	To    calendar.Date
	Slots calendar.Slots
}

// DateSlots expands the interval into its half-days, in chronological order
// (AM before PM). It returns nil for an empty or invalid interval.
func (i Interval) DateSlots() []DateSlot {
	slots := i.Slots.Expand()
	dates := calendar.Range(i.From, i.To)
	out := make([]DateSlot, 0, len(dates)*len(slots))
	for _, d := range dates {
		for _, s := range slots {
			out = append(out, DateSlot{Date: d, Slot: s})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Days returns the number of dates covered (0 if To is before From).
func (i Interval) Days() int {
	if i.To.Before(i.From) {
		return 0
	}
	return i.From.DaysUntil(i.To) + 1
}

// FreeSpots records which spots are free on which half-days. It is built by
// the store from the active spots minus the valid allocations (spec 7.2, INV-04)
// and consumed by the pure functions of this package. A spot/half-day that was
// never added is considered occupied. The zero value is not usable: use
// NewFreeSpots.
type FreeSpots struct {
	m map[string]map[uuid.UUID]struct{}
}

// NewFreeSpots returns an empty set.
func NewFreeSpots() *FreeSpots { return &FreeSpots{m: map[string]map[uuid.UUID]struct{}{}} }

// Add marks spots as free on the given half-day.
func (f *FreeSpots) Add(d calendar.Date, s calendar.Slot, spots ...uuid.UUID) {
	k := DateSlot{d, s}.key()
	set := f.m[k]
	if set == nil {
		set = make(map[uuid.UUID]struct{}, len(spots))
		f.m[k] = set
	}
	for _, id := range spots {
		set[id] = struct{}{}
	}
}

// AddInterval marks spots as free on every half-day of the interval.
func (f *FreeSpots) AddInterval(i Interval, spots ...uuid.UUID) {
	for _, ds := range i.DateSlots() {
		f.Add(ds.Date, ds.Slot, spots...)
	}
}

// Remove marks a spot as occupied on the given half-day.
func (f *FreeSpots) Remove(d calendar.Date, s calendar.Slot, spot uuid.UUID) {
	if set := f.m[DateSlot{d, s}.key()]; set != nil {
		delete(set, spot)
	}
}

// IsFree reports whether spot is free on the half-day.
func (f *FreeSpots) IsFree(spot uuid.UUID, d calendar.Date, s calendar.Slot) bool {
	if f == nil {
		return false
	}
	_, ok := f.m[DateSlot{d, s}.key()][spot]
	return ok
}

// Count returns the number of free spots on the half-day.
func (f *FreeSpots) Count(d calendar.Date, s calendar.Slot) int {
	if f == nil {
		return 0
	}
	return len(f.m[DateSlot{d, s}.key()])
}

// Conflicts returns the half-days of the interval on which spot is not free,
// in chronological order.
func (f *FreeSpots) Conflicts(spot uuid.UUID, i Interval) []DateSlot {
	var out []DateSlot
	for _, ds := range i.DateSlots() {
		if !f.IsFree(spot, ds.Date, ds.Slot) {
			out = append(out, ds)
		}
	}
	return out
}

// FreeFor reports whether spot is free on every half-day of the interval. An
// empty interval is never free.
func (f *FreeSpots) FreeFor(spot uuid.UUID, i Interval) bool {
	ds := i.DateSlots()
	if len(ds) == 0 {
		return false
	}
	for _, x := range ds {
		if !f.IsFree(spot, x.Date, x.Slot) {
			return false
		}
	}
	return true
}

// Clone returns a deep copy.
func (f *FreeSpots) Clone() *FreeSpots {
	out := NewFreeSpots()
	if f == nil {
		return out
	}
	for k, set := range f.m {
		c := make(map[uuid.UUID]struct{}, len(set))
		for id := range set {
			c[id] = struct{}{}
		}
		out.m[k] = c
	}
	return out
}

// SpotState is the public availability state of a spot over an interval.
type SpotState string

// Spot states (spec 7.8).
const (
	SpotAvailable   SpotState = "available"
	SpotUnavailable SpotState = "unavailable"
)

// SpotStatus is the availability of one spot over an interval.
type SpotStatus struct {
	SpotID uuid.UUID
	State  SpotState
}

// SeatAvailability computes, for a seat-mode zone, the state of each spot over
// the interval: available only if free on every date and slot requested
// (spec 7.8). The result follows the order of spots.
func SeatAvailability(spots []uuid.UUID, free *FreeSpots, i Interval) []SpotStatus {
	out := make([]SpotStatus, len(spots))
	for n, id := range spots {
		st := SpotUnavailable
		if free.FreeFor(id, i) {
			st = SpotAvailable
		}
		out[n] = SpotStatus{SpotID: id, State: st}
	}
	return out
}

// Quantities holds an available quantity per half-day (for zone-mode zones and
// quantity pools). Half-days never set count as 0. The zero value is not
// usable: use NewQuantities.
type Quantities struct {
	m map[string]int
}

// NewQuantities returns an empty set of quantities.
func NewQuantities() *Quantities { return &Quantities{m: map[string]int{}} }

// Set records the available quantity on a half-day.
func (q *Quantities) Set(d calendar.Date, s calendar.Slot, n int) { q.m[DateSlot{d, s}.key()] = n }

// Get returns the available quantity on a half-day (0 if unknown).
func (q *Quantities) Get(d calendar.Date, s calendar.Slot) int {
	if q == nil {
		return 0
	}
	return q.m[DateSlot{d, s}.key()]
}

// MinAvailable returns the minimum quantity over every half-day of the
// interval, never below 0 (spec 7.8). An empty interval yields 0.
func MinAvailable(q *Quantities, i Interval) int {
	ds := i.DateSlots()
	if len(ds) == 0 {
		return 0
	}
	m := q.Get(ds[0].Date, ds[0].Slot)
	for _, x := range ds[1:] {
		m = min(m, q.Get(x.Date, x.Slot))
	}
	return max(m, 0)
}

// ZoneFree builds the quantity available in a zone-mode zone per half-day from
// the free spots: the number of free spots of the zone minus the lines still
// pending assignment on that half-day (INV-08). pending may be nil.
func ZoneFree(zoneSpots []uuid.UUID, free *FreeSpots, pending *Quantities, i Interval) *Quantities {
	out := NewQuantities()
	for _, ds := range i.DateSlots() {
		n := 0
		for _, id := range zoneSpots {
			if free.IsFree(id, ds.Date, ds.Slot) {
				n++
			}
		}
		out.Set(ds.Date, ds.Slot, max(n-pending.Get(ds.Date, ds.Slot), 0))
	}
	return out
}
