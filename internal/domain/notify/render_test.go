package notify

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

var update = flag.Bool("update", false, "rewrite the golden files of the notify templates")

// fixtureBooking is a fixed booking for the golden tests (no database).
func fixtureBooking() *bookingData {
	loc := "it"
	est := &store.Establishment{
		ID: "est-1", Slug: "lido-delfino", Name: "Lido Delfino", Timezone: "Europe/Rome", Currency: "EUR",
		DefaultLocale: "it", ContactEmail: "info@lidodelfino.example", ContactPhone: "+39 0541 123456",
		Address: store.Address{Street: "Via del Mare 1", PostalCode: "47921", City: "Rimini", Province: "RN"},
		Theme:   store.Theme{PrimaryColor: "#f2b134", LogoURL: "https://lidodelfino.example/logo.png"},
	}
	zoneID, packID, extraID, spot := "zone-1", "pack-1", "extra-1", "spot-1"
	return &bookingData{
		Est: est,
		Booking: &store.Booking{
			ID: "0190a000-0000-7000-8000-000000000001", Code: "LD-2027-000001", Status: statusConfirmed,
			StartsOn: calendar.MustParse("2027-07-10"), EndsOn: calendar.MustParse("2027-07-12"), Slots: calendar.SlotsFull,
			TotalAmount: money.Cents(123450), PaidAmount: 30000, DepositDue: 37000, Locale: &loc, Version: 3,
			PolicySnapshot: store.JSON(`{"version":1,"rules":{"cancellation":[{"until_days_before":2,"refund_percent":50,"as":"credit"},{"until_days_before":7,"refund_percent":100}],"modification":{"allowed_until_days_before":3}},"resolved_at":"2027-06-01T08:00:00Z"}`),
		},
		Customer: &store.Customer{ID: "cu-1", FirstName: "Mario", LastName: "Rossi", Locale: "it"},
		Lines:    []store.BookingLine{{PackID: packID, ZoneID: zoneID, SpotID: &spot, Quantity: 1, AssignmentStatus: store.AssignmentAssigned}},
		Zones:    map[string]store.Zone{zoneID: {ID: zoneID, Name: store.I18nText{"it": "Prima fila", "en": "Front row", "de": "Erste Reihe", "fr": "Premier rang"}}},
		Packs:    map[string]store.Pack{packID: {ID: packID, Name: store.I18nText{"it": "Ombrellone e 2 lettini", "en": "Umbrella and 2 sunbeds"}}},
		Extras:   []store.BookingLineExtra{{ExtraID: extraID, Quantity: 2}},
		ExtraDef: map[string]store.Extra{extraID: {ID: extraID, Name: store.I18nText{"it": "Lettino aggiuntivo", "de": "Zusätzliche Liege"}}},
		Spots:    []string{"A1"},
	}
}

const fixtureManageURL = "https://arenile.example/b/lido-delfino/m/TOKEN_TOKEN_TOKEN_TOKEN_TOKEN_TOKEN_TOKEN_A"

// fixtureContent mirrors what compose passes for each template.
func fixtureContent(t *testing.T, tpl string, cat catalog) Content {
	t.Helper()
	d := fixtureBooking()
	if tpl == TemplateTest {
		return Content{Locale: cat.locale, Manager: true, Contacts: establishmentContacts(d.Est), Vars: map[string]string{"establishment.name": d.Est.Name}}
	}
	expires := time.Date(2027, 8, 12, 22, 0, 0, 0, time.UTC)
	o := contentOptions{Manager: managerTemplates[tpl], Vars: map[string]string{
		"refund.amount": formatMoney(15000, "EUR", cat.locale), "payment.amount": formatMoney(37000, "EUR", cat.locale),
		"message.template": TemplateBookingConfirmed, "message.attempts": "8",
	}}
	if waitlistTemplates[tpl] {
		vars := map[string]string{
			"establishment.name": d.Est.Name, "customer.first_name": d.Customer.FirstName, "customer.last_name": d.Customer.LastName,
			"waitlist.period": formatPeriod(cat, d.Booking.StartsOn, d.Booking.EndsOn), "waitlist.pack": "Ombrellone e 2 lettini",
			"waitlist_url": fixtureManageURL, "offer.expires": formatDateTime(cat, expires),
		}
		return Content{Locale: cat.locale, Vars: vars, Contacts: establishmentContacts(d.Est)}
	}
	switch tpl {
	case TemplateBookingConfirmed, TemplateBookingModified:
		o.QR, o.ICS, o.Policy, o.ManageURL, o.LinkExpires = true, true, tpl == TemplateBookingConfirmed, fixtureManageURL, expires
	case TemplateManageLink:
		o.ManageURL, o.LinkExpires = fixtureManageURL, expires
	case TemplateRenewalProposal, TemplateRenewalReminder:
		o.ManageURL, o.LinkExpires, o.Policy = fixtureManageURL, expires, tpl == TemplateRenewalProposal
		o.Vars["renewal.deadline"] = formatDeadline(cat, time.Date(2027, 3, 31, 23, 59, 0, 0, time.UTC))
	}
	return bookingContent(cat, d, o)
}

func TestRender_Golden(t *testing.T) {
	for _, tpl := range Templates {
		for _, loc := range Locales {
			t.Run(tpl+"/"+loc, func(t *testing.T) {
				cat, err := catalogFor(loc)
				if err != nil {
					t.Fatal(err)
				}
				src, err := builtinTemplate(tpl, loc)
				if err != nil {
					t.Fatal(err)
				}
				r, err := Render(src, cat, BrandingFor(fixtureBooking().Est), fixtureContent(t, tpl, cat))
				if err != nil {
					t.Fatal(err)
				}
				checkRendered(t, tpl, r)
				golden(t, filepath.Join("testdata", "golden", tpl+"."+loc+".txt"), "Subject: "+r.Subject+"\n\n"+r.Text)
				golden(t, filepath.Join("testdata", "golden", tpl+"."+loc+".html"), r.HTML)
			})
		}
	}
}

// checkRendered asserts properties every rendered email must have.
func checkRendered(t *testing.T, tpl string, r Rendered) {
	t.Helper()
	for _, part := range []string{r.Subject, r.Text, r.HTML} {
		if strings.Contains(part, "{{") || strings.Contains(part, "}}") {
			t.Fatalf("unrendered variable or partial in %q", part)
		}
	}
	if strings.ContainsAny(r.Subject, "\r\n") {
		t.Fatal("subject contains a line break")
	}
	// No tracking: no remote images except the https logo, no pixels.
	for _, m := range regexp.MustCompile(`<img[^>]+src="([^"]+)"`).FindAllStringSubmatch(r.HTML, -1) {
		if m[1] != "https://lidodelfino.example/logo.png" && m[1] != "cid:"+QRContentID {
			t.Fatalf("unexpected image %q", m[1])
		}
	}
	if strings.Contains(r.HTML, `width="1"`) || strings.Contains(r.HTML, "ZgotmplZ") {
		t.Fatal("tracking pixel or unsafe template value")
	}
	wantQR := tpl == TemplateBookingConfirmed || tpl == TemplateBookingModified
	if got := strings.Contains(r.HTML, "cid:"+QRContentID); got != wantQR {
		t.Fatalf("QR image present = %v, want %v", got, wantQR)
	}
	wantLink := wantQR || tpl == TemplateManageLink || waitlistTemplates[tpl] || renewalTemplates[tpl]
	if got := strings.Contains(r.HTML, fixtureManageURL) && strings.Contains(r.Text, fixtureManageURL); got != wantLink {
		t.Fatalf("manage link present = %v, want %v", got, wantLink)
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file (run go test -run TestRender_Golden -update): %v", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from the rendering (run with -update after reviewing):\n%s", path, firstDiff(string(want), got))
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range max(len(al), len(bl)) {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return "line " + strconv.Itoa(i+1) + ":\n- " + x + "\n+ " + y
		}
	}
	return ""
}

func TestRender_EscapesValuesAndRejectsUnsafeLinks(t *testing.T) {
	cat, _ := catalogFor("it")
	src, err := parseTemplate("---\nsubject: Ciao {{customer.first_name}}\n---\n# Ciao {{customer.first_name}}\n\n" +
		"[clicca](javascript:alert(1)) e [sito](https://lido.example/?a=1&b=2) e [gestisci]({{manage_url}})\n\n" +
		"{{> unknown_partial}}\n\n{{> qr}}\n\n<b>raw</b> **forte** _corsivo_ {{customer.last_name}}\n\n- uno\n- due")
	if err != nil {
		t.Fatal(err)
	}
	c := Content{Vars: map[string]string{
		"customer.first_name": "<script>x</script>\r\nBcc: a@b.c", "customer.last_name": "**non_markdown**",
		"manage_url": `https://x.example/m/"onmouseover`,
	}}
	r, err := Render(src, cat, BrandingFor(&store.Establishment{Name: "L"}), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script>", "javascript:", "<b>raw</b>", `"onmouseover`, "unknown_partial", "cid:"} {
		if strings.Contains(r.HTML, bad) {
			t.Errorf("HTML contains %q", bad)
		}
	}
	for _, good := range []string{
		"&lt;script&gt;", "&lt;b&gt;raw&lt;/b&gt;", "<strong>forte</strong>", "<em>corsivo</em>",
		`href="https://lido.example/?a=1&amp;b=2"`, "**non_markdown**", `href="https://x.example/m/&#34;onmouseover"`, "<li",
	} {
		if !strings.Contains(r.HTML, good) {
			t.Errorf("HTML misses %q\n%s", good, r.HTML)
		}
	}
	if r.Subject != "Ciao <script>x</script> Bcc: a@b.c" {
		t.Errorf("subject %q", r.Subject)
	}
	if !strings.Contains(r.Text, "sito (https://lido.example/?a=1&b=2)") || strings.Contains(r.Text, "javascript") {
		t.Errorf("text links:\n%s", r.Text)
	}
}

func TestCatalogs_KeyParity(t *testing.T) {
	all, err := loadCatalogs()
	if err != nil {
		t.Fatal(err)
	}
	keys := func(m map[string]string) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}
	src := keys(all[SourceLocale])
	for _, l := range Locales {
		if got := keys(all[l]); !slices.Equal(got, src) {
			t.Errorf("catalog %s keys differ from %s", l, SourceLocale)
		}
		for k, v := range all[l] {
			if strings.Count(v, "{") != strings.Count(all[SourceLocale][k], "{") {
				t.Errorf("%s.%s placeholders differ from the source", l, k)
			}
		}
	}
}

func TestTemplates_ParityAndVariables(t *testing.T) {
	known := map[string]bool{
		"establishment.name": true, "customer.first_name": true, "customer.last_name": true, "booking.code": true,
		"booking.period": true, "booking.total": true, "booking.paid": true, "booking.balance": true, "manage_url": true,
		"link.expires": true, "refund.amount": true, "payment.amount": true, "message.template": true, "message.attempts": true,
	}
	for _, v := range WaitlistVars {
		known[v] = true
	}
	for _, v := range RenewalVars {
		known[v] = true
	}
	partials := map[string]bool{"summary": true, "qr": true, "manage_button": true, "policy": true, "ics_note": true}
	for _, tpl := range Templates {
		var srcParts []string
		for _, l := range Locales {
			raw, err := templateFS.ReadFile("templates/" + l + "/" + tpl + ".md")
			if err != nil {
				t.Errorf("template %s missing in %s", tpl, l)
				continue
			}
			src, err := parseTemplate(string(raw))
			if err != nil {
				t.Errorf("%s/%s: %v", l, tpl, err)
				continue
			}
			for _, m := range reVar.FindAllStringSubmatch(src.Subject+src.Preheader+src.Body, -1) {
				if !known[m[1]] {
					t.Errorf("%s/%s uses unknown variable %q", l, tpl, m[1])
				}
			}
			var ps []string
			for _, m := range regexp.MustCompile(`\{\{>\s*([a-z_]+)\s*\}\}`).FindAllStringSubmatch(src.Body, -1) {
				if !partials[m[1]] {
					t.Errorf("%s/%s uses unknown partial %q", l, tpl, m[1])
				}
				ps = append(ps, m[1])
			}
			if l == SourceLocale {
				srcParts = ps
			} else if !slices.Equal(ps, srcParts) {
				t.Errorf("%s/%s partials %v differ from the source %v", l, tpl, ps, srcParts)
			}
		}
	}
}

func TestNormalizeLocale_Scenarios(t *testing.T) {
	for in, want := range map[string]string{"it": "it", "de-CH": "de", "EN_us": "en", "fr": "fr", "es": "", "": ""} {
		if got := NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
	if pickLocale("es", "", "de") != "de" || pickLocale() != "it" {
		t.Fatal("pickLocale fallback")
	}
}

func TestPolicyLines_Scenarios(t *testing.T) {
	cat, _ := catalogFor("it")
	got := policyLines(cat, fixtureBooking().Booking.PolicySnapshot)
	want := []string{
		"Fino a 7 giorni prima dell'arrivo: rimborso del 100%",
		"Fino a 2 giorni prima dell'arrivo: credito del 50%",
		"Dopo questi termini l'importo pagato non è rimborsabile.",
		"Puoi modificare la prenotazione fino a 3 giorni prima dell'arrivo.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
	none := policyLines(cat, store.JSON(`{"version":1,"rules":{},"resolved_at":"2027-06-01T08:00:00Z"}`))
	if !slices.Equal(none, []string{"Questa prenotazione non prevede rimborsi in caso di cancellazione."}) {
		t.Fatalf("no tiers: %q", none)
	}
	if policyLines(cat, store.JSON(`{}`)) != nil {
		t.Fatal("invalid snapshot must give no lines")
	}
}
