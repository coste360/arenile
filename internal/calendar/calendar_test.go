package calendar

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate_JSONRoundTrip(t *testing.T) {
	type wrap struct {
		D Date `json:"d"`
	}
	for _, s := range []string{"2026-01-01", "2026-02-28", "2028-02-29", "2026-12-31"} {
		in := wrap{MustParse(s)}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		if want := `{"d":"` + s + `"}`; string(b) != want {
			t.Fatalf("marshal = %s, want %s", b, want)
		}
		var out wrap
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if !out.D.Equal(in.D) {
			t.Fatalf("round trip %s -> %s", in.D, out.D)
		}
	}
}

func TestDate_JSONRejectsInvalid(t *testing.T) {
	for _, in := range []string{`"2026-02-30"`, `"01/07/2026"`, `"2026-07-01T00:00:00Z"`, `20260701`, `null`} {
		var d Date
		if err := json.Unmarshal([]byte(in), &d); err == nil && in != `null` {
			t.Errorf("%s: expected error, got %v", in, d)
		}
	}
}

func TestDate_TextRoundTrip(t *testing.T) {
	d := New(2026, time.August, 15)
	b, err := d.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	var out Date
	if err := out.UnmarshalText(b); err != nil || !out.Equal(d) {
		t.Fatalf("text round trip: %v %v", out, err)
	}
	if err := out.UnmarshalText([]byte("ferragosto")); err == nil {
		t.Fatal("expected error")
	}
}

func TestDate_SQLRoundTrip(t *testing.T) {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Skip("tzdata not available")
	}
	d := New(2026, time.July, 1)
	v, err := d.Value()
	if err != nil || v != "2026-07-01" {
		t.Fatalf("Value = %v, %v", v, err)
	}
	tests := []struct {
		name string
		src  any
	}{
		{"string", "2026-07-01"},
		{"bytes", []byte("2026-07-01")},
		{"time utc", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		// a DATE column read by a driver in a local zone keeps its calendar day
		{"time local", time.Date(2026, 7, 1, 0, 0, 0, 0, rome)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Date
			if err := got.Scan(tt.src); err != nil {
				t.Fatal(err)
			}
			if !got.Equal(d) || got.Time().Location() != time.UTC {
				t.Fatalf("Scan(%v) = %v", tt.src, got)
			}
		})
	}
	var got Date
	for _, bad := range []any{42, nil, "2026-13-01"} {
		if err := got.Scan(bad); err == nil {
			t.Errorf("Scan(%v): expected error", bad)
		}
	}
}

func TestIn_UsesLocation(t *testing.T) {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Skip("tzdata not available")
	}
	// 22:30 UTC on 30 June is already 1 July in Rome (CEST, UTC+2).
	instant := time.Date(2026, 6, 30, 22, 30, 0, 0, time.UTC)
	if got := In(instant, rome); got.String() != "2026-07-01" {
		t.Fatalf("In(Rome) = %s", got)
	}
	if got := In(instant, time.UTC); got.String() != "2026-06-30" {
		t.Fatalf("In(UTC) = %s", got)
	}
}

func TestRange_Inclusive(t *testing.T) {
	from, to := MustParse("2026-03-28"), MustParse("2026-04-02") // spans the DST change
	r := Range(from, to)
	if len(r) != 6 || from.DaysUntil(to) != 5 || !r[5].Equal(to) {
		t.Fatalf("Range = %v, DaysUntil = %d", r, from.DaysUntil(to))
	}
	if Range(to, from) != nil {
		t.Fatal("reversed range must be nil")
	}
	if MustParse("2026-07-05").ISOWeekday() != 7 || MustParse("2026-07-06").ISOWeekday() != 1 {
		t.Fatal("ISOWeekday")
	}
}

func TestSlots_Expand(t *testing.T) {
	for _, s := range []Slots{SlotsAM, SlotsPM, SlotsFull} {
		if !s.Valid() || len(s.Expand()) == 0 {
			t.Errorf("%q: valid=%v expand=%v", s, s.Valid(), s.Expand())
		}
	}
	if len(SlotsFull.Expand()) != 2 {
		t.Fatalf("full day = %v", SlotsFull.Expand())
	}
	if Slots("night").Valid() {
		t.Fatal("unknown slots must be invalid")
	}
}
