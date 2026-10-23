package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
)

// iCalendar (RFC 5545) rendering: a pure builder (functional core) used by
// Service.ICS and by the confirmation email attachment.
//
// Conformance notes:
//   - lines end with CRLF and are folded at 75 octets (3.1) without
//     splitting a UTF-8 sequence; continuation lines start with one space;
//   - TEXT values escape backslash, semicolon, comma and newlines (3.3.11);
//   - all-day events use VALUE=DATE with an exclusive DTEND (3.6.1);
//   - timed events carry TZID and the matching VTIMEZONE (3.6.5), generated
//     from the Go time zone database for the years the event spans; an event
//     repeated on several days (a half-day slot over a stay) uses
//     RRULE:FREQ=DAILY;COUNT=n;
//   - UID is stable per booking ("<booking id>@<host>") and SEQUENCE follows
//     the booking version, so calendar clients update the same event after a
//     modification or a cancellation (STATUS:CANCELLED).

// ICSContentType is the media type of .ics files.
const ICSContentType = "text/calendar; charset=utf-8"

const icsProdID = "-//Coste360//Arenile//IT"

// ClockTime is a wall-clock time of day in the establishment timezone.
type ClockTime struct {
	Hour, Minute int
}

// ParseClockTime parses "HH:MM".
func ParseClockTime(s string) (ClockTime, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return ClockTime{}, fmt.Errorf("notify: clock time %q: %w", s, err)
	}
	return ClockTime{Hour: t.Hour(), Minute: t.Minute()}, nil
}

// ICSEvent is one VEVENT.
type ICSEvent struct {
	UID         string
	Summary     string
	Description string
	Location    string
	URL         string
	// Organizer is the establishment contact (CN + mailto), optional.
	OrganizerName  string
	OrganizerEmail string
	// From and To are the first and last day (inclusive).
	From, To calendar.Date
	// StartTime and EndTime, when both set, make a timed event repeated
	// every day from From to To in Location TZ; nil = all-day event.
	StartTime, EndTime *ClockTime
	TZ                 *time.Location
	// Stamp is DTSTAMP (UTC); Sequence the revision (booking version - 1).
	Stamp     time.Time
	Sequence  int
	Cancelled bool
}

// ICSCalendar is a VCALENDAR with its events.
type ICSCalendar struct {
	Name   string
	Events []ICSEvent
}

// Bytes renders the calendar.
func (c ICSCalendar) Bytes() []byte {
	w := &icsWriter{}
	w.line("BEGIN:VCALENDAR")
	w.line("VERSION:2.0")
	w.line("PRODID:" + icsProdID)
	w.line("CALSCALE:GREGORIAN")
	w.line("METHOD:PUBLISH")
	if c.Name != "" {
		w.line("X-WR-CALNAME:" + EscapeICSText(c.Name))
	}
	// One VTIMEZONE per distinct TZID of timed events.
	seen := map[string]bool{}
	for _, e := range c.Events {
		if !e.timed() || seen[e.TZ.String()] {
			continue
		}
		seen[e.TZ.String()] = true
		writeVTimezone(w, e.TZ, e.From.Year(), e.To.Year())
	}
	for _, e := range c.Events {
		writeVEvent(w, e)
	}
	w.line("END:VCALENDAR")
	return []byte(w.b.String())
}

func (e ICSEvent) timed() bool {
	return e.StartTime != nil && e.EndTime != nil && e.TZ != nil
}

func writeVEvent(w *icsWriter, e ICSEvent) {
	w.line("BEGIN:VEVENT")
	w.line("UID:" + EscapeICSText(e.UID))
	w.line("DTSTAMP:" + e.Stamp.UTC().Format("20060102T150405Z"))
	if e.timed() {
		tzid := e.TZ.String()
		start := time.Date(e.From.Year(), e.From.Month(), e.From.Day(), e.StartTime.Hour, e.StartTime.Minute, 0, 0, e.TZ)
		end := time.Date(e.From.Year(), e.From.Month(), e.From.Day(), e.EndTime.Hour, e.EndTime.Minute, 0, 0, e.TZ)
		w.line("DTSTART;TZID=" + tzid + ":" + start.Format("20060102T150405"))
		w.line("DTEND;TZID=" + tzid + ":" + end.Format("20060102T150405"))
		if days := e.From.DaysUntil(e.To) + 1; days > 1 {
			w.line("RRULE:FREQ=DAILY;COUNT=" + strconv.Itoa(days))
		}
	} else {
		w.line("DTSTART;VALUE=DATE:" + icsDate(e.From))
		w.line("DTEND;VALUE=DATE:" + icsDate(e.To.AddDays(1)))
		// All-day events do not block time in the attendee's calendar.
		w.line("TRANSP:TRANSPARENT")
	}
	w.line("SEQUENCE:" + strconv.Itoa(max(e.Sequence, 0)))
	if e.Cancelled {
		w.line("STATUS:CANCELLED")
	} else {
		w.line("STATUS:CONFIRMED")
	}
	w.line("SUMMARY:" + EscapeICSText(e.Summary))
	if e.Description != "" {
		w.line("DESCRIPTION:" + EscapeICSText(e.Description))
	}
	if e.Location != "" {
		w.line("LOCATION:" + EscapeICSText(e.Location))
	}
	if e.URL != "" {
		w.line("URL:" + e.URL)
	}
	if e.OrganizerEmail != "" {
		cn := ""
		if e.OrganizerName != "" {
			cn = ";CN=" + icsParamValue(e.OrganizerName)
		}
		w.line("ORGANIZER" + cn + ":mailto:" + e.OrganizerEmail)
	}
	w.line("END:VEVENT")
}

// writeVTimezone emits the VTIMEZONE of loc with the explicit observances
// in effect from January 1st of fromYear to the end of toYear (no RRULE: the
// transitions come from the zone database, so historical and future rule
// changes are exact for the period of the booking).
func writeVTimezone(w *icsWriter, loc *time.Location, fromYear, toYear int) {
	w.line("BEGIN:VTIMEZONE")
	w.line("TZID:" + loc.String())
	start := time.Date(fromYear, time.January, 1, 0, 0, 0, 0, loc)
	end := time.Date(toYear+1, time.January, 1, 0, 0, 0, 0, loc)
	// The observance in effect at the start of the period.
	name, offset := start.Zone()
	kind := "STANDARD"
	if start.IsDST() {
		kind = "DAYLIGHT"
	}
	writeObservance(w, kind, start, offset, offset, name)
	t := start
	for {
		_, zoneEnd := t.ZoneBounds()
		if zoneEnd.IsZero() || !zoneEnd.Before(end) {
			break
		}
		_, from := t.Zone()
		next := zoneEnd.In(loc)
		name, to := next.Zone()
		kind := "STANDARD"
		if next.IsDST() {
			kind = "DAYLIGHT"
		}
		// DTSTART is the local time of the transition in the previous offset.
		wall := zoneEnd.UTC().Add(time.Duration(from) * time.Second)
		writeObservance(w, kind, wall, from, to, name)
		t = next
	}
	w.line("END:VTIMEZONE")
}

func writeObservance(w *icsWriter, kind string, wall time.Time, from, to int, name string) {
	w.line("BEGIN:" + kind)
	w.line("DTSTART:" + time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, time.UTC).Format("20060102T150405"))
	w.line("TZOFFSETFROM:" + icsOffset(from))
	w.line("TZOFFSETTO:" + icsOffset(to))
	if name != "" {
		w.line("TZNAME:" + EscapeICSText(name))
	}
	w.line("END:" + kind)
}

func icsDate(d calendar.Date) string { return d.Time().Format("20060102") }

// icsOffset formats a UTC offset in seconds as ±HHMM[SS].
func icsOffset(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	s := fmt.Sprintf("%s%02d%02d", sign, sec/3600, sec%3600/60)
	if sec%60 != 0 {
		s += fmt.Sprintf("%02d", sec%60)
	}
	return s
}

// EscapeICSText escapes a TEXT value (RFC 5545 3.3.11).
func EscapeICSText(s string) string {
	var b strings.Builder
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ';':
			b.WriteString(`\;`)
		case ',':
			b.WriteString(`\,`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
		default:
			if r < 0x20 && r != '\t' {
				continue // control characters are not allowed in TEXT
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// icsParamValue quotes a parameter value when needed (3.2): DQUOTE is not
// allowed inside, so it is dropped, and control characters (CR, LF, ...)
// are removed so that a value never injects a new content line.
func icsParamValue(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, s)
	if strings.ContainsAny(s, ":;,") {
		return `"` + s + `"`
	}
	return s
}

// icsWriter accumulates folded content lines.
type icsWriter struct{ b strings.Builder }

func (w *icsWriter) line(s string) { w.b.WriteString(FoldICSLine(s)) }

// FoldICSLine terminates a content line with CRLF, folding it so that no
// physical line exceeds 75 octets (excluding CRLF) and no UTF-8 sequence is
// split (RFC 5545 3.1).
func FoldICSLine(s string) string {
	const limit = 75
	var b strings.Builder
	first := true
	for len(s) > 0 {
		room := limit
		if !first {
			room = limit - 1 // the leading space counts
		}
		if len(s) <= room {
			if !first {
				b.WriteByte(' ')
			}
			b.WriteString(s)
			break
		}
		cut := room
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		if !first {
			b.WriteByte(' ')
		}
		b.WriteString(s[:cut])
		b.WriteString("\r\n")
		s = s[cut:]
		first = false
	}
	b.WriteString("\r\n")
	return b.String()
}
