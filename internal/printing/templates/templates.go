package templates

import (
	"errors"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/money"
)

// errNotImplemented is returned by the renderers until the templates builder
// lands them.
var errNotImplemented = errors.New("templates: not implemented")

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
func Ticket(_ TicketDoc, _ Layout) ([]byte, error) { return nil, errNotImplemented }

// Receipt renders a ReceiptDoc.
func Receipt(_ ReceiptDoc, _ Layout) ([]byte, error) { return nil, errNotImplemented }

// ZSummary renders a ZSummaryDoc.
func ZSummary(_ ZSummaryDoc, _ Layout) ([]byte, error) { return nil, errNotImplemented }

// TestPage renders a TestPageDoc.
func TestPage(_ TestPageDoc, _ Layout) ([]byte, error) { return nil, errNotImplemented }
