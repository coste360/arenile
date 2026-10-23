package rollover

import (
	"errors"
	"slices"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

var d = calendar.MustParse

func season(year int, opens, closes string) store.Season {
	return store.Season{Year: year, OpensOn: d(opens), ClosesOn: d(closes)}
}

func TestTargetDates_Scenarios(t *testing.T) {
	src := season(2026, "2026-05-16", "2026-09-20") // Saturday .. Sunday
	override := d("2027-06-01")
	badYear := d("2028-05-15")
	for _, tc := range []struct {
		name         string
		year         int
		opens        *calendar.Date
		closes       *calendar.Date
		want         [2]string
		wantInvalid  bool
		invalidField string
	}{
		{name: "next year keeps the weekday", year: 2027, want: [2]string{"2027-05-15", "2027-09-19"}},
		{name: "two years ahead", year: 2028, want: [2]string{"2028-05-13", "2028-09-17"}},
		{name: "override opening", year: 2027, opens: &override, want: [2]string{"2027-06-01", "2027-09-19"}},
		{name: "same year", year: 2026, wantInvalid: true, invalidField: "target_year"},
		{name: "opening outside the year", year: 2027, opens: &badYear, wantInvalid: true, invalidField: "opens_on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opens, closes, err := TargetDates(src, tc.year, tc.opens, tc.closes)
			if tc.wantInvalid {
				var ve *configerr.ValidationError
				if !errors.As(err, &ve) || ve.Fields[0].Field != tc.invalidField {
					t.Fatalf("want validation error on %s, got %v", tc.invalidField, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := [2]string{opens.String(), closes.String()}; got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanClosureDays_Scenarios(t *testing.T) {
	cd := func(date, reason string, slots ...calendar.Slot) store.ClosureDay {
		return store.ClosureDay{Date: d(date), Reason: reason, Slots: slots}
	}
	for _, tc := range []struct {
		name     string
		src      []store.ClosureDay
		existing map[string]bool
		want     []string // shifted dates
		warnings int
	}{
		{
			name: "weekday kept and order by date",
			src:  []store.ClosureDay{cd("2026-07-18", "festa", calendar.AM), cd("2026-06-06", "regata")},
			want: []string{"2027-06-05", "2027-07-17"},
		},
		{
			name: "fixed holiday keeps its date",
			src:  []store.ClosureDay{cd("2026-08-15", "ferragosto")},
			want: []string{"2027-08-15"},
		},
		{
			name:     "outside the target season is dropped",
			src:      []store.ClosureDay{cd("2026-09-20", "fine")},
			want:     []string{},
			warnings: 1,
		},
		{
			name:     "already closed in the target",
			src:      []store.ClosureDay{cd("2026-07-18", "festa")},
			existing: map[string]bool{"2027-07-17": true},
			want:     []string{},
			warnings: 1,
		},
		{
			name:     "two closures on the same shifted day",
			src:      []store.ClosureDay{cd("2026-07-18", "a"), cd("2026-07-18", "b")},
			want:     []string{"2027-07-17"},
			warnings: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings := PlanClosureDays(tc.src, 1, d("2027-05-15"), d("2027-09-12"), tc.existing)
			dates := []string{}
			for _, c := range got {
				dates = append(dates, c.Date.String())
			}
			if !slices.Equal(dates, tc.want) || len(warnings) != tc.warnings {
				t.Fatalf("got %v (%d warnings: %v), want %v (%d)", dates, len(warnings), warnings, tc.want, tc.warnings)
			}
		})
	}
	// Slots and reason are carried over.
	got, _ := PlanClosureDays([]store.ClosureDay{cd("2026-07-18", "festa", calendar.PM)}, 1, d("2027-05-15"), d("2027-09-12"), nil)
	if len(got) != 1 || got[0].Reason != "festa" || !slices.Equal(got[0].Slots, []calendar.Slot{calendar.PM}) || got[0].From.String() != "2026-07-18" {
		t.Fatalf("copy: %+v", got)
	}
}
