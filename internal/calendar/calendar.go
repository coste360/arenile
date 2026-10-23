// Package calendar models calendar dates (no time, no zone) and half-day slots.
// Dates are always interpreted in the establishment timezone.
package calendar

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const layout = "2006-01-02"

// Date is a calendar day, serialised as "YYYY-MM-DD".
type Date struct {
	t time.Time // always midnight UTC
}

// New builds a Date from year, month, day.
func New(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// Parse parses "YYYY-MM-DD".
func Parse(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("calendar: invalid date %q: %w", s, err)
	}
	return Date{t}, nil
}

// MustParse is Parse that panics; for tests and constants.
func MustParse(s string) Date {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// In returns the calendar date of instant t in loc.
func In(t time.Time, loc *time.Location) Date {
	y, m, d := t.In(loc).Date()
	return New(y, m, d)
}

func (d Date) String() string       { return d.t.Format(layout) }
func (d Date) IsZero() bool         { return d.t.IsZero() }
func (d Date) Year() int            { return d.t.Year() }
func (d Date) Month() time.Month    { return d.t.Month() }
func (d Date) Day() int             { return d.t.Day() }
func (d Date) AddDays(n int) Date   { return Date{d.t.AddDate(0, 0, n)} }
func (d Date) Before(o Date) bool   { return d.t.Before(o.t) }
func (d Date) After(o Date) bool    { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool    { return d.t.Equal(o.t) }
func (d Date) Time() time.Time      { return d.t }
func (d Date) DaysUntil(o Date) int { return int(o.t.Sub(d.t).Hours() / 24) }

// ISOWeekday returns 1 (Monday) .. 7 (Sunday).
func (d Date) ISOWeekday() int {
	wd := int(d.t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

// Range returns all dates from 'from' to 'to' inclusive.
func Range(from, to Date) []Date {
	if to.Before(from) {
		return nil
	}
	out := make([]Date, 0, from.DaysUntil(to)+1)
	for d := from; !d.After(to); d = d.AddDays(1) {
		out = append(out, d)
	}
	return out
}

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	p, err := Parse(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}

func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *Date) UnmarshalText(b []byte) error {
	p, err := Parse(string(b))
	if err != nil {
		return err
	}
	*d = p
	return nil
}

// Bind implements the oapi-codegen runtime Binder interface, so generated
// handlers can read a Date from query and path parameters ("YYYY-MM-DD").
func (d *Date) Bind(src string) error { return d.UnmarshalText([]byte(src)) }

// Value implements driver.Valuer (stored as "YYYY-MM-DD" text / DATE).
func (d Date) Value() (driver.Value, error) { return d.String(), nil }

// Scan implements sql.Scanner.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return d.UnmarshalText([]byte(v))
	case []byte:
		return d.UnmarshalText(v)
	case time.Time:
		*d = New(v.Year(), v.Month(), v.Day())
		return nil
	default:
		return fmt.Errorf("calendar: cannot scan %T", src)
	}
}

// Slot is a half-day unit.
type Slot string

const (
	AM Slot = "AM"
	PM Slot = "PM"
)

// Slots is the booked span of a day: AM, PM or FULL (= AM+PM).
type Slots string

const (
	SlotsAM   Slots = "AM"
	SlotsPM   Slots = "PM"
	SlotsFull Slots = "FULL"
)

// Expand returns the individual slots covered.
func (s Slots) Expand() []Slot {
	switch s {
	case SlotsAM:
		return []Slot{AM}
	case SlotsPM:
		return []Slot{PM}
	case SlotsFull:
		return []Slot{AM, PM}
	}
	return nil
}

// Valid reports whether s is a known value.
func (s Slots) Valid() bool { return s == SlotsAM || s == SlotsPM || s == SlotsFull }
