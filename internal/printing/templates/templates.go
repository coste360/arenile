package templates

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/money"
)

// ErrInvalidLayout is returned for a Layout outside the supported values.
var ErrInvalidLayout = errors.New("templates: invalid layout")

// Layout is the printer side of a rendering: paper and codepage, logo,
// cut, feed and drawer.
type Layout struct {
	PaperWidthMM  int // 58 or 80
	CharsPerLine  int // font A columns (32 on 58 mm, 48 on 80 mm by default)
	Codepage      escpos.Codepage
	CodepageTable *int           // ESC t override for clones; nil = Epson default
	Logo          *escpos.Raster // cached establishment logo; nil = none
	Cut           escpos.CutMode
	FeedLines     int
	Copies        int // 1-3, each copy cut separately
	KickDrawerPin int // 0 = no kick, else 2 or 5
	Lang          string
}

// DefaultCharsPerLine is the font A width of a paper width.
func DefaultCharsPerLine(paperWidthMM int) int {
	if paperWidthMM == 58 {
		return 32
	}
	return 48
}

// Header is the establishment block printed at the top of every document.
type Header struct {
	Name      string
	Address   string // optional
	VATNumber string // optional, "P.IVA ..."
	Phone     string // optional, establishment phone (business data)
}

// Line is a priced row (label on the left, amount on the right).
type Line struct {
	Label    string
	Quantity int // printed as "2 x" when > 1
	Amount   money.Cents
}

// VATLine is one rate of the VAT breakdown.
type VATLine struct {
	RateBP  int64 // basis points, 2200 = 22%
	Taxable money.Cents
	Tax     money.Cents
}

// TicketDoc is the daily-sale ticket: establishment, ticket number and
// date, spot or zone ("da assegnare" when Spot is empty), formula, pack,
// extras, total with VAT breakdown, payment method, QR with the booking
// code. It contains no customer personal data.
type TicketDoc struct {
	Header        Header
	Number        string        // ticket number as shown to the customer
	Date          calendar.Date // service day
	IssuedAt      time.Time     // instant, printed in the establishment timezone
	Location      *time.Location
	Zone          string // zone name
	Spot          string // spot label; empty = "da assegnare"
	Formula       string // e.g. "Giornata intera", "Mezza giornata"
	Pack          string // pack name
	PackAmount    money.Cents
	Extras        []Line
	Total         money.Cents
	VAT           []VATLine
	PaymentMethod string // localized label of the method
	BookingCode   string // printed and encoded in the QR
	QRPayload     string // QR content; empty = BookingCode
	Notes         string // optional footer
}

// ReceiptDoc is the non-fiscal receipt of a payment ("Ricevuta non fiscale").
type ReceiptDoc struct {
	Header        Header
	Number        string
	IssuedAt      time.Time
	Location      *time.Location
	BookingCode   string
	Lines         []Line
	Total         money.Cents
	VAT           []VATLine
	PaymentMethod string
	Operator      string // operator display name (staff, not a customer)
}

// ZSummaryDoc is the cash closing summary printed on the receipt printer.
type ZSummaryDoc struct {
	Header       Header
	Register     string
	SessionID    string
	OpenedAt     time.Time
	ClosedAt     *time.Time // nil = draft "Non chiusa"
	Location     *time.Location
	Operators    []string
	OpeningFloat money.Cents
	ByMethod     []Line // expected total per payment method
	Movements    []Line // deposits (+) and withdrawals (-)
	Counted      []Line
	Difference   money.Cents
}

// TestPageDoc is the "Stampa di prova" page.
type TestPageDoc struct {
	Header      Header
	PrinterName string
	Host        string
	Port        int
	PrintedAt   time.Time
	Location    *time.Location
}

// Ticket renders a TicketDoc.
func Ticket(d TicketDoc, l Layout) ([]byte, error) {
	return render(l, d.Header, func(e *escpos.Encoder) {
		lang := l.Lang
		e.Align(escpos.Center).Bold(true).Size(2, 1).Text(label(lang, lblTicket)+" "+d.Number).Size(1, 1).Bold(false).Align(escpos.Left)
		e.Columns(label(lang, lblServiceDay), FormatDate(d.Date))
		if !d.IssuedAt.IsZero() {
			e.Columns(label(lang, lblIssued), FormatDateTime(d.IssuedAt, d.Location))
		}
		e.Rule('-')
		if d.Zone != "" {
			e.Columns(label(lang, lblZone), d.Zone)
		}
		spot := d.Spot
		if spot == "" {
			spot = label(lang, lblToAssign)
		}
		e.Bold(true).Columns(label(lang, lblSpot), spot).Bold(false)
		if d.Formula != "" {
			e.Columns(label(lang, lblFormula), d.Formula)
		}
		e.Rule('-')
		if d.Pack != "" {
			e.Columns(d.Pack, FormatMoney(d.PackAmount))
		}
		lines(e, d.Extras)
		e.Rule('-')
		total(e, lang, d.Total, d.VAT)
		if d.PaymentMethod != "" {
			e.Columns(label(lang, lblPayment), d.PaymentMethod)
		}
		qr := d.QRPayload
		if qr == "" {
			qr = d.BookingCode
		}
		if qr != "" {
			e.Feed(1).Align(escpos.Center).QR(qr, escpos.QROptions{Size: qrSize(l), ECC: escpos.ECCM}).Feed(1)
		}
		if d.BookingCode != "" {
			e.Align(escpos.Center).Bold(true).Line(d.BookingCode).Bold(false)
		}
		e.Align(escpos.Left)
		if d.Notes != "" {
			e.Rule('-').Text(d.Notes)
		}
		e.Align(escpos.Center).Text(label(lang, lblNonFiscal)).Align(escpos.Left)
	})
}

// Receipt renders a ReceiptDoc.
func Receipt(d ReceiptDoc, l Layout) ([]byte, error) {
	return render(l, d.Header, func(e *escpos.Encoder) {
		lang := l.Lang
		e.Align(escpos.Center).Bold(true).Text(label(lang, lblReceipt)).Bold(false).Align(escpos.Left)
		if d.Number != "" {
			e.Columns(label(lang, lblNumber), d.Number)
		}
		e.Columns(label(lang, lblDate), FormatDateTime(d.IssuedAt, d.Location))
		if d.BookingCode != "" {
			e.Columns(label(lang, lblBookingCode), d.BookingCode)
		}
		e.Rule('-')
		lines(e, d.Lines)
		e.Rule('-')
		total(e, lang, d.Total, d.VAT)
		if d.PaymentMethod != "" {
			e.Columns(label(lang, lblPayment), d.PaymentMethod)
		}
		if d.Operator != "" {
			e.Columns(label(lang, lblOperator), d.Operator)
		}
		e.Feed(1).Align(escpos.Center).Text(label(lang, lblNonFiscal)).Align(escpos.Left)
	})
}

// ZSummary renders a ZSummaryDoc.
func ZSummary(d ZSummaryDoc, l Layout) ([]byte, error) {
	return render(l, d.Header, func(e *escpos.Encoder) {
		lang := l.Lang
		e.Align(escpos.Center).Bold(true).Text(label(lang, lblZTitle))
		if d.ClosedAt == nil {
			e.Text("*** " + label(lang, lblZDraft) + " ***")
		}
		e.Bold(false).Align(escpos.Left)
		if d.Register != "" {
			e.Columns(label(lang, lblRegister), d.Register)
		}
		if d.SessionID != "" {
			e.Columns(label(lang, lblSession), shortID(d.SessionID))
		}
		e.Columns(label(lang, lblOpened), FormatDateTime(d.OpenedAt, d.Location))
		if d.ClosedAt != nil {
			e.Columns(label(lang, lblClosed), FormatDateTime(*d.ClosedAt, d.Location))
		}
		if len(d.Operators) > 0 {
			e.Text(label(lang, lblOperators) + ": " + strings.Join(d.Operators, ", "))
		}
		e.Rule('-')
		e.Columns(label(lang, lblOpeningFloat), FormatMoney(d.OpeningFloat))
		section := func(title string, ls []Line, totalLabel string) {
			e.Rule('-').Bold(true).Line(title).Bold(false)
			if len(ls) == 0 {
				e.Line("  " + label(lang, lblNone))
				return
			}
			lines(e, ls)
			if totalLabel != "" {
				var s money.Cents
				for _, x := range ls {
					s += x.Amount
				}
				e.Bold(true).Columns(totalLabel, FormatMoney(s)).Bold(false)
			}
		}
		section(label(lang, lblByMethod), d.ByMethod, label(lang, lblExpectedTotal))
		section(label(lang, lblMovements), d.Movements, "")
		if d.ClosedAt != nil {
			section(label(lang, lblCounted), d.Counted, label(lang, lblCountedTotal))
			e.Rule('=').Bold(true).Size(1, 2).Columns(label(lang, lblDifference), FormatMoney(d.Difference)).Size(1, 1).Bold(false)
		}
	})
}

// TestPage renders a TestPageDoc.
func TestPage(d TestPageDoc, l Layout) ([]byte, error) {
	// Normalize first: the page prints the effective characters per line and codepage.
	if err := validate(&l); err != nil {
		return nil, err
	}
	return render(l, d.Header, func(e *escpos.Encoder) {
		lang := l.Lang
		e.Align(escpos.Center).Bold(true).Size(2, 2).Text(label(lang, lblTestTitle)).Size(1, 1).Bold(false).Align(escpos.Left)
		e.Rule('-')
		e.Columns(label(lang, lblPrinter), d.PrinterName)
		e.Columns(label(lang, lblAddress), d.Host+":"+strconv.Itoa(d.Port))
		e.Columns(label(lang, lblPaper), fmt.Sprintf("%d mm, %d %s", l.PaperWidthMM, l.CharsPerLine, label(lang, lblChars)))
		cp := strings.ToUpper(string(l.Codepage))
		if l.CodepageTable != nil {
			cp += fmt.Sprintf(" (ESC t %d)", *l.CodepageTable)
		}
		e.Columns(label(lang, lblCodepage), cp)
		if !d.PrintedAt.IsZero() {
			e.Columns(label(lang, lblDate), FormatDateTime(d.PrintedAt, d.Location))
		}
		e.Rule('-')
		e.Line(label(lang, lblCharset) + ":")
		e.Text("àèéìòù ÀÈÉÌÒÙ ç ñ ü ß € 1.234,50")
		e.Rule('-')
		e.Line(label(lang, lblStyles) + ":")
		e.Bold(true).Line(label(lang, lblBold)).Bold(false)
		e.Size(2, 2).Line(label(lang, lblDouble)).Size(1, 1)
		e.Align(escpos.Center).Line("<- center ->").Align(escpos.Right).Line("right").Align(escpos.Left)
		e.Rule('-')
		e.Line(label(lang, lblColumns) + ":")
		e.Columns(label(lang, lblSample), FormatMoney(2500))
		e.Columns("2 x Lettino", FormatMoney(1000))
		e.Feed(1).Align(escpos.Center).QR("ARENILE-TEST", escpos.QROptions{Size: qrSize(l), ECC: escpos.ECCM}).Feed(1)
		e.Text(label(lang, lblTestOK)).Align(escpos.Left)
	})
}

// render validates the layout and writes Copies copies of: init, logo,
// header, body, mono mark, feed, cut. The drawer kicks once, after the
// first copy.
func render(l Layout, h Header, body func(e *escpos.Encoder)) ([]byte, error) {
	if err := validate(&l); err != nil {
		return nil, err
	}
	e := escpos.New(escpos.Config{CharsPerLine: l.CharsPerLine, Codepage: l.Codepage, CodepageTable: l.CodepageTable})
	for i := range l.Copies {
		e.Init()
		if l.Logo != nil && len(l.Logo.Data) > 0 {
			e.Align(escpos.Center).Raster(*l.Logo).Align(escpos.Left)
		}
		header(e, l.Lang, h)
		body(e)
		e.Feed(1).Align(escpos.Center).FontB(true).Line("Arenile").FontB(false).Align(escpos.Left)
		e.Feed(l.FeedLines).Cut(l.Cut)
		if i == 0 {
			e.KickDrawer(l.KickDrawerPin)
		}
	}
	return e.Bytes(), nil
}

func validate(l *Layout) error {
	if l.PaperWidthMM != 58 && l.PaperWidthMM != 80 {
		return fmt.Errorf("%w: paper width %d", ErrInvalidLayout, l.PaperWidthMM)
	}
	if l.CharsPerLine == 0 {
		l.CharsPerLine = DefaultCharsPerLine(l.PaperWidthMM)
	}
	if l.CharsPerLine < 24 || l.CharsPerLine > 64 {
		return fmt.Errorf("%w: chars per line %d", ErrInvalidLayout, l.CharsPerLine)
	}
	if l.Codepage == "" {
		l.Codepage = escpos.CP858
	}
	if !l.Codepage.Valid() {
		return fmt.Errorf("%w: codepage %q", ErrInvalidLayout, l.Codepage)
	}
	switch l.Cut {
	case "":
		l.Cut = escpos.CutPartial
	case escpos.CutFull, escpos.CutPartial, escpos.CutNone:
	default:
		return fmt.Errorf("%w: cut %q", ErrInvalidLayout, l.Cut)
	}
	if l.FeedLines < 0 || l.FeedLines > 10 {
		return fmt.Errorf("%w: feed lines %d", ErrInvalidLayout, l.FeedLines)
	}
	if l.Copies == 0 {
		l.Copies = 1
	}
	if l.Copies < 1 || l.Copies > 3 {
		return fmt.Errorf("%w: copies %d", ErrInvalidLayout, l.Copies)
	}
	if l.KickDrawerPin != 0 && l.KickDrawerPin != 2 && l.KickDrawerPin != 5 {
		return fmt.Errorf("%w: drawer pin %d", ErrInvalidLayout, l.KickDrawerPin)
	}
	return nil
}

func header(e *escpos.Encoder, lang string, h Header) {
	e.Align(escpos.Center)
	if h.Name != "" {
		e.Bold(true).Size(2, 2).Text(h.Name).Size(1, 1).Bold(false)
	}
	if h.Address != "" {
		e.Text(h.Address)
	}
	if h.VATNumber != "" {
		e.Text(label(lang, lblVATNumber) + " " + h.VATNumber)
	}
	if h.Phone != "" {
		e.Text(label(lang, lblPhone) + " " + h.Phone)
	}
	e.Align(escpos.Left).Rule('=')
}

func lines(e *escpos.Encoder, ls []Line) {
	for _, x := range ls {
		lbl := x.Label
		if x.Quantity > 1 {
			lbl = strconv.Itoa(x.Quantity) + " x " + lbl
		}
		e.Columns(lbl, FormatMoney(x.Amount))
	}
}

func total(e *escpos.Encoder, lang string, t money.Cents, vat []VATLine) {
	e.Bold(true).Size(1, 2).Columns(label(lang, lblTotal), FormatMoney(t)).Size(1, 1).Bold(false)
	for _, v := range vat {
		e.Columns(fmt.Sprintf("%s %s %s %s", label(lang, lblVAT), FormatRate(v.RateBP), label(lang, lblVATOn), FormatMoney(v.Taxable)), FormatMoney(v.Tax))
	}
}

func qrSize(l Layout) int {
	if l.PaperWidthMM == 58 {
		return 5
	}
	return 6
}

// FormatMoney formats cents as "€ 1.234,50" (negative "€ -2,00").
func FormatMoney(c money.Cents) string {
	v := int64(c)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	euros := strconv.FormatInt(v/100, 10)
	var b strings.Builder
	for i, r := range euros {
		if i > 0 && (len(euros)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("€ %s%s,%02d", sign, b.String(), v%100)
}

// FormatRate formats basis points as a percentage: 2200 → "22%", 550 → "5,5%".
func FormatRate(bp int64) string {
	s := strconv.FormatInt(bp/100, 10)
	if r := bp % 100; r != 0 {
		s += "," + strings.TrimRight(fmt.Sprintf("%02d", r), "0")
	}
	return s + "%"
}

// FormatDate formats a calendar date as dd/MM/yyyy.
func FormatDate(d calendar.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.Time().Format("02/01/2006")
}

// FormatDateTime formats an instant in loc (UTC when nil) as dd/MM/yyyy HH:mm.
func FormatDateTime(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("02/01/2006 15:04")
}

// shortID is the last 8 hex digits of a UUID, upper case.
func shortID(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	return strings.ToUpper(id)
}

// ShortID is shortID for callers composing document numbers.
func ShortID(id string) string { return shortID(id) }
