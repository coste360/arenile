package policy

import (
	"errors"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/coste360/arenile/internal/calendar"
)

func rome(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

// at returns the instant of local time hh:mm on date d in loc.
func at(d string, hh, mm int, loc *time.Location) time.Time {
	dd := calendar.MustParse(d)
	return time.Date(dd.Year(), dd.Month(), dd.Day(), hh, mm, 0, 0, loc)
}

func codes(v Validation) []Code {
	out := make([]Code, len(v.Violations))
	for i, x := range v.Violations {
		out[i] = x.Code
	}
	return out
}

func sameCodes(a, b []Code) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func req(start, end string, slots calendar.Slots, f Formula, spots int) Request {
	return Request{
		StartsOn: calendar.MustParse(start), EndsOn: calendar.MustParse(end),
		Slots: slots, Formula: f, SpotCount: spots,
	}
}

func TestValidate_Table(t *testing.T) {
	loc := rome(t)
	def := DefaultRules()
	// Today is Saturday 2027-07-10 in Rome.
	const today = "2027-07-10"
	full, am, pm := calendar.SlotsFull, calendar.SlotsAM, calendar.SlotsPM

	weekOnly := Merge(def, Rules{Duration: &Duration{AllowedFormulas: []Formula{FormulaWeek}, FixedStartWeekday: Ptr(6)}})
	minLead2 := Merge(def, Rules{MinLeadTime: &MinLeadTime{Days: Ptr(2), CutoffTime: Ptr("18:00")}})
	noSameDay := Merge(def, Rules{SameDayBooking: &SameDayBooking{Enabled: Ptr(false)}})
	sameDayPMOnly := Merge(def, Rules{SameDayBooking: &SameDayBooking{OnlySlot: Ptr(OnlySlotPM), Until: Ptr("16:00")}})
	sameDayAMOnly := Merge(def, Rules{SameDayBooking: &SameDayBooking{OnlySlot: Ptr(OnlySlotAM), Until: Ptr("11:00")}})
	sameDayNoOnly := Merge(def, Rules{SameDayBooking: &SameDayBooking{OnlySlot: Ptr("")}})
	approval := Merge(def, Rules{ApprovalRequired: Ptr(true)})
	anyWeekday := Merge(weekOnly, Rules{Duration: &Duration{FixedStartWeekday: Ptr(0)}})
	noCutoff := Rules{MinLeadTime: &MinLeadTime{Days: Ptr(0)}}
	badCutoff := Rules{MinLeadTime: &MinLeadTime{Days: Ptr(0), CutoffTime: Ptr("25:99")}}
	minMax := Rules{Duration: &Duration{MinDays: Ptr(3), MaxDays: Ptr(5)}}

	tests := []struct {
		name  string
		req   Request
		rules Rules
		now   time.Time
		want  []Code
		appr  bool
	}{
		{"empty rules accept anything", req("2027-07-11", "2027-07-11", full, FormulaDay, 99), Rules{}, at(today, 10, 0, loc), nil, false},
		{"empty rules still reject past start", req("2027-07-09", "2027-07-09", full, FormulaDay, 1), Rules{}, at(today, 10, 0, loc), []Code{CodeLeadTimeTooShort}, false},
		{"tomorrow before cutoff ok", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), def, at(today, 19, 59, loc), nil, false},
		{"tomorrow at cutoff", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), def, at(today, 20, 0, loc), []Code{CodeCutoffPassed}, false},
		{"tomorrow after cutoff", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), def, at(today, 23, 30, loc), []Code{CodeCutoffPassed}, false},
		{"day after tomorrow late evening ok", req("2027-07-12", "2027-07-12", full, FormulaDay, 1), def, at(today, 23, 30, loc), nil, false},
		{"same day morning full ok", req(today, today, full, FormulaDay, 1), def, at(today, 9, 0, loc), nil, false},
		{"same day 12:59 AM ok", req(today, today, am, FormulaHalfDay, 1), def, at(today, 12, 59, loc), nil, false},
		{"same day after until full closed", req(today, today, full, FormulaDay, 1), def, at(today, 13, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day after until AM closed", req(today, today, am, FormulaHalfDay, 1), def, at(today, 14, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day after until PM ok with PM_after", req(today, today, pm, FormulaHalfDay, 1), def, at(today, 15, 0, loc), nil, false},
		{"same day disabled", req(today, today, pm, FormulaHalfDay, 1), noSameDay, at(today, 8, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day PM only rejects full", req(today, today, full, FormulaDay, 1), sameDayPMOnly, at(today, 8, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day PM only accepts PM", req(today, today, pm, FormulaHalfDay, 1), sameDayPMOnly, at(today, 8, 0, loc), nil, false},
		{"same day PM only after until", req(today, today, pm, FormulaHalfDay, 1), sameDayPMOnly, at(today, 16, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day AM only accepts AM", req(today, today, am, FormulaHalfDay, 1), sameDayAMOnly, at(today, 7, 0, loc), nil, false},
		{"same day AM only rejects PM", req(today, today, pm, FormulaHalfDay, 1), sameDayAMOnly, at(today, 7, 0, loc), []Code{CodeSameDayClosed}, false},
		{"same day no only_slot after until", req(today, today, pm, FormulaHalfDay, 1), sameDayNoOnly, at(today, 13, 30, loc), []Code{CodeSameDayClosed}, false},
		{"same day no same_day config", req(today, today, full, FormulaDay, 1), noCutoff, at(today, 22, 0, loc), nil, false},
		{"yesterday", req("2027-07-09", "2027-07-12", full, FormulaMultiDay, 1), def, at(today, 8, 0, loc), []Code{CodeLeadTimeTooShort}, false},
		{"min lead 2 tomorrow", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), minLead2, at(today, 8, 0, loc), []Code{CodeLeadTimeTooShort}, false},
		{"min lead 2 same day", req(today, today, full, FormulaDay, 1), minLead2, at(today, 8, 0, loc), []Code{CodeLeadTimeTooShort}, false},
		{"min lead 2 in two days before cutoff", req("2027-07-12", "2027-07-12", full, FormulaDay, 1), minLead2, at(today, 17, 59, loc), nil, false},
		{"min lead 2 in two days after cutoff", req("2027-07-12", "2027-07-12", full, FormulaDay, 1), minLead2, at(today, 18, 0, loc), []Code{CodeCutoffPassed}, false},
		{"min lead 2 in three days late", req("2027-07-13", "2027-07-13", full, FormulaDay, 1), minLead2, at(today, 23, 0, loc), nil, false},
		{"no cutoff tomorrow late", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), noCutoff, at(today, 23, 59, loc), nil, false},
		{"malformed cutoff ignored", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), badCutoff, at(today, 23, 59, loc), nil, false},
		{"max lead exactly 120", req("2027-11-07", "2027-11-07", full, FormulaDay, 1), def, at(today, 8, 0, loc), nil, false},
		{"max lead 121", req("2027-11-08", "2027-11-08", full, FormulaDay, 1), def, at(today, 8, 0, loc), []Code{CodeLeadTimeTooLong}, false},
		{"formula not allowed", req("2027-07-17", "2027-07-23", full, FormulaDay, 1), weekOnly, at(today, 8, 0, loc), []Code{CodeFormulaNotAllowed}, false},
		{"week from saturday ok", req("2027-07-17", "2027-07-23", full, FormulaWeek, 1), weekOnly, at(today, 8, 0, loc), nil, false},
		{"week from sunday wrong weekday", req("2027-07-18", "2027-07-24", full, FormulaWeek, 1), weekOnly, at(today, 8, 0, loc), []Code{CodeStartWeekdayNotAllowed}, false},
		{"weekday override 0 means any", req("2027-07-18", "2027-07-24", full, FormulaWeek, 1), anyWeekday, at(today, 8, 0, loc), nil, false},
		{"multi day 30 ok", req("2027-07-12", "2027-08-10", full, FormulaMultiDay, 1), def, at(today, 8, 0, loc), nil, false},
		{"multi day 31 too long", req("2027-07-12", "2027-08-11", full, FormulaMultiDay, 1), def, at(today, 8, 0, loc), []Code{CodeDurationTooLong}, false},
		{"month formula exempt from max days", req("2027-07-12", "2027-08-11", full, FormulaMonth, 1), def, at(today, 8, 0, loc), nil, false},
		{"season formula exempt and lead ok", req("2027-07-12", "2027-09-30", full, FormulaSeason, 1), def, at(today, 8, 0, loc), nil, false},
		{"min 3 days with 2", req("2027-07-12", "2027-07-13", full, FormulaMultiDay, 1), minMax, at(today, 8, 0, loc), []Code{CodeDurationTooShort}, false},
		{"min 3 days with 3", req("2027-07-12", "2027-07-14", full, FormulaMultiDay, 1), minMax, at(today, 8, 0, loc), nil, false},
		{"max 5 days with 5", req("2027-07-12", "2027-07-16", full, FormulaMultiDay, 1), minMax, at(today, 8, 0, loc), nil, false},
		{"max 5 days with 6", req("2027-07-12", "2027-07-17", full, FormulaMultiDay, 1), minMax, at(today, 8, 0, loc), []Code{CodeDurationTooLong}, false},
		{"end before start", req("2027-07-12", "2027-07-11", full, FormulaDay, 1), def, at(today, 8, 0, loc), []Code{CodeDurationTooShort}, false},
		{"four spots ok", req("2027-07-12", "2027-07-12", full, FormulaDay, 4), def, at(today, 8, 0, loc), nil, false},
		{"five spots too many", req("2027-07-12", "2027-07-12", full, FormulaDay, 5), def, at(today, 8, 0, loc), []Code{CodeTooManySpots}, false},
		{"approval flag not violation", req("2027-07-12", "2027-07-12", full, FormulaDay, 1), approval, at(today, 8, 0, loc), nil, true},
		{"approval flag with violations", req("2027-07-12", "2027-07-12", full, FormulaDay, 9), approval, at(today, 8, 0, loc), []Code{CodeTooManySpots}, true},
		{
			"multiple violations in stable order",
			req("2027-07-11", "2027-07-11", full, FormulaDay, 9), weekOnly, at(today, 21, 0, loc),
			[]Code{CodeCutoffPassed, CodeFormulaNotAllowed, CodeStartWeekdayNotAllowed, CodeTooManySpots},
			false,
		},
		// "now" is 2027-07-10 23:30 UTC = 2027-07-11 01:30 in Rome: local today is the 11th.
		{"timezone: UTC instant is next local day", req("2027-07-11", "2027-07-11", full, FormulaDay, 1), def, time.Date(2027, 7, 10, 23, 30, 0, 0, time.UTC), nil, false},
		{"timezone: local yesterday", req("2027-07-10", "2027-07-10", full, FormulaDay, 1), def, time.Date(2027, 7, 10, 23, 30, 0, 0, time.UTC), []Code{CodeLeadTimeTooShort}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(tc.req, tc.rules, tc.now, loc)
			if !sameCodes(codes(got), tc.want) {
				t.Fatalf("codes = %v, want %v", codes(got), tc.want)
			}
			if got.ApprovalRequired != tc.appr {
				t.Fatalf("approval = %v, want %v", got.ApprovalRequired, tc.appr)
			}
			err := got.Err()
			if (len(tc.want) == 0) != (err == nil) {
				t.Fatalf("err = %v, want violations %v", err, tc.want)
			}
			if err != nil {
				if !errors.Is(err, ErrPolicyViolation) {
					t.Fatalf("errors.Is(ErrPolicyViolation) false")
				}
				var ve *ViolationError
				if !errors.As(err, &ve) || !ve.Has(tc.want[0]) {
					t.Fatalf("ViolationError missing %s", tc.want[0])
				}
			}
		})
	}
}

func TestPolicy_NilLocationPanics(t *testing.T) {
	now := time.Date(2027, 7, 10, 20, 30, 0, 0, time.UTC)
	start := calendar.MustParse("2027-07-11")
	calls := map[string]func(){
		"Validate": func() {
			Validate(req("2027-07-11", "2027-07-11", calendar.SlotsFull, FormulaDay, 1), DefaultRules(), now, nil)
		},
		"CancellationOutcome": func() { CancellationOutcome(start, 1000, now, nil, DefaultRules()) },
		"ModificationAllowed": func() { ModificationAllowed(start, now, nil, DefaultRules()) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("nil location must panic, not fall back to UTC")
				}
			}()
			call()
		})
	}
}

// TestValidate_CutoffAt2330Rome: at 23:30 in Rome (21:30 UTC) the 23:00
// cut-off for tomorrow has passed; evaluating in UTC would wrongly accept it.
func TestValidate_CutoffAt2330Rome(t *testing.T) {
	loc := rome(t)
	r := Rules{MinLeadTime: &MinLeadTime{Days: Ptr(1), CutoffTime: Ptr("23:00")}}
	now := at("2027-07-10", 23, 30, loc)
	got := Validate(req("2027-07-11", "2027-07-11", calendar.SlotsFull, FormulaDay, 1), r, now, loc)
	if !sameCodes(codes(got), []Code{CodeCutoffPassed}) {
		t.Fatalf("Rome: codes = %v, want cutoff_passed", codes(got))
	}
	if utc := Validate(req("2027-07-11", "2027-07-11", calendar.SlotsFull, FormulaDay, 1), r, now, time.UTC); utc.OK() == got.OK() {
		t.Fatal("test must distinguish the establishment timezone from UTC")
	}
}

// TestCancellationOutcome_TierAtMidnightRome: at 00:30 in Rome on the start
// day (22:30 UTC the day before) the same-day tier applies.
func TestCancellationOutcome_TierAtMidnightRome(t *testing.T) {
	loc := rome(t)
	r := Rules{Cancellation: []CancellationTier{{UntilDaysBefore: 1, RefundPercent: 100}, {UntilDaysBefore: 0, RefundPercent: 0}}}
	got := CancellationOutcome(calendar.MustParse("2027-07-11"), 10000, at("2027-07-11", 0, 30, loc), loc, r)
	if got.DaysBefore != 0 || got.RefundAmount != 0 || got.TierApplied != 1 {
		t.Fatalf("got %+v, want same-day tier with no refund", got)
	}
}

func TestValidate_ViolationParams(t *testing.T) {
	loc := rome(t)
	got := Validate(req("2027-07-12", "2027-07-12", calendar.SlotsFull, FormulaDay, 6), DefaultRules(), at("2027-07-10", 8, 0, loc), loc)
	if len(got.Violations) != 1 {
		t.Fatalf("violations = %v", got.Violations)
	}
	p := got.Violations[0].Params
	if p["spots"] != "6" || p["max_spots"] != "4" {
		t.Fatalf("params = %v", p)
	}
	if msg := got.Err().Error(); msg != "policy: violation: too_many_spots" {
		t.Fatalf("message = %q", msg)
	}
}
