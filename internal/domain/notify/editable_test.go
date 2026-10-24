package notify

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestValidateTexts_Scenarios(t *testing.T) {
	ok := Texts{Subject: "Prenotazione {codice}", Opening: "Ciao {nome} {cognome},\nbenvenuto.", Closing: "A presto", Signature: "Lo staff di {stabilimento}"}
	tests := []struct {
		name   string
		in     Texts
		fields []string
	}{
		{"valid", ok, nil},
		{"every placeholder", Texts{Subject: "{" + strings.Join(Placeholders, "} {") + "}"}, nil},
		{"literal braces are text", Texts{Subject: "Sconto {10%} e { nome } e {}"}, nil},
		{"empty subject", Texts{Subject: ""}, []string{"subject"}},
		{"unknown placeholder", Texts{Subject: "Ciao {name}", Opening: "{nome} {email} {email}"}, []string{"subject", "opening"}},
		{"subject on two lines", Texts{Subject: "a\nb"}, []string{"subject"}},
		{"subject too long", Texts{Subject: strings.Repeat("é", MaxSubjectLen+1)}, []string{"subject"}},
		{"opening at limit", Texts{Subject: "x", Opening: strings.Repeat("a", MaxBodyTextLen)}, nil},
		{"closing too long", Texts{Subject: "x", Closing: strings.Repeat("a", MaxBodyTextLen+1)}, []string{"closing"}},
		{"signature too long", Texts{Subject: "x", Signature: strings.Repeat("a", MaxSignatureLen+1)}, []string{"signature"}},
		{"control character", Texts{Subject: "x", Opening: "a\x00b"}, []string{"opening"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTexts(tt.in)
			if tt.fields == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var te *TemplateError
			if !errors.As(err, &te) || !errors.Is(err, ErrTemplateInvalid) {
				t.Fatalf("want *TemplateError, got %v", err)
			}
			var got []string
			for _, f := range te.Fields {
				got = append(got, f.Field)
			}
			if !slices.Equal(got, tt.fields) {
				t.Fatalf("fields = %v, want %v", got, tt.fields)
			}
		})
	}
}

func TestUnknownPlaceholders_Dedup(t *testing.T) {
	got := UnknownPlaceholders("{a} {nome} {b} {a} {{codice}}")
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("got %v", got)
	}
}

func TestNormalizeTexts_LineEndings(t *testing.T) {
	got := NormalizeTexts(Texts{Subject: "  Ciao  ", Opening: "\r\n riga 1  \r\nriga 2\r\rfine \n"})
	if got.Subject != "Ciao" || got.Opening != "riga 1\nriga 2\n\nfine" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolvePlaceholders_PlainValues(t *testing.T) {
	vals := map[string]string{"nome": "<b>Mario</b>", "codice": "{saldo}"}
	got := ResolvePlaceholders("Ciao {nome}, {codice} {saldo} {ignoto}", vals)
	// Values are inserted once (no recursive expansion) and unknown names stay.
	if got != "Ciao <b>Mario</b>, {saldo}  {ignoto}" {
		t.Fatalf("got %q", got)
	}
}

func TestPlainTextHTML_EscapesAndParagraphs(t *testing.T) {
	h, txt := plainTextHTML("Uno <script>alert(1)</script>\nriga & due\n\n  \n**Tre** [x](javascript:y)")
	if strings.Count(h, "<p ") != 2 || !strings.Contains(h, "Uno &lt;script&gt;alert(1)&lt;/script&gt;<br>riga &amp; due") {
		t.Fatalf("html %s", h)
	}
	if strings.Contains(h, "<strong>") || strings.Contains(h, "<a ") || !strings.Contains(h, "**Tre** [x](javascript:y)") {
		t.Fatalf("markdown must stay literal: %s", h)
	}
	if txt != "Uno <script>alert(1)</script>\nriga & due\n\n**Tre** [x](javascript:y)" {
		t.Fatalf("text %q", txt)
	}
	if h, txt := plainTextHTML("  \n "); h != "" || txt != "" {
		t.Fatal("empty text must render nothing")
	}
}

// TestRender_EditableTextsEscaped renders an edited confirmation whose
// customer data and texts carry markup: nothing reaches the HTML unescaped
// and the subject is one line.
func TestRender_EditableTextsEscaped(t *testing.T) {
	cat, _ := catalogFor("it")
	d := fixtureBooking()
	d.Customer.FirstName = `<img src=x onerror=alert(1)>`
	c := bookingContent(cat, d, contentOptions{})
	texts := ResolveTexts(Texts{
		Subject: "Ciao {nome}\r\nBcc: x@y.z", Opening: "<b>Benvenuto</b> {nome}\n{postazione}", Closing: "Saldo {saldo} – {scadenza}",
		Signature: "Lo staff\n{stabilimento}",
	}, placeholderValues(TemplateBookingConfirmed, c.Vars))
	c.Texts = &texts
	src, err := builtinTemplate(TemplateBookingConfirmed, "it")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(src, cat, BrandingFor(d.Est), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<img src=x", "<b>Benvenuto"} {
		if strings.Contains(r.HTML, bad) {
			t.Errorf("HTML contains %q", bad)
		}
	}
	for _, good := range []string{"&lt;b&gt;Benvenuto&lt;/b&gt; &lt;img src=x onerror=alert(1)&gt;<br>A1", "Saldo 934,50", "– all&#39;arrivo", "Lo staff<br>Lido Delfino"} {
		if !strings.Contains(r.HTML, good) {
			t.Errorf("HTML misses %q", good)
		}
	}
	if strings.ContainsAny(r.Subject, "\r\n") || !strings.HasPrefix(r.Subject, "Ciao <img") {
		t.Errorf("subject %q", r.Subject)
	}
}

// TestDefaultTexts_ValidInEveryLocale: the built-in texts of every editable
// kind pass the editor validation, so "restore default" and a save of the
// unchanged defaults always succeed.
func TestDefaultTexts_ValidInEveryLocale(t *testing.T) {
	for _, k := range EditableKinds {
		for _, l := range Locales {
			def, err := defaultTexts(k, l)
			if err != nil {
				t.Fatalf("%s/%s: %v", k, l, err)
			}
			if def.Subject == "" || def.Opening == "" {
				t.Errorf("%s/%s: empty default subject or opening", k, l)
			}
			if err := ValidateTexts(NormalizeTexts(def)); err != nil {
				t.Errorf("%s/%s: %v", k, l, err)
			}
			if NormalizeTexts(def) != def {
				t.Errorf("%s/%s: defaults are not normalised", k, l)
			}
			if len(FixedBlocks(k, l)) == 0 && k != TemplateBookingRejected {
				t.Errorf("%s/%s: no fixed blocks", k, l)
			}
		}
	}
}

func TestBalanceDueDate_Scenarios(t *testing.T) {
	b := func(snap string) *store.Booking {
		return &store.Booking{StartsOn: calendar.MustParse("2026-07-10"), PolicySnapshot: store.JSON(snap)}
	}
	tests := []struct {
		name      string
		snap      string
		want      string
		onArrival bool
		ok        bool
	}{
		{"days before", `{"version":1,"rules":{"payment":{"balance_due":"days_before:7"}}}`, "2026-07-03", false, true},
		{"days before zero", `{"version":1,"rules":{"payment":{"balance_due":"days_before:0"}}}`, "2026-07-10", false, true},
		{"on arrival", `{"version":1,"rules":{"payment":{"balance_due":"on_arrival"}}}`, "2026-07-10", true, true},
		{"no rule", `{"version":1,"rules":{}}`, "2026-07-10", true, true},
		{"invalid rule", `{"version":1,"rules":{"payment":{"balance_due":"soon"}}}`, "", false, false},
		{"invalid snapshot", `{}`, "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, onArrival, ok := BalanceDueDate(b(tt.snap))
			if ok != tt.ok || onArrival != tt.onArrival || (ok && d.String() != tt.want) {
				t.Fatalf("got %v %v %v", d, onArrival, ok)
			}
		})
	}
}

// TestReminderWindow_TimezonesAndDST checks the send instant and the daily
// window in several timezones, across both DST changes of Europe/Rome and
// on a wall time skipped by the spring-forward change.
func TestReminderWindow_TimezonesAndDST(t *testing.T) {
	load := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	}
	rome, ny, auckland := load("Europe/Rome"), load("America/New_York"), load("Pacific/Auckland")
	utc := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tests := []struct {
		name    string
		now     string
		loc     *time.Location
		hhmm    string
		today   string
		at      string
		wantDue bool
	}{
		{"rome summer before", "2026-07-09T15:59:00Z", rome, "18:00", "2026-07-09", "2026-07-09T16:00:00Z", false},
		{"rome summer at", "2026-07-09T16:00:00Z", rome, "18:00", "2026-07-09", "2026-07-09T16:00:00Z", true},
		{"rome winter", "2026-01-15T17:00:00Z", rome, "18:00", "2026-01-15", "2026-01-15T17:00:00Z", true},
		{"rome spring forward day", "2026-03-29T16:30:00Z", rome, "18:00", "2026-03-29", "2026-03-29T16:00:00Z", true},
		{"rome fall back day", "2026-10-25T16:59:00Z", rome, "18:00", "2026-10-25", "2026-10-25T17:00:00Z", false},
		{"rome skipped wall time", "2026-03-29T02:00:00Z", rome, "02:30", "2026-03-29", "2026-03-29T01:30:00Z", true},
		{"rome late night is still today", "2026-07-09T21:59:00Z", rome, "18:00", "2026-07-09", "2026-07-09T16:00:00Z", true},
		{"rome after midnight is tomorrow", "2026-07-09T22:01:00Z", rome, "18:00", "2026-07-10", "2026-07-10T16:00:00Z", false},
		{"new york dst", "2026-07-09T21:00:00Z", ny, "18:00", "2026-07-09", "2026-07-09T22:00:00Z", false},
		{"new york utc is next day", "2026-07-10T01:00:00Z", ny, "18:00", "2026-07-09", "2026-07-09T22:00:00Z", true},
		{"auckland ahead of utc", "2026-07-09T06:00:00Z", auckland, "18:00", "2026-07-09", "2026-07-09T06:00:00Z", true},
		{"bad time", "2026-07-09T06:00:00Z", rome, "25:00", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			today, at, due := ReminderWindow(utc(tt.now), tt.loc, tt.hhmm)
			if due != tt.wantDue {
				t.Fatalf("due = %v, want %v (at %v)", due, tt.wantDue, at)
			}
			if tt.today == "" {
				return
			}
			if today.String() != tt.today || !at.Equal(utc(tt.at)) {
				t.Fatalf("today %s at %s, want %s %s", today, at.Format(time.RFC3339), tt.today, tt.at)
			}
		})
	}
}

func TestReminderEligible_Scenarios(t *testing.T) {
	base := store.Booking{Status: statusConfirmed, TotalAmount: money.Cents(1000), PaidAmount: 400}
	with := func(f func(*store.Booking)) *store.Booking { b := base; f(&b); return &b }
	tests := []struct {
		name string
		kind string
		b    *store.Booking
		want bool
	}{
		{"day before confirmed", TemplateDayBeforeReminder, &base, true},
		{"balance with balance", TemplateBalanceReminder, &base, true},
		{"balance paid", TemplateBalanceReminder, with(func(b *store.Booking) { b.PaidAmount = 1000 }), false},
		{"test booking", TemplateDayBeforeReminder, with(func(b *store.Booking) { b.Test = true }), false},
		{"cancelled", TemplateDayBeforeReminder, with(func(b *store.Booking) { b.Status = statusCancelled }), false},
		{"expired", TemplateBalanceReminder, with(func(b *store.Booking) { b.Status = statusExpired }), false},
		{"pending payment", TemplateDayBeforeReminder, with(func(b *store.Booking) { b.Status = statusPendingPayment }), false},
		{"checked in", TemplateDayBeforeReminder, with(func(b *store.Booking) { b.Status = statusCheckedIn }), false},
		{"other kind", TemplateBookingConfirmed, &base, false},
		{"nil", TemplateDayBeforeReminder, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReminderEligible(tt.kind, tt.b); got != tt.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
