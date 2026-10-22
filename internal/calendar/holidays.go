package calendar

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Easter returns Easter Sunday of the given year in the Gregorian calendar,
// computed with the anonymous Gregorian algorithm (Meeus/Jones/Butcher).
func Easter(year int) Date {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return New(year, time.Month(month), day)
}

// EasterMonday returns the Monday after Easter Sunday ("Lunedì dell'Angelo").
func EasterMonday(year int) Date { return Easter(year).AddDays(1) }

// fixedNationalHolidays are the Italian national holidays with a fixed
// month/day (Legge 260/1949 and following amendments).
var fixedNationalHolidays = [...]MonthDay{
	{time.January, 1},   // Capodanno
	{time.January, 6},   // Epifania
	{time.April, 25},    // Festa della Liberazione
	{time.May, 1},       // Festa del Lavoro
	{time.June, 2},      // Festa della Repubblica
	{time.August, 15},   // Ferragosto / Assunzione
	{time.November, 1},  // Ognissanti
	{time.December, 8},  // Immacolata Concezione
	{time.December, 25}, // Natale
	{time.December, 26}, // Santo Stefano
}

// IsItalianFixedHoliday reports whether d is an Italian national holiday with a
// fixed calendar date (all national holidays except Easter Sunday and Monday).
func IsItalianFixedHoliday(d Date) bool {
	md := MonthDayOf(d)
	for _, h := range fixedNationalHolidays {
		if h == md {
			return true
		}
	}
	return false
}

// IsItalianHoliday reports whether d is an Italian national holiday, including
// Easter Sunday and Easter Monday.
func IsItalianHoliday(d Date) bool {
	if d.IsZero() {
		return false
	}
	if IsItalianFixedHoliday(d) {
		return true
	}
	e := Easter(d.Year())
	return d.Equal(e) || d.Equal(e.AddDays(1))
}

// ItalianHolidays returns the Italian national holidays of a year, sorted.
func ItalianHolidays(year int) []Date {
	out := make([]Date, 0, len(fixedNationalHolidays)+2)
	for _, h := range fixedNationalHolidays {
		out = append(out, New(year, h.Month, h.Day))
	}
	out = append(out, Easter(year), EasterMonday(year))
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// MonthDay is a recurring yearly date, serialised as "MM-DD".
type MonthDay struct {
	Month time.Month
	Day   int
}

// MonthDayOf returns the month/day of d.
func MonthDayOf(d Date) MonthDay { return MonthDay{d.Month(), d.Day()} }

// ParseMonthDay parses "MM-DD".
func ParseMonthDay(s string) (MonthDay, error) {
	// Validate against a leap year so that "02-29" is accepted.
	t, err := time.Parse("2006-01-02", "2000-"+s)
	if err != nil || len(s) != 5 {
		return MonthDay{}, fmt.Errorf("calendar: invalid month-day %q", s)
	}
	return MonthDay{t.Month(), t.Day()}, nil
}

func (md MonthDay) String() string { return fmt.Sprintf("%02d-%02d", int(md.Month), md.Day) }

// In returns the date of md in year. February 29 maps to February 28 in
// non-leap years.
func (md MonthDay) In(year int) Date {
	d := New(year, md.Month, md.Day)
	if d.Month() != md.Month { // overflowed (Feb 29 in a non-leap year)
		return New(year, md.Month+1, 1).AddDays(-1)
	}
	return d
}

func (md MonthDay) MarshalJSON() ([]byte, error) { return json.Marshal(md.String()) }

func (md *MonthDay) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	p, err := ParseMonthDay(s)
	if err != nil {
		return err
	}
	*md = p
	return nil
}

// Holidays is a holiday calendar: Italian national holidays plus local
// holidays of an establishment (e.g. the patron saint), either on specific
// dates or recurring every year. The zero value contains only the national
// holidays. Holidays is immutable and safe for concurrent use.
type Holidays struct {
	dates     map[int]struct{}
	recurring map[MonthDay]struct{}
}

func dateKey(d Date) int { return d.Year()*10000 + int(d.Month())*100 + d.Day() }

// NewHolidays builds a calendar with extra local holidays on specific dates
// and recurring local holidays (same month/day every year).
func NewHolidays(dates []Date, recurring []MonthDay) Holidays {
	h := Holidays{
		dates:     make(map[int]struct{}, len(dates)),
		recurring: make(map[MonthDay]struct{}, len(recurring)),
	}
	for _, d := range dates {
		if !d.IsZero() {
			h.dates[dateKey(d)] = struct{}{}
		}
	}
	for _, md := range recurring {
		h.recurring[md] = struct{}{}
	}
	return h
}

// IsLocal reports whether d is one of the local holidays.
func (h Holidays) IsLocal(d Date) bool {
	if d.IsZero() {
		return false
	}
	if _, ok := h.dates[dateKey(d)]; ok {
		return true
	}
	_, ok := h.recurring[MonthDayOf(d)]
	return ok
}

// IsHoliday reports whether d is a national or local holiday.
func (h Holidays) IsHoliday(d Date) bool { return IsItalianHoliday(d) || h.IsLocal(d) }
