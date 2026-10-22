package calendar

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEaster_KnownYears(t *testing.T) {
	cases := []struct {
		year int
		want string
	}{
		{2019, "2019-04-21"},
		{2024, "2024-03-31"},
		{2025, "2025-04-20"},
		{2026, "2026-04-05"},
		{2027, "2027-03-28"},
		{2028, "2028-04-16"},
		{2029, "2029-04-01"},
		{2030, "2030-04-21"},
		{2038, "2038-04-25"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := Easter(tc.year).String(); got != tc.want {
				t.Fatalf("Easter(%d) = %s, want %s", tc.year, got, tc.want)
			}
		})
	}
}

func TestIsItalianHoliday_Dates(t *testing.T) {
	cases := []struct {
		date string
		want bool
	}{
		{"2027-01-01", true},
		{"2027-01-06", true},
		{"2027-03-28", true}, // Easter 2027
		{"2027-03-29", true}, // Easter Monday 2027
		{"2027-03-30", false},
		{"2025-04-20", true},
		{"2025-04-21", true},
		{"2026-04-05", true},
		{"2026-04-06", true},
		{"2028-04-16", true},
		{"2028-04-17", true},
		{"2028-04-18", false},
		{"2027-04-25", true},
		{"2027-05-01", true},
		{"2027-06-02", true},
		{"2027-08-15", true},
		{"2027-08-14", false},
		{"2027-08-16", false},
		{"2027-11-01", true},
		{"2027-12-08", true},
		{"2027-12-25", true},
		{"2027-12-26", true},
		{"2027-07-05", false},
		{"2027-06-29", false},
	}
	for _, tc := range cases {
		t.Run(tc.date, func(t *testing.T) {
			if got := IsItalianHoliday(MustParse(tc.date)); got != tc.want {
				t.Fatalf("IsItalianHoliday(%s) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestIsItalianHoliday_ZeroDate(t *testing.T) {
	if IsItalianHoliday(Date{}) {
		t.Fatal("zero date must not be a holiday")
	}
}

func TestItalianHolidays_Year(t *testing.T) {
	got := ItalianHolidays(2027)
	if len(got) != 12 {
		t.Fatalf("got %d holidays, want 12", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i-1].Before(got[i]) {
			t.Fatalf("not sorted at %d: %v", i, got)
		}
	}
	if got[2].String() != "2027-03-28" {
		t.Fatalf("third holiday = %s, want Easter 2027-03-28", got[2])
	}
}

func TestHolidays_Local(t *testing.T) {
	h := NewHolidays(
		[]Date{MustParse("2027-07-16")},
		[]MonthDay{{time.June, 29}},
	)
	cases := []struct {
		name  string
		date  string
		local bool
		any   bool
	}{
		{"specific local date", "2027-07-16", true, true},
		{"specific date other year", "2028-07-16", false, false},
		{"recurring patron 2027", "2027-06-29", true, true},
		{"recurring patron 2031", "2031-06-29", true, true},
		{"national only", "2027-08-15", false, true},
		{"ordinary day", "2027-07-15", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := MustParse(tc.date)
			if got := h.IsLocal(d); got != tc.local {
				t.Fatalf("IsLocal = %v, want %v", got, tc.local)
			}
			if got := h.IsHoliday(d); got != tc.any {
				t.Fatalf("IsHoliday = %v, want %v", got, tc.any)
			}
		})
	}
}

func TestHolidays_ZeroValue(t *testing.T) {
	var h Holidays
	if !h.IsHoliday(MustParse("2027-12-25")) {
		t.Fatal("zero Holidays must include national holidays")
	}
	if h.IsLocal(MustParse("2027-12-25")) {
		t.Fatal("zero Holidays has no local holidays")
	}
}

func TestMonthDay_ParseAndJSON(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"06-29", false},
		{"02-29", false},
		{"13-01", true},
		{"6-29", true},
		{"02-30", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			md, err := ParseMonthDay(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			b, err := json.Marshal(md)
			if err != nil {
				t.Fatal(err)
			}
			var back MonthDay
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			if back != md || md.String() != tc.in {
				t.Fatalf("round trip %q -> %s -> %v", tc.in, b, back)
			}
		})
	}
}

func TestMonthDay_InLeapFallback(t *testing.T) {
	md := MonthDay{time.February, 29}
	if got := md.In(2027).String(); got != "2027-02-28" {
		t.Fatalf("Feb 29 in 2027 = %s", got)
	}
	if got := md.In(2028).String(); got != "2028-02-29" {
		t.Fatalf("Feb 29 in 2028 = %s", got)
	}
}
