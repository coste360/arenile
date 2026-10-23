package report

import (
	"fmt"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// MaxPeriodDays bounds the span of a report (inclusive days).
const MaxPeriodDays = 400

// Period is an inclusive span of calendar dates in the establishment
// timezone.
type Period struct {
	From calendar.Date
	To   calendar.Date
}

// NewPeriod validates from..to: both set, to >= from, at most MaxPeriodDays
// days. Errors are configerr validation errors on the query parameters.
func NewPeriod(from, to calendar.Date) (Period, error) {
	ve := &configerr.ValidationError{}
	switch {
	case from.IsZero():
		ve.Add("from", "Obbligatorio.")
	case to.IsZero():
		ve.Add("to", "Obbligatorio.")
	case to.Before(from):
		ve.Add("to", "Non può precedere from.")
	case from.DaysUntil(to)+1 > MaxPeriodDays:
		ve.Add("to", fmt.Sprintf("Il periodo può coprire al massimo %d giorni.", MaxPeriodDays))
	}
	if err := ve.OrNil(); err != nil {
		return Period{}, err
	}
	return Period{From: from, To: to}, nil
}

// Days returns the number of days of the period, both ends included.
func (p Period) Days() int { return p.From.DaysUntil(p.To) + 1 }

// Contains reports whether d is inside the period.
func (p Period) Contains(d calendar.Date) bool { return !d.Before(p.From) && !d.After(p.To) }

// ShiftToYear returns the same day and month in year; 29 February becomes 28
// February when year is not a leap year.
func ShiftToYear(d calendar.Date, year int) calendar.Date {
	day := d.Day()
	if d.Month() == time.February && day == 29 && !isLeap(year) {
		day = 28
	}
	return calendar.New(year, d.Month(), day)
}

// PreviousSpan returns the comparison span of p: the same calendar span
// moved to compareYear (0 = the year of p.From minus one). The year offset
// is applied to both ends, so a span crossing New Year keeps its shape.
func PreviousSpan(p Period, compareYear int) Period {
	if compareYear == 0 {
		compareYear = p.From.Year() - 1
	}
	offset := compareYear - p.From.Year()
	return Period{From: ShiftToYear(p.From, p.From.Year()+offset), To: ShiftToYear(p.To, p.To.Year()+offset)}
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// WeekKey returns the ISO week key of d ("2027-W27").
func WeekKey(d calendar.Date) string {
	y, w := d.Time().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// MonthKey returns the month key of d ("2027-07").
func MonthKey(d calendar.Date) string { return fmt.Sprintf("%04d-%02d", d.Year(), int(d.Month())) }

// Ratio returns num/den in basis points (10000 = 100%), truncated toward
// zero; 0 when den is 0.
func Ratio(num, den int64) int64 {
	if den == 0 {
		return 0
	}
	return num * 10000 / den
}

// DeltaBP returns the relative change (cur-prev)/prev in basis points,
// truncated toward zero, or nil when prev is 0 (undefined).
func DeltaBP(cur, prev int64) *int64 {
	if prev == 0 {
		return nil
	}
	d := (cur - prev) * 10000 / prev
	return &d
}

// LeadTimeBucket is a lead-time class of the bookings report.
type LeadTimeBucket string

// Lead-time buckets (API enum LeadTimeBucket).
const (
	LeadSameDay LeadTimeBucket = "same_day"
	Lead1to3    LeadTimeBucket = "d1_3"
	Lead4to7    LeadTimeBucket = "d4_7"
	Lead8to14   LeadTimeBucket = "d8_14"
	Lead15to30  LeadTimeBucket = "d15_30"
	Lead31to60  LeadTimeBucket = "d31_60"
	Lead61Plus  LeadTimeBucket = "d61_plus"
)

// LeadTimeBuckets lists the buckets in display order.
var LeadTimeBuckets = []LeadTimeBucket{LeadSameDay, Lead1to3, Lead4to7, Lead8to14, Lead15to30, Lead31to60, Lead61Plus}

// LeadTimeBucketOf classifies a lead time in days (negative values, e.g.
// bookings created after their first day, count as same day).
func LeadTimeBucketOf(days int) LeadTimeBucket {
	switch {
	case days <= 0:
		return LeadSameDay
	case days <= 3:
		return Lead1to3
	case days <= 7:
		return Lead4to7
	case days <= 14:
		return Lead8to14
	case days <= 30:
		return Lead15to30
	case days <= 60:
		return Lead31to60
	default:
		return Lead61Plus
	}
}

// WeekdaySpan returns the weekday-aligned comparison span of p: the same
// number of days, starting on the day with the weekday of p.From nearest to
// the calendar-aligned start (PreviousSpan(p, compareYear).From), at most 3
// days away. For the previous year it is p moved back by 364 days (52
// weeks), so Saturdays compare with Saturdays.
func WeekdaySpan(p Period, compareYear int) Period {
	anchor := PreviousSpan(p, compareYear).From
	shift := (p.From.ISOWeekday() - anchor.ISOWeekday() + 7) % 7 // 0..6 forward
	if shift > 3 {
		shift -= 7 // nearer going back
	}
	from := anchor.AddDays(shift)
	return Period{From: from, To: from.AddDays(p.Days() - 1)}
}

// ComparisonSpan returns the comparison span of p for compareYear (0 = the
// previous year) and align (empty = AlignCalendar).
func ComparisonSpan(p Period, compareYear int, align Align) Period {
	if align == AlignWeekday {
		return WeekdaySpan(p, compareYear)
	}
	return PreviousSpan(p, compareYear)
}

// dayWindow returns the UTC half-open window [start of p.From, start of the
// day after p.To) in loc: the instants whose calendar date in loc is in p
// (DST transitions included, since each bound is resolved separately).
func dayWindow(p Period, loc *time.Location) (from, to time.Time) {
	start := time.Date(p.From.Year(), p.From.Month(), p.From.Day(), 0, 0, 0, 0, loc)
	end := p.To.AddDays(1)
	return start.UTC(), time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc).UTC()
}
