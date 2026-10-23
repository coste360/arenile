package notify

import (
	"testing"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/money"
	"github.com/coste360/arenile/internal/store"
)

func TestBrandingFor_ContrastIsAA(t *testing.T) {
	tests := []struct {
		primary, wantPrimary, wantOn string
	}{
		{"#0E6BA8", "#0E6BA8", "#FFFFFF"},
		{"#f2b134", "#F2B134", "#111111"},
		{"#FFF", "#FFFFFF", "#111111"},
		{"#000000", "#000000", "#FFFFFF"},
		{"", DefaultPrimaryColor, "#FFFFFF"},
		{"red", DefaultPrimaryColor, "#FFFFFF"},
		{"#12345G", DefaultPrimaryColor, "#FFFFFF"},
	}
	for _, tt := range tests {
		b := BrandingFor(&store.Establishment{Name: "L", Theme: store.Theme{PrimaryColor: tt.primary}})
		if b.Primary != tt.wantPrimary || b.OnPrimary != tt.wantOn {
			t.Errorf("%q: got %s on %s", tt.primary, b.OnPrimary, b.Primary)
		}
		if r := ContrastRatio(b.Primary, b.OnPrimary); r < 4.5 {
			t.Errorf("%q: text %s on %s contrast %.2f < 4.5", tt.primary, b.OnPrimary, b.Primary, r)
		}
		if r := ContrastRatio(b.Link, "#FFFFFF"); r < 4.5 {
			t.Errorf("%q: link %s contrast %.2f < 4.5", tt.primary, b.Link, r)
		}
	}
}

func TestBrandingFor_LogoHTTPSOnly(t *testing.T) {
	for url, want := range map[string]string{
		"https://lido.example/logo.png": "https://lido.example/logo.png",
		"http://lido.example/logo.png":  "",
		"javascript:alert(1)":           "",
		"data:image/png;base64,AAAA":    "",
	} {
		if got := BrandingFor(&store.Establishment{Theme: store.Theme{LogoURL: url}}).LogoURL; got != want {
			t.Errorf("%q -> %q", url, got)
		}
	}
}

func TestContrastRatio_Known(t *testing.T) {
	if r := ContrastRatio("#000000", "#FFFFFF"); r < 20.99 || r > 21.01 {
		t.Fatalf("black/white = %f", r)
	}
	if r := ContrastRatio("#777777", "#777777"); r != 1 {
		t.Fatalf("same colour = %f", r)
	}
}

func TestFormatMoney_Locales(t *testing.T) {
	tests := []struct {
		c      money.Cents
		cur    string
		locale string
		want   string
	}{
		{123450, "EUR", "it", "1.234,50 €"},
		{123450, "EUR", "de", "1.234,50 €"},
		{123450, "EUR", "fr", "1 234,50 €"},
		{123450, "EUR", "en", "€1,234.50"},
		{5, "EUR", "it", "0,05 €"},
		{-1999, "EUR", "en", "-€19.99"},
		{100000000, "CHF", "it", "1.000.000,00 CHF"},
	}
	for _, tt := range tests {
		if got := formatMoney(tt.c, tt.cur, tt.locale); got != tt.want {
			t.Errorf("formatMoney(%d, %s, %s) = %q, want %q", tt.c, tt.cur, tt.locale, got, tt.want)
		}
	}
}

func TestFormatDate_Locales(t *testing.T) {
	d := calendar.MustParse("2026-07-11")
	for loc, want := range map[string]string{
		"it": "sabato 11 luglio 2026", "en": "Saturday, 11 July 2026",
		"de": "Samstag, 11. Juli 2026", "fr": "samedi 11 juillet 2026",
	} {
		cat, _ := catalogFor(loc)
		if got := formatDate(cat, d); got != want {
			t.Errorf("%s: %q, want %q", loc, got, want)
		}
	}
	cat, _ := catalogFor("it")
	if got := formatPeriod(cat, d, d.AddDays(2)); got != "dal sabato 11 luglio 2026 al lunedì 13 luglio 2026" {
		t.Errorf("period %q", got)
	}
}

func TestMarkdown_Blocks(t *testing.T) {
	blocks := parseBlocks("# T\n\npara one\nline two\n\n- a\n- b\n\n## S\n\n### s\n\n- a\nnot item")
	kinds := ""
	for _, b := range blocks {
		kinds += b.kind + " "
	}
	if kinds != "h1 p ul h2 h3 p " {
		t.Fatalf("kinds %q", kinds)
	}
	if got := blockHTML(blocks[1], mdStyle{Link: "#000"}, nil); got != `<p style="`+styleP+`">para one<br>line two</p>` {
		t.Fatalf("paragraph %q", got)
	}
	if got := blockText(blocks[0], nil); got != "T\n===" {
		t.Fatalf("heading text %q", got)
	}
}

func TestMarkdown_ItalicDoesNotTouchVariables(t *testing.T) {
	got := blockHTML(mdBlock{kind: "p", lines: []string{"{{customer.first_name}} _ok_ snake_case_word"}}, mdStyle{}, map[string]string{"customer.first_name": "A_b_c"})
	want := `<p style="` + styleP + `">A_b_c <em>ok</em> snake_case_word</p>`
	if got != want {
		t.Fatalf("got %q", got)
	}
}
