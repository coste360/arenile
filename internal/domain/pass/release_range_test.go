package pass

import (
	"fmt"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
)

func TestExpandRange_Scenarios(t *testing.T) {
	pass := &store.Booking{StartsOn: d("2027-06-01"), EndsOn: d("2027-09-15"), Slots: calendar.SlotsFull}
	tests := []struct {
		name     string
		from, to string
		weekdays []int
		today    string
		want     string // first..last (count), "" = empty
		errField string
	}{
		{"plain range", "2027-07-10", "2027-07-20", nil, "2027-07-01", "2027-07-10..2027-07-20 (11)", ""},
		{"every monday of july", "2027-07-01", "2027-07-31", []int{1}, "2027-06-01", "2027-07-05..2027-07-26 (4)", ""},
		{"weekend days", "2027-07-01", "2027-07-11", []int{6, 7}, "2027-06-01", "2027-07-03..2027-07-11 (4)", ""},
		{"clamped to the pass", "2027-05-01", "2027-10-31", []int{3}, "2027-06-01", "2027-06-02..2027-09-15 (16)", ""},
		{"past days skipped", "2027-07-01", "2027-07-31", []int{1}, "2027-07-13", "2027-07-19..2027-07-26 (2)", ""},
		{"wholly past", "2027-06-01", "2027-06-30", nil, "2027-07-13", "", ""},
		{"no matching weekday", "2027-07-06", "2027-07-08", []int{1}, "2027-06-01", "", ""},
		{"inverted", "2027-07-20", "2027-07-10", nil, "2027-06-01", "", "to"},
		{"bad weekday", "2027-07-01", "2027-07-31", []int{0, 8}, "2027-06-01", "", "weekdays[0]"},
		{"too long", "2027-01-01", "2028-06-01", nil, "2027-06-01", "", "to"},
		{"missing bound", "", "2027-07-31", nil, "2027-06-01", "", "from"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ReleaseRange{Weekdays: tt.weekdays}
			if tt.from != "" {
				r.From = d(tt.from)
			}
			r.To = d(tt.to)
			got, err := expandRange(pass, r, d(tt.today))
			if tt.errField != "" {
				assertField(t, err, tt.errField)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s := summarize(got); s != tt.want {
				t.Fatalf("got %s, want %s", s, tt.want)
			}
			for _, x := range got {
				if len(tt.weekdays) > 0 && !slices.Contains(tt.weekdays, x.ISOWeekday()) {
					t.Fatalf("%s is weekday %d", x, x.ISOWeekday())
				}
			}
		})
	}
}

func TestReleaseSlots_AgainstPass(t *testing.T) {
	am := &store.Booking{Slots: calendar.SlotsAM}
	if got, err := releaseSlots(am, ""); err != nil || len(got) != 1 || got[0] != calendar.AM {
		t.Fatalf("default slots %v %v", got, err)
	}
	if _, err := releaseSlots(am, calendar.SlotsPM); err == nil {
		t.Fatal("an AM pass cannot release the afternoon")
	}
	if _, err := releaseSlots(am, "XX"); err == nil {
		t.Fatal("invalid slots accepted")
	}
}

func summarize(ds []calendar.Date) string {
	if len(ds) == 0 {
		return ""
	}
	return fmt.Sprintf("%s..%s (%d)", ds[0], ds[len(ds)-1], len(ds))
}
