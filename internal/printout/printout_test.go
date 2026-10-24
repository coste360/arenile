package printout

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/money"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/")

func sampleZ(paper, locale string, closed bool) ZReport {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		panic(err)
	}
	opened := time.Date(2027, 7, 15, 6, 30, 0, 0, time.UTC)
	z := ZReport{
		Locale: locale, Paper: paper, Location: rome, Currency: "EUR",
		Header: Header{
			Name: "Lido Azzurro", AddressLines: []string{"Lungomare Caboto 12", "04024 Gaeta (LT)"},
			LegalName: "Azzurro S.r.l.", VATNumber: "IT01234567890", ConcessionRef: "CDM 12/2020",
		},
		SessionID: "0190f1e2-0000-7000-8000-000000000001", RegisterName: "Bar <spiaggia>", RegisterCode: "bar",
		Closed: closed, OpenedAt: opened, OpenedBy: "Giulia", GeneratedAt: opened.Add(10 * time.Hour),
		OpeningFloat: 10000, MovementsIn: 2000, MovementsOut: 5000, Payments: 3, Refunds: 1,
		Methods: []ZMethodRow{
			{Method: "card", Takings: 6000, Expected: 6000},
			{Method: "cash", Takings: 3000, Expected: 10000},
		},
		VAT: []ZVATRow{
			{RateBP: 1000, Taxable: 1000, Tax: 100, Gross: 1100},
			{RateBP: 2200, Taxable: 6475, Tax: 1425, Gross: 7900},
		},
		Movements: []ZMovement{
			{At: opened.Add(time.Hour), Kind: "in", Amount: 2000, Reason: "cambio"},
			{At: opened.Add(5 * time.Hour), Kind: "out", Amount: 5000, Reason: "versamento <banca>"},
		},
		Notes: "turno mattina",
	}
	if closed {
		c := opened.Add(9 * time.Hour)
		z.ClosedAt, z.ClosedBy = &c, "Marco"
		z.Methods[0].Counted, z.Methods[0].Difference = 6000, 0
		z.Methods[1].Counted, z.Methods[1].Difference = 9800, -200
		z.Difference = -200
	}
	return z
}

func render(t *testing.T, z ZReport) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderZReport(&buf, z); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestRenderZReport_Golden pins the printed document for both papers.
func TestRenderZReport_Golden(t *testing.T) {
	tests := []struct {
		name   string
		paper  string
		locale string
		closed bool
	}{
		{"a4_closed_it", PaperA4, "it", true},
		{"receipt80_closed_it", PaperReceipt80, "it", true},
		{"a4_open_de", PaperA4, "de-CH", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := render(t, sampleZ(tt.paper, tt.locale, tt.closed))
			path := filepath.Join("testdata", "zreport_"+tt.name+".html")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/printout -update)", err)
			}
			if got != string(want) {
				t.Fatalf("%s differs from the golden file (rerun with -update after checking the change)", path)
			}
		})
	}
}

// TestRenderZReport_Structure checks the content independently of layout:
// header, mono logo, escaping, totals, draft marker, locale, paper.
func TestRenderZReport_Structure(t *testing.T) {
	closed := render(t, sampleZ(PaperA4, "it", true))
	for _, want := range []string{
		"<!doctype html>", `<html lang="it">`, "<h1>Lido Azzurro</h1>", "P. IVA IT01234567890", "Concessione CDM 12/2020",
		`<img class="logo" src="data:image/svg&#43;xml;base64,`, `alt="Arenile"`,
		"Chiusura Z — Bar &lt;spiaggia&gt;", "versamento &lt;banca&gt;",
		"15/07/2027 08:30", "15/07/2027 17:30", // Europe/Rome
		"100,00 €", "−2,00 €", "10%", "22%", "@page { size: A4;",
		`data-testid="difference"`, "Firma dell&#39;operatore",
	} {
		if !strings.Contains(closed, want) {
			t.Errorf("closed A4 misses %q", want)
		}
	}
	if strings.Contains(closed, `data-testid="draft"`) || strings.Contains(closed, "<script") {
		t.Error("closed A4 has a draft marker or a script")
	}
	// Methods in print order: cash before card.
	if strings.Index(closed, "Contanti") > strings.Index(closed, "Carta") {
		t.Error("cash must come before card")
	}
	// The totals row adds up the rows.
	if !regexp.MustCompile(`<tfoot><tr>\s*<td>Totale</td>\s*<td class="num wide">90,00\x{a0}€</td>\s*<td class="num">160,00\x{a0}€</td>\s*<td class="num">158,00\x{a0}€</td>`).MatchString(closed) {
		t.Error("totals row")
	}

	open := render(t, sampleZ(PaperReceipt80, "de", false))
	for _, want := range []string{`<html lang="de">`, "Z-Abschluss", `data-testid="draft"`, "Nicht abgeschlossen", "size: 80mm auto", "15.07.2027 08:30", `class="receipt80 receipt"`} {
		if !strings.Contains(open, want) {
			t.Errorf("open receipt misses %q", want)
		}
	}
	if strings.Contains(open, "Gezählt") {
		t.Error("an open session has no counted column")
	}
}

func TestFormatters_Locales(t *testing.T) {
	tests := []struct {
		c      money.Cents
		locale string
		want   string
	}{
		{123450, "it", "1.234,50 €"},
		{123450, "en", "€1,234.50"},
		{123450, "fr", "1 234,50 €"},
		{-5, "de", "−0,05 €"},
		{0, "it", "0,00 €"},
	}
	for _, tt := range tests {
		if got := FormatMoney(tt.c, "EUR", tt.locale); got != tt.want {
			t.Errorf("FormatMoney(%d, %s) = %q, want %q", tt.c, tt.locale, got, tt.want)
		}
	}
	for bp, want := range map[int64]string{2200: "22%", 1000: "10%", 550: "5,5%", 0: "0%", 425: "4,25%"} {
		if got := FormatRate(bp, "it"); got != want {
			t.Errorf("FormatRate(%d) = %q, want %q", bp, got, want)
		}
	}
	for in, want := range map[string]string{"de-CH": "de", "EN_us": "en", "es": "it", "": "it", "fr": "fr"} {
		if got := NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCatalogs_KeyParity keeps the four catalogs aligned with Italian.
func TestCatalogs_KeyParity(t *testing.T) {
	if err := load(); err != nil {
		t.Fatal(err)
	}
	for _, loc := range Locales {
		if len(catalogs[loc]) != len(catalogs[SourceLocale]) {
			t.Errorf("%s has %d keys, it has %d", loc, len(catalogs[loc]), len(catalogs[SourceLocale]))
		}
		for k := range catalogs[SourceLocale] {
			if catalogs[loc][k] == "" {
				t.Errorf("%s misses %q", loc, k)
			}
		}
	}
}

// TestBrandLogoMatchesBrandKit keeps the embedded mono logo identical to the
// brand kit (ADR 0018) and free of live text.
func TestBrandLogoMatchesBrandKit(t *testing.T) {
	kit, err := os.ReadFile(filepath.Join("..", "..", "assets", "brand", "arenile", "outlined", "arenile-logo-horizontal-navy.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(kit, logoSVG) {
		t.Fatal("internal/printout/brand/arenile-logo-horizontal-navy.svg differs from the brand kit: copy it again")
	}
	if bytes.Contains(logoSVG, []byte("<text")) {
		t.Fatal("the print logo must be outlined (no <text>)")
	}
}
