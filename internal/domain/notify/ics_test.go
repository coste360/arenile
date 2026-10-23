package notify

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/calendar"
)

func TestFoldICSLine_Scenarios(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"short", "SUMMARY:Lido"},
		{"exactly 75", "X:" + strings.Repeat("a", 73)},
		{"76 octets", "X:" + strings.Repeat("a", 74)},
		{"long ascii", "DESCRIPTION:" + strings.Repeat("abcdefghij", 30)},
		{"multibyte at boundary", "SUMMARY:" + strings.Repeat("è", 100)},
		{"emoji", "SUMMARY:" + strings.Repeat("🏖", 40)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := FoldICSLine(tt.in)
			if !strings.HasSuffix(out, "\r\n") {
				t.Fatalf("missing CRLF: %q", out)
			}
			physical := strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n")
			var unfolded strings.Builder
			for i, l := range physical {
				if len(l) > 75 {
					t.Errorf("line %d has %d octets", i, len(l))
				}
				if !utf8.ValidString(l) {
					t.Errorf("line %d splits a UTF-8 sequence: %q", i, l)
				}
				if i > 0 {
					if !strings.HasPrefix(l, " ") {
						t.Errorf("continuation %d without leading space", i)
					}
					l = l[1:]
				}
				unfolded.WriteString(l)
			}
			if unfolded.String() != tt.in {
				t.Fatalf("unfold mismatch:\n%q\n%q", unfolded.String(), tt.in)
			}
			if len(tt.in) <= 75 && len(physical) != 1 {
				t.Fatalf("folded a short line: %q", out)
			}
		})
	}
}

func TestEscapeICSText_Scenarios(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"a,b;c", `a\,b\;c`},
		{`back\slash`, `back\\slash`},
		{"two\nlines", `two\nlines`},
		{"crlf\r\nline", `crlf\nline`},
		{"bell\x07", "bell"},
		{"tab\tok", "tab\tok"},
	}
	for _, tt := range tests {
		if got := EscapeICSText(tt.in); got != tt.want {
			t.Errorf("EscapeICSText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func icsLines(t *testing.T, b []byte) []string {
	t.Helper()
	s := string(b)
	if strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
		t.Fatal("bare LF in calendar")
	}
	// Unfold (RFC 5545 3.1) and check physical line lengths.
	for _, l := range strings.Split(s, "\r\n") {
		if len(l) > 75 {
			t.Fatalf("physical line of %d octets: %q", len(l), l)
		}
	}
	s = strings.ReplaceAll(s, "\r\n ", "")
	return strings.Split(strings.TrimSuffix(s, "\r\n"), "\r\n")
}

func has(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

func TestICSCalendar_AllDayStay(t *testing.T) {
	rome, _ := time.LoadLocation("Europe/Rome")
	cal := ICSCalendar{Name: "Lido Delfino", Events: []ICSEvent{{
		UID: "0190-abc@arenile.example", Summary: "Lido Delfino – prenotazione LD-2027-000001",
		Description: "Postazioni: A1, A2\nhttps://arenile.example/b/lido/m/tok", Location: "Lido Delfino, Via del Mare 1, Rimini",
		URL: "https://arenile.example/b/lido/m/tok", OrganizerName: "Lido; Delfino", OrganizerEmail: "info@lido.example",
		From: calendar.MustParse("2027-07-10"), To: calendar.MustParse("2027-07-12"), TZ: rome,
		Stamp: time.Date(2027, 6, 1, 8, 0, 0, 0, time.UTC), Sequence: 2,
	}}}
	lines := icsLines(t, cal.Bytes())
	for _, want := range []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:" + icsProdID, "CALSCALE:GREGORIAN", "METHOD:PUBLISH",
		"BEGIN:VEVENT", "UID:0190-abc@arenile.example", "DTSTAMP:20270601T080000Z",
		"DTSTART;VALUE=DATE:20270710", "DTEND;VALUE=DATE:20270713", // exclusive end
		"SEQUENCE:2", "STATUS:CONFIRMED", "TRANSP:TRANSPARENT",
		`DESCRIPTION:Postazioni: A1\, A2\nhttps://arenile.example/b/lido/m/tok`,
		`LOCATION:Lido Delfino\, Via del Mare 1\, Rimini`,
		`ORGANIZER;CN="Lido; Delfino":mailto:info@lido.example`,
		"END:VEVENT", "END:VCALENDAR",
	} {
		if !has(lines, want) {
			t.Errorf("missing line %q\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if has(lines, "BEGIN:VTIMEZONE") {
		t.Error("all-day events need no VTIMEZONE")
	}
	if lines[0] != "BEGIN:VCALENDAR" || lines[len(lines)-1] != "END:VCALENDAR" {
		t.Fatal("calendar not wrapped in VCALENDAR")
	}
	assertBalanced(t, lines)
}

func TestICSCalendar_TimedSlotWithTimezone(t *testing.T) {
	rome, _ := time.LoadLocation("Europe/Rome")
	cal := ICSCalendar{Events: []ICSEvent{{
		UID: "x@h", Summary: "Mattina", From: calendar.MustParse("2026-07-10"), To: calendar.MustParse("2026-07-12"),
		StartTime: &ClockTime{8, 30}, EndTime: &ClockTime{13, 0}, TZ: rome,
		Stamp: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Cancelled: true,
	}}}
	lines := icsLines(t, cal.Bytes())
	for _, want := range []string{
		"BEGIN:VTIMEZONE", "TZID:Europe/Rome",
		// DST start 2026-03-29 02:00 CET -> CEST, end 2026-10-25 03:00 CEST -> CET.
		"BEGIN:DAYLIGHT", "DTSTART:20260329T020000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0200", "TZNAME:CEST",
		"BEGIN:STANDARD", "DTSTART:20261025T030000", "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100", "TZNAME:CET",
		"DTSTART;TZID=Europe/Rome:20260710T083000", "DTEND;TZID=Europe/Rome:20260710T130000",
		"RRULE:FREQ=DAILY;COUNT=3", "STATUS:CANCELLED", "SEQUENCE:0",
	} {
		if !has(lines, want) {
			t.Errorf("missing line %q\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if has(lines, "TRANSP:TRANSPARENT") {
		t.Error("timed events keep the default transparency")
	}
	assertBalanced(t, lines)
}

func TestICSCalendar_UTCZoneHasSingleObservance(t *testing.T) {
	cal := ICSCalendar{Events: []ICSEvent{{
		UID: "x@h", Summary: "s", From: calendar.MustParse("2026-07-10"), To: calendar.MustParse("2026-07-10"),
		StartTime: &ClockTime{9, 0}, EndTime: &ClockTime{18, 0}, TZ: time.UTC, Stamp: time.Unix(0, 0),
	}}}
	lines := icsLines(t, cal.Bytes())
	if n := strings.Count(strings.Join(lines, "\n"), "BEGIN:STANDARD"); n != 1 {
		t.Fatalf("STANDARD observances = %d", n)
	}
	if !has(lines, "TZOFFSETFROM:+0000") || has(lines, "RRULE:FREQ=DAILY;COUNT=1") {
		t.Fatalf("unexpected calendar:\n%s", strings.Join(lines, "\n"))
	}
}

func assertBalanced(t *testing.T, lines []string) {
	t.Helper()
	var stack []string
	for _, l := range lines {
		if v, ok := strings.CutPrefix(l, "BEGIN:"); ok {
			stack = append(stack, v)
		}
		if v, ok := strings.CutPrefix(l, "END:"); ok {
			if len(stack) == 0 || stack[len(stack)-1] != v {
				t.Fatalf("unbalanced END:%s", v)
			}
			stack = stack[:len(stack)-1]
		}
		if !strings.Contains(l, ":") {
			t.Fatalf("content line without colon: %q", l)
		}
	}
	if len(stack) != 0 {
		t.Fatalf("unclosed components %v", stack)
	}
}

func TestParseClockTime_Scenarios(t *testing.T) {
	if c, err := ParseClockTime("08:30"); err != nil || c != (ClockTime{8, 30}) {
		t.Fatalf("got %v %v", c, err)
	}
	if _, err := ParseClockTime("25:00"); err == nil {
		t.Fatal("accepted 25:00")
	}
}

// TestICSParamValue_NoInjection: parameter values (ORGANIZER;CN=...) never
// carry DQUOTE or control characters, so an establishment name cannot
// inject content lines into the calendar.
func TestICSParamValue_NoInjection(t *testing.T) {
	for in, want := range map[string]string{
		"Lido Delfino":                        "Lido Delfino",
		"Lido; Delfino":                       `"Lido; Delfino"`,
		"Lido \"Delfino\"":                    "Lido Delfino",
		"Lido\r\nATTENDEE:mailto:x@y.z":        `"LidoATTENDEE:mailto:x@y.z"`,
		"Lido\nBEGIN:VALARM\x7f":               `"LidoBEGIN:VALARM"`,
	} {
		if got := icsParamValue(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
