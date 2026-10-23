package report

import (
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
)

func d(s string) calendar.Date { return calendar.MustParse(s) }

func TestNewPeriod_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		from, to calendar.Date
		ok       bool
		days     int
	}{
		{"single day", d("2027-07-01"), d("2027-07-01"), true, 1},
		{"season", d("2027-05-01"), d("2027-09-30"), true, 153},
		{"max span", d("2027-01-01"), d("2027-01-01").AddDays(MaxPeriodDays - 1), true, MaxPeriodDays},
		{"too long", d("2027-01-01"), d("2027-01-01").AddDays(MaxPeriodDays), false, 0},
		{"reversed", d("2027-07-02"), d("2027-07-01"), false, 0},
		{"missing from", calendar.Date{}, d("2027-07-01"), false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPeriod(tt.from, tt.to)
			if !tt.ok {
				if !errors.Is(err, configerr.ErrValidation) {
					t.Fatalf("want validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Days() != tt.days {
				t.Fatalf("days = %d, want %d", p.Days(), tt.days)
			}
		})
	}
}

func TestPreviousSpan_Scenarios(t *testing.T) {
	tests := []struct {
		name         string
		from, to     string
		year         int
		wantF, wantT string
	}{
		{"default previous year", "2027-06-01", "2027-08-31", 0, "2026-06-01", "2026-08-31"},
		{"explicit year", "2027-06-01", "2027-06-30", 2024, "2024-06-01", "2024-06-30"},
		{"leap day to non leap", "2028-02-29", "2028-03-01", 0, "2027-02-28", "2027-03-01"},
		{"across new year", "2027-12-20", "2028-01-10", 0, "2026-12-20", "2027-01-10"},
		{"comparison year has the leap day in the middle", "2027-02-28", "2027-03-01", 2024, "2024-02-28", "2024-03-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PreviousSpan(Period{From: d(tt.from), To: d(tt.to)}, tt.year)
			if got.From.String() != tt.wantF || got.To.String() != tt.wantT {
				t.Fatalf("got %s..%s, want %s..%s", got.From, got.To, tt.wantF, tt.wantT)
			}
		})
	}
}

func TestRatioAndDelta_Scenarios(t *testing.T) {
	if got := Ratio(1, 3); got != 3333 {
		t.Fatalf("Ratio(1,3) = %d", got)
	}
	if got := Ratio(5, 0); got != 0 {
		t.Fatalf("Ratio(5,0) = %d", got)
	}
	if DeltaBP(10, 0) != nil {
		t.Fatal("DeltaBP with prev 0 must be nil")
	}
	if got := DeltaBP(150, 100); got == nil || *got != 5000 {
		t.Fatalf("DeltaBP(150,100) = %v", got)
	}
	if got := DeltaBP(50, 100); got == nil || *got != -5000 {
		t.Fatalf("DeltaBP(50,100) = %v", got)
	}
}

func TestKeysAndLeadTime_Scenarios(t *testing.T) {
	if got := WeekKey(d("2027-07-01")); got != "2027-W26" {
		t.Fatalf("WeekKey = %s", got)
	}
	if got := MonthKey(d("2027-07-01")); got != "2027-07" {
		t.Fatalf("MonthKey = %s", got)
	}
	cases := map[int]LeadTimeBucket{-2: LeadSameDay, 0: LeadSameDay, 1: Lead1to3, 7: Lead4to7, 14: Lead8to14, 30: Lead15to30, 60: Lead31to60, 61: Lead61Plus}
	for days, want := range cases {
		if got := LeadTimeBucketOf(days); got != want {
			t.Fatalf("LeadTimeBucketOf(%d) = %s, want %s", days, got, want)
		}
	}
}
