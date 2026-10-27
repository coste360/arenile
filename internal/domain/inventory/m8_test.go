package inventory

import (
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/store"
)

func TestSetupLabel_Compact(t *testing.T) {
	tests := []struct {
		name  string
		items []SetupItem
		want  string
	}{
		{"none", nil, ""},
		{"one", []SetupItem{{MapLabel: "L", Quantity: 2}}, "2L"},
		{"two", []SetupItem{{MapLabel: "L", Quantity: 2}, {MapLabel: "S", Quantity: 1}}, "2L 1S"},
		{"zero quantity and empty label skipped", []SetupItem{{MapLabel: "L", Quantity: 0}, {MapLabel: "", Quantity: 3}, {MapLabel: "Max", Quantity: 1}}, "1Max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetupLabel(tt.items); got != tt.want {
				t.Fatalf("SetupLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpotSegment_SplitStay(t *testing.T) {
	d := calendar.MustParse
	rows := []store.LineSpotDay{
		{BookingLineID: "l1", SpotID: "A5", Date: d("2027-08-10")},
		{BookingLineID: "l1", SpotID: "A5", Date: d("2027-08-11")},
		{BookingLineID: "l1", SpotID: "A7", Date: d("2027-08-12")},
		{BookingLineID: "l1", SpotID: "A5", Date: d("2027-08-13")},
		{BookingLineID: "l2", SpotID: "A5", Date: d("2027-08-12")},
	}
	tests := []struct {
		name, line, spot, date string
		want                   string
	}{
		{"first run", "l1", "A5", "2027-08-11", "2027-08-10..2027-08-11"},
		{"back to the first spot", "l1", "A5", "2027-08-13", "2027-08-13..2027-08-13"},
		{"middle spot", "l1", "A7", "2027-08-12", "2027-08-12..2027-08-12"},
		{"not split", "l3", "A5", "2027-08-12", ""},
		{"no line", "", "A5", "2027-08-12", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, ok := spotSegment(rows, tt.line, tt.spot, d(tt.date))
			got := ""
			if ok {
				got = from.String() + ".." + to.String()
			}
			if got != tt.want {
				t.Fatalf("segment %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFreeDays_PartiallyFree(t *testing.T) {
	d := calendar.MustParse
	open := []availability.DateSlot{
		{Date: d("2027-08-01"), Slot: calendar.AM},
		{Date: d("2027-08-01"), Slot: calendar.PM},
		{Date: d("2027-08-02"), Slot: calendar.AM},
		{Date: d("2027-08-02"), Slot: calendar.PM},
		{Date: d("2027-08-03"), Slot: calendar.AM}, // PM closed
	}
	taken := map[availability.DateSlot]bool{{Date: d("2027-08-02"), Slot: calendar.PM}: true}
	free, total := freeDays(open, func(ds availability.DateSlot) bool { return !taken[ds] })
	if total != 3 || len(free) != 2 || free[0].String() != "2027-08-01" || free[1].String() != "2027-08-03" {
		t.Fatalf("free %v of %d", free, total)
	}
	if free, total := freeDays(nil, nil); total != 0 || free == nil || len(free) != 0 {
		t.Fatalf("empty range: %v %d", free, total)
	}
}
