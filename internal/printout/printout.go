// Package printout renders print-ready documents (ADR 0019): today the Z
// closing summary of a cash session (spec 14.1) as a self-contained HTML
// page with print styles for A4 and 80 mm receipt paper. The PDF is the
// browser's print ("Save as PDF"): no PDF library, no new runtime
// dependency, selectable and accessible text, the same totals the panel
// shows because they come from the cash service.
//
// The documents carry the establishment header and the monochrome navy
// Arenile logo (ADR 0018 brand rules: prints use the mono mark), embedded
// as a data URI so the page needs no network access. The logo file is a
// copy of assets/brand/arenile/outlined/arenile-logo-horizontal-navy.svg
// (TestBrandLogoMatchesBrandKit keeps them identical).
//
// Rendering is a pure function of its input (no clock, no I/O): the caller
// passes the instants already resolved and the timezone.
package printout

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/money"
)

//go:embed brand/arenile-logo-horizontal-navy.svg
var logoSVG []byte

//go:embed i18n/*.json
var catalogFS embed.FS

//go:embed zreport.html.tmpl
var zreportTmpl string

// Locales are the built-in locales; Italian is the source.
var Locales = []string{"it", "en", "de", "fr"}

// SourceLocale is the fallback locale.
const SourceLocale = "it"

// ContentSecurityPolicy is the CSP of the printed documents: inline styles
// and data: images only, no script, frameable by the panel (print through a
// hidden iframe).
const ContentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// Paper formats.
const (
	PaperA4        = "a4"
	PaperReceipt80 = "receipt80"
)

var (
	loadOnce sync.Once
	catalogs map[string]map[string]string
	tmpl     *template.Template
	logoURI  template.URL
	loadErr  error
)

func load() error {
	loadOnce.Do(func() {
		catalogs = map[string]map[string]string{}
		for _, loc := range Locales {
			raw, err := catalogFS.ReadFile("i18n/" + loc + ".json")
			if err != nil {
				loadErr = fmt.Errorf("printout: catalog %s: %w", loc, err)
				return
			}
			m := map[string]string{}
			if err := json.Unmarshal(raw, &m); err != nil {
				loadErr = fmt.Errorf("printout: catalog %s: %w", loc, err)
				return
			}
			catalogs[loc] = m
		}
		// The logo is a trusted, embedded brand asset.
		logoURI = template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoSVG)) //nolint:gosec // embedded asset
		tmpl, loadErr = template.New("zreport").Parse(zreportTmpl)
	})
	return loadErr
}

// NormalizeLocale returns a built-in locale for an IETF tag ("de-CH" ->
// "de"), SourceLocale when unknown.
func NormalizeLocale(tag string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	base, _, _ = strings.Cut(base, "_")
	for _, l := range Locales {
		if l == base {
			return l
		}
	}
	return SourceLocale
}

// Header is the establishment header of a printed document.
type Header struct {
	Name          string
	AddressLines  []string
	LegalName     string
	VATNumber     string
	FiscalCode    string
	ConcessionRef string
}

// ZMethodRow is one payment method of the Z summary.
type ZMethodRow struct {
	Method     string
	Takings    money.Cents
	Expected   money.Cents
	Counted    money.Cents
	Difference money.Cents
}

// ZVATRow is one VAT rate of the takings.
type ZVATRow struct {
	RateBP  int64
	Taxable money.Cents
	Tax     money.Cents
	Gross   money.Cents
}

// ZMovement is a cash deposit (in) or withdrawal (out).
type ZMovement struct {
	At     time.Time
	Kind   string
	Amount money.Cents
	Reason string
}

// ZReport is the input of RenderZReport. Amounts are cents; instants are
// shown in Location.
type ZReport struct {
	Locale       string
	Paper        string
	Location     *time.Location
	Currency     string
	Header       Header
	SessionID    string
	RegisterName string
	RegisterCode string
	Closed       bool
	OpenedAt     time.Time
	ClosedAt     *time.Time
	OpenedBy     string
	ClosedBy     string
	GeneratedAt  time.Time
	OpeningFloat money.Cents
	MovementsIn  money.Cents
	MovementsOut money.Cents
	Payments     int
	Refunds      int
	Methods      []ZMethodRow
	VAT          []ZVATRow
	Movements    []ZMovement
	Difference   money.Cents
	Notes        string
}

// methodOrder is the print order of the payment methods.
var methodOrder = map[string]int{"cash": 0, "card": 1, "bank_transfer": 2, "wallet": 3, "credit": 4, "other": 5}

// SortMethods orders rows cash, card, bank transfer, wallet, credit,
// other, then unknown methods alphabetically.
func SortMethods(rows []ZMethodRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		oi, ok := methodOrder[rows[i].Method]
		if !ok {
			oi = len(methodOrder)
		}
		oj, ok := methodOrder[rows[j].Method]
		if !ok {
			oj = len(methodOrder)
		}
		if oi != oj {
			return oi < oj
		}
		return rows[i].Method < rows[j].Method
	})
}

type zView struct {
	Lang, Paper, Title           string
	Logo                         template.URL
	Header                       Header
	Register                     string
	Closed                       bool
	OpenedAt, ClosedAt           string
	OpenedBy, ClosedBy           string
	GeneratedAt, SessionID       string
	Float, MovIn, MovOut         string
	Payments, Refunds            int
	Methods                      []zMethodView
	Totals                       zMethodView
	VAT                          []zVATView
	VATTotal                     zVATView
	Movements                    []zMovementView
	Difference                   string
	DifferenceTone               string
	Notes                        string
	T                            func(string) string
	Receipt                      bool
	PageCSS                      template.CSS
	HasVAT, HasMovements, HasPay bool
}

type zMethodView struct {
	Label, Takings, Expected, Counted, Difference, Tone string
}

type zVATView struct {
	Rate, Taxable, Tax, Gross string
}

type zMovementView struct {
	Time, Kind, Amount, Reason string
}

// RenderZReport writes the Z summary as a complete HTML document.
func RenderZReport(w io.Writer, z ZReport) error {
	if err := load(); err != nil {
		return err
	}
	loc := NormalizeLocale(z.Locale)
	cat := catalogs[loc]
	t := func(k string) string {
		if v, ok := cat[k]; ok {
			return v
		}
		if v, ok := catalogs[SourceLocale][k]; ok {
			return v
		}
		return k
	}
	tz := z.Location
	if tz == nil {
		tz = time.UTC
	}
	m := func(c money.Cents) string { return FormatMoney(c, z.Currency, loc) }
	dt := func(ts time.Time) string { return FormatDateTime(ts.In(tz), loc) }
	paper := z.Paper
	if paper != PaperReceipt80 {
		paper = PaperA4
	}
	v := zView{
		Lang: loc, Paper: paper, Title: t("z.title"), Logo: logoURI, Header: z.Header, Closed: z.Closed,
		OpenedAt: dt(z.OpenedAt), OpenedBy: z.OpenedBy, ClosedBy: z.ClosedBy, GeneratedAt: dt(z.GeneratedAt),
		SessionID: z.SessionID, Float: m(z.OpeningFloat), MovIn: m(z.MovementsIn), MovOut: m(z.MovementsOut),
		Payments: z.Payments, Refunds: z.Refunds, Notes: z.Notes, T: t, Receipt: paper == PaperReceipt80,
		Difference: m(z.Difference), DifferenceTone: tone(z.Difference),
	}
	v.Register = z.RegisterName
	if v.Register == "" {
		v.Register = z.RegisterCode
	}
	if z.ClosedAt != nil {
		v.ClosedAt = dt(*z.ClosedAt)
	}
	if v.Receipt {
		v.PageCSS = "@page { size: 80mm auto; margin: 3mm; }"
	} else {
		v.PageCSS = "@page { size: A4; margin: 14mm 14mm 16mm; }"
	}
	rows := append([]ZMethodRow(nil), z.Methods...)
	SortMethods(rows)
	var tot ZMethodRow
	for _, r := range rows {
		tot.Takings += r.Takings
		tot.Expected += r.Expected
		tot.Counted += r.Counted
		tot.Difference += r.Difference
		v.Methods = append(v.Methods, zMethodView{
			Label: t("method." + r.Method), Takings: m(r.Takings), Expected: m(r.Expected),
			Counted: m(r.Counted), Difference: m(r.Difference), Tone: tone(r.Difference),
		})
	}
	v.Totals = zMethodView{
		Label: t("z.total"), Takings: m(tot.Takings), Expected: m(tot.Expected), Counted: m(tot.Counted),
		Difference: m(z.Difference), Tone: tone(z.Difference),
	}
	var vt ZVATRow
	for _, r := range z.VAT {
		vt.Taxable += r.Taxable
		vt.Tax += r.Tax
		vt.Gross += r.Gross
		v.VAT = append(v.VAT, zVATView{Rate: FormatRate(r.RateBP, loc), Taxable: m(r.Taxable), Tax: m(r.Tax), Gross: m(r.Gross)})
	}
	v.VATTotal = zVATView{Rate: t("z.total"), Taxable: m(vt.Taxable), Tax: m(vt.Tax), Gross: m(vt.Gross)}
	for _, mv := range z.Movements {
		v.Movements = append(v.Movements, zMovementView{
			Time: FormatTime(mv.At.In(tz)), Kind: t("z.kind." + mv.Kind), Amount: m(mv.Amount), Reason: mv.Reason,
		})
	}
	v.HasVAT, v.HasMovements, v.HasPay = len(v.VAT) > 0, len(v.Movements) > 0, len(v.Methods) > 0
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		return fmt.Errorf("printout: render z report: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func tone(c money.Cents) string {
	switch {
	case c < 0:
		return "neg"
	case c > 0:
		return "pos"
	}
	return ""
}

// FormatMoney formats cents for locale: it/de/fr "1.234,50 €" (fr groups
// with a space), en "€1,234.50".
func FormatMoney(c money.Cents, currency, locale string) string {
	neg := c < 0
	if neg {
		c = -c
	}
	units := strconv.FormatInt(int64(c)/100, 10)
	cents := int64(c) % 100
	group, decimal := ".", ","
	switch locale {
	case "en":
		group, decimal = ",", "."
	case "fr":
		group = " "
	}
	var b strings.Builder
	for i, r := range units {
		if i > 0 && (len(units)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(r)
	}
	num := b.String() + decimal + fmt.Sprintf("%02d", cents)
	sym := currencySymbol(currency)
	sign := ""
	if neg {
		sign = "−"
	}
	if locale == "en" {
		return sign + sym + num
	}
	return sign + num + " " + sym
}

func currencySymbol(code string) string {
	switch strings.ToUpper(code) {
	case "", "EUR":
		return "€"
	case "GBP":
		return "£"
	case "USD":
		return "$"
	}
	return strings.ToUpper(code)
}

// FormatDateTime formats an instant already in the establishment timezone:
// "15/07/2027 08:30" (it, en, fr), "15.07.2027 08:30" (de).
func FormatDateTime(t time.Time, locale string) string {
	if locale == "de" {
		return t.Format("02.01.2006 15:04")
	}
	return t.Format("02/01/2006 15:04")
}

// FormatTime formats the time of an instant already in the establishment
// timezone ("08:30").
func FormatTime(t time.Time) string { return t.Format("15:04") }

// FormatRate formats basis points as a percentage ("22%", "5,5%"; en "5.5%").
func FormatRate(bp int64, locale string) string {
	s := strconv.FormatInt(bp/100, 10)
	if frac := bp % 100; frac != 0 {
		f := strings.TrimRight(fmt.Sprintf("%02d", frac), "0")
		sep := ","
		if locale == "en" {
			sep = "."
		}
		s += sep + f
	}
	return s + "%"
}
