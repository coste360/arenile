package pass

import (
	"errors"
	"fmt"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var d = calendar.MustParse

func dp(s string) *calendar.Date { x := d(s); return &x }

func TestPassInterval_Defaults(t *testing.T) {
	season := &store.Season{OpensOn: d("2027-05-15"), ClosesOn: d("2027-09-15")}
	tests := []struct {
		name     string
		from, to *calendar.Date
		today    string
		want     string
		errField string
	}{
		{"whole season before opening", nil, nil, "2027-03-01", "2027-05-15..2027-09-15", ""},
		{"season started: from today", nil, nil, "2027-07-01", "2027-07-01..2027-09-15", ""},
		{"explicit range", dp("2027-06-01"), dp("2027-06-30"), "2027-03-01", "2027-06-01..2027-06-30", ""},
		{"start before season", dp("2027-05-01"), nil, "2027-03-01", "", "starts_on"},
		{"end after season", nil, dp("2027-10-01"), "2027-03-01", "", "ends_on"},
		{"inverted", dp("2027-07-01"), dp("2027-06-01"), "2027-03-01", "", "ends_on"},
		{"season over", nil, nil, "2027-10-01", "", "ends_on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv, err := passInterval(season, tt.from, tt.to, d(tt.today))
			if tt.errField != "" {
				assertField(t, err, tt.errField)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := iv.from.String() + ".." + iv.to.String(); got != tt.want {
				t.Fatalf("interval %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPlanRelease_Validation(t *testing.T) {
	full := &store.Booking{StartsOn: d("2027-06-01"), EndsOn: d("2027-09-15"), Slots: calendar.SlotsFull}
	am := &store.Booking{StartsOn: d("2027-06-01"), EndsOn: d("2027-09-15"), Slots: calendar.SlotsAM}
	today := d("2027-07-01")
	tests := []struct {
		name     string
		b        *store.Booking
		dates    []string
		slots    calendar.Slots
		want     string
		errField string
	}{
		{"full day default", full, []string{"2027-07-10"}, "", "2027-07-10/AM 2027-07-10/PM", ""},
		{"only afternoon", full, []string{"2027-07-10"}, calendar.SlotsPM, "2027-07-10/PM", ""},
		{"sorted and deduplicated", full, []string{"2027-07-12", "2027-07-10", "2027-07-12"}, calendar.SlotsAM, "2027-07-10/AM 2027-07-12/AM", ""},
		{"today allowed", am, []string{"2027-07-01"}, "", "2027-07-01/AM", ""},
		{"am pass cannot release pm", am, []string{"2027-07-10"}, calendar.SlotsPM, "", "slots"},
		{"am pass cannot release full", am, []string{"2027-07-10"}, calendar.SlotsFull, "", "slots"},
		{"past day", full, []string{"2027-06-30"}, "", "", "dates[0]"},
		{"outside the pass", full, []string{"2027-09-16"}, "", "", "dates[0]"},
		{"no dates", full, nil, "", "", "dates"},
		{"bad slots", full, []string{"2027-07-10"}, "X", "", "slots"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dates []calendar.Date
			for _, s := range tt.dates {
				dates = append(dates, d(s))
			}
			days, err := planRelease(tt.b, dates, tt.slots, today)
			if tt.errField != "" {
				assertField(t, err, tt.errField)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := fmtDays(days); got != tt.want {
				t.Fatalf("days %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOverlapping_ActiveReleases(t *testing.T) {
	days := []availability.DateSlot{{Date: d("2027-07-10"), Slot: calendar.AM}, {Date: d("2027-07-10"), Slot: calendar.PM}}
	active := []store.SeasonRelease{{Date: d("2027-07-10"), Slot: calendar.PM}}
	if got := fmtDays(overlapping(days, active)); got != "2027-07-10/PM" {
		t.Fatalf("overlap %q", got)
	}
}

func TestReleaseCredit_PolicySnapshot(t *testing.T) {
	line := &store.BookingLine{LineTotal: 6000}
	week := &store.Booking{StartsOn: d("2027-07-10"), EndsOn: d("2027-07-12"), Slots: calendar.SlotsFull} // 6 half-days
	per := resaleAmountPerHalfDay(line, week)
	if per != 1000 {
		t.Fatalf("per half-day %v", per)
	}
	tests := []struct {
		name     string
		snapshot string
		want     money.Cents
	}{
		{"enabled 50%", `{"policy_id":"p","rules":{"season_release_credit":{"enabled":true,"percent":50}}}`, 500},
		{"disabled", `{"rules":{"season_release_credit":{"enabled":false,"percent":50}}}`, 0},
		{"absent", `{"rules":{}}`, 0},
		{"malformed", `not json`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := releaseCredit(per, store.JSON(tt.snapshot)); got != tt.want {
				t.Fatalf("credit %v, want %v", got, tt.want)
			}
		})
	}
	if got := resaleAmountPerHalfDay(&store.BookingLine{LineTotal: 1000}, &store.Booking{StartsOn: d("2027-07-10"), EndsOn: d("2027-07-10"), Slots: calendar.SlotsFull}); got != 500 {
		t.Fatalf("one day full %v", got)
	}
}

func assertField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *configerr.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error on %q, got %v", field, err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("no error on %q: %+v", field, ve.Fields)
}

func fmtDays(days []availability.DateSlot) string {
	s := ""
	for i, x := range days {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%s/%s", x.Date, x.Slot)
	}
	return s
}
