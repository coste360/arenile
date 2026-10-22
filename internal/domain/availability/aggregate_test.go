package availability

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/calendar"
)

func TestInterval_DateSlots(t *testing.T) {
	tests := []struct {
		name string
		i    Interval
		want int
		days int
	}{
		{"one full day", iv(1, 1, calendar.SlotsFull), 2, 1},
		{"one AM", iv(1, 1, calendar.SlotsAM), 1, 1},
		{"week full", iv(1, 7, calendar.SlotsFull), 14, 7},
		{"week PM", iv(1, 7, calendar.SlotsPM), 7, 7},
		{"reversed", iv(3, 1, calendar.SlotsFull), 0, 0},
		{"invalid slots", iv(1, 3, calendar.Slots("")), 0, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := tc.i.DateSlots()
			if len(ds) != tc.want || tc.i.Days() != tc.days {
				t.Fatalf("len = %d days = %d", len(ds), tc.i.Days())
			}
			if tc.want == 2 && (ds[0].Slot != calendar.AM || ds[1].Slot != calendar.PM) {
				t.Fatalf("order = %v", ds)
			}
		})
	}
}

func TestSeatAvailability_Table(t *testing.T) {
	a, b, c := id(1), id(2), id(3)
	fs := NewFreeSpots()
	fs.AddInterval(iv(1, 7, calendar.SlotsFull), a)
	fs.AddInterval(iv(1, 7, calendar.SlotsAM), b)
	fs.AddInterval(iv(1, 3, calendar.SlotsFull), c)
	fs.AddInterval(iv(5, 7, calendar.SlotsFull), c)
	tests := []struct {
		name string
		i    Interval
		want []SpotState
	}{
		{"full week", iv(1, 7, calendar.SlotsFull), []SpotState{SpotAvailable, SpotUnavailable, SpotUnavailable}},
		{"mornings", iv(1, 7, calendar.SlotsAM), []SpotState{SpotAvailable, SpotAvailable, SpotUnavailable}},
		{"first three days", iv(1, 3, calendar.SlotsFull), []SpotState{SpotAvailable, SpotUnavailable, SpotAvailable}},
		{"gap day only", iv(4, 4, calendar.SlotsPM), []SpotState{SpotAvailable, SpotUnavailable, SpotUnavailable}},
		{"outside known dates", iv(8, 9, calendar.SlotsFull), []SpotState{SpotUnavailable, SpotUnavailable, SpotUnavailable}},
		{"empty interval", iv(3, 2, calendar.SlotsFull), []SpotState{SpotUnavailable, SpotUnavailable, SpotUnavailable}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SeatAvailability([]uuid.UUID{a, b, c}, fs, tc.i)
			for n, s := range got {
				if s.State != tc.want[n] || s.SpotID != []uuid.UUID{a, b, c}[n] {
					t.Fatalf("spot %d = %+v, want %s", n, s, tc.want[n])
				}
			}
		})
	}
}

func TestMinAvailable_Table(t *testing.T) {
	q := NewQuantities()
	for n := 1; n <= 7; n++ {
		q.Set(d(n), calendar.AM, 10+n)
		q.Set(d(n), calendar.PM, 20+n)
	}
	q.Set(d(4), calendar.PM, 2)
	q.Set(d(6), calendar.AM, -3)
	tests := []struct {
		name string
		i    Interval
		want int
	}{
		{"single AM", iv(1, 1, calendar.SlotsAM), 11},
		{"single full", iv(2, 2, calendar.SlotsFull), 12},
		{"PM dip on day 4", iv(1, 5, calendar.SlotsPM), 2},
		{"AM ignores PM dip", iv(1, 5, calendar.SlotsAM), 11},
		{"negative clamps to zero", iv(5, 7, calendar.SlotsFull), 0},
		{"unknown day counts zero", iv(7, 8, calendar.SlotsPM), 0},
		{"empty interval", iv(2, 1, calendar.SlotsFull), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MinAvailable(q, tc.i); got != tc.want {
				t.Fatalf("MinAvailable = %d, want %d", got, tc.want)
			}
		})
	}
	if MinAvailable(nil, iv(1, 1, calendar.SlotsAM)) != 0 {
		t.Fatal("nil quantities")
	}
}

func TestZoneFree_Scenarios(t *testing.T) {
	spots := []uuid.UUID{id(1), id(2), id(3)}
	fs := NewFreeSpots()
	fs.AddInterval(iv(1, 3, calendar.SlotsFull), spots...)
	fs.Remove(d(2), calendar.AM, id(1))
	pending := NewQuantities()
	pending.Set(d(3), calendar.PM, 2)
	pending.Set(d(1), calendar.AM, 5)
	got := ZoneFree(spots, fs, pending, iv(1, 3, calendar.SlotsFull))
	want := map[DateSlot]int{
		{d(1), calendar.AM}: 0, {d(1), calendar.PM}: 3,
		{d(2), calendar.AM}: 2, {d(2), calendar.PM}: 3,
		{d(3), calendar.AM}: 3, {d(3), calendar.PM}: 1,
	}
	for k, v := range want {
		if g := got.Get(k.Date, k.Slot); g != v {
			t.Fatalf("%v = %d, want %d", k, g, v)
		}
	}
	if MinAvailable(got, iv(2, 3, calendar.SlotsFull)) != 1 {
		t.Fatal("min over zone")
	}
	if ZoneFree(spots, fs, nil, iv(1, 1, calendar.SlotsAM)).Get(d(1), calendar.AM) != 3 {
		t.Fatal("nil pending")
	}
}

func TestFreeSpots_Scenarios(t *testing.T) {
	a := id(1)
	fs := NewFreeSpots()
	if fs.IsFree(a, d(1), calendar.AM) || fs.Count(d(1), calendar.AM) != 0 {
		t.Fatal("empty set")
	}
	fs.Add(d(1), calendar.AM, a, id(2))
	if !fs.IsFree(a, d(1), calendar.AM) || fs.IsFree(a, d(1), calendar.PM) || fs.Count(d(1), calendar.AM) != 2 {
		t.Fatal("add")
	}
	c := fs.Clone()
	fs.Remove(d(1), calendar.AM, a)
	fs.Remove(d(9), calendar.AM, a) // unknown half-day: no-op
	if fs.IsFree(a, d(1), calendar.AM) || !c.IsFree(a, d(1), calendar.AM) {
		t.Fatal("clone must be independent")
	}
	conf := c.Conflicts(a, iv(1, 2, calendar.SlotsFull))
	wantConf := []DateSlot{{d(1), calendar.PM}, {d(2), calendar.AM}, {d(2), calendar.PM}}
	if !reflect.DeepEqual(conf, wantConf) {
		t.Fatalf("conflicts = %v", conf)
	}
	if c.FreeFor(a, iv(2, 1, calendar.SlotsFull)) {
		t.Fatal("empty interval is never free")
	}
	var nilSet *FreeSpots
	if nilSet.IsFree(a, d(1), calendar.AM) || nilSet.Count(d(1), calendar.AM) != 0 || nilSet.Clone() == nil {
		t.Fatal("nil set")
	}
	// Dates parsed from strings and computed with AddDays address the same half-day.
	fs.Add(calendar.MustParse("2027-08-05"), calendar.PM, a)
	if !fs.IsFree(a, d(5), calendar.PM) {
		t.Fatal("date key normalisation")
	}
}

func TestSpotUnavailableError_Scenarios(t *testing.T) {
	var err error = &SpotUnavailableError{SpotID: id(7), Conflicts: []DateSlot{{d(1), calendar.AM}, {d(2), calendar.PM}}}
	if !errors.Is(err, ErrSpotUnavailable) || errors.Is(err, ErrPoolExhausted) {
		t.Fatal("errors.Is")
	}
	wrapped := errors.Join(errors.New("ctx"), err)
	var sue *SpotUnavailableError
	if !errors.As(wrapped, &sue) || len(sue.Conflicts) != 2 {
		t.Fatal("errors.As")
	}
	if msg := err.Error(); !strings.Contains(msg, "2027-08-01 AM, 2027-08-02 PM") {
		t.Fatalf("message = %q", msg)
	}
	for _, e := range []error{ErrPoolExhausted, ErrHoldExpired} {
		if errors.Is(e, ErrSpotUnavailable) {
			t.Fatal("sentinels must be distinct")
		}
	}
}
