package templates

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/money"
)

var update = flag.Bool("update", false, "rewrite the golden fixtures in testdata")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".bin")
	if *update {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		// Human-readable twin for review.
		if err := os.WriteFile(filepath.Join("testdata", name+".txt"), []byte(escpos.Dump(got)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // G304: fixed test fixture path
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from golden\n got:\n%s\nwant:\n%s", name, escpos.Dump(got), escpos.Dump(want))
	}
}

var (
	rome   = mustLoc("Europe/Rome")
	issued = time.Date(2026, 8, 14, 7, 35, 0, 0, time.UTC)
	hdr    = Header{Name: "Lido Azzurro", Address: "Lungomare Caio Duilio 12, Gaeta (LT)", VATNumber: "01234567890", Phone: "0771 123456"}
)

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

func ticketDoc() TicketDoc {
	return TicketDoc{
		Header: hdr, Number: "0042", Date: calendar.New(2026, 8, 14), IssuedAt: issued, Location: rome,
		Zone: "Prima fila", Spot: "A12", Formula: "Giornata intera", Pack: "Ombrellone + 2 lettini", PackAmount: 2500,
		Extras:        []Line{{Label: "Lettino extra", Quantity: 2, Amount: 1000}, {Label: "Cassaforte", Quantity: 1, Amount: 300}},
		Total:         3800,
		VAT:           []VATLine{{RateBP: 2200, Taxable: 3115, Tax: 685}},
		PaymentMethod: "Contanti", BookingCode: "ARN-7F3K",
	}
}

func layout(w int) Layout {
	return Layout{PaperWidthMM: w, Codepage: escpos.CP858, Cut: escpos.CutPartial, FeedLines: 3, Copies: 1, Lang: "it"}
}

func TestTemplates_Golden(t *testing.T) {
	closed := issued.Add(10 * time.Hour)
	logo := escpos.Raster{WidthBytes: 2, Height: 2, Data: []byte{0xf0, 0x0f, 0x0f, 0xf0}}
	for _, w := range []int{58, 80} {
		ws := map[int]string{58: "58", 80: "80"}[w]
		tests := []struct {
			name   string
			render func() ([]byte, error)
		}{
			{"ticket", func() ([]byte, error) { return Ticket(ticketDoc(), layout(w)) }},
			{"ticket_unassigned_logo_kick_en", func() ([]byte, error) {
				d := ticketDoc()
				d.Spot, d.QRPayload, d.Notes = "", "https://lido.example/b/ARN-7F3K", "Grazie e buona giornata!"
				d.Holder = "Rossi"
				l := layout(w)
				l.Logo, l.KickDrawerPin, l.Lang, l.Cut = &logo, 2, "en", escpos.CutFull
				return Ticket(d, l)
			}},
			{"ticket_entry_reprint", func() ([]byte, error) {
				d := ticketDoc()
				d.Number, d.Entry, d.Reprint, d.Voided = "#037", true, true, true
				d.Zone, d.Spot, d.Formula, d.Pack, d.PackAmount = "Spiaggia libera", "", "Giornata intera", "", 0
				d.Extras = []Line{{Label: "Adulto", Quantity: 2, Amount: 2000}, {Label: "Bambino", Quantity: 1, Amount: 500}}
				d.Total, d.VAT, d.QRPayload = 2500, []VATLine{{RateBP: 1000, Taxable: 2273, Tax: 227}}, "gt_4XQ9ZK2M7P"
				return Ticket(d, layout(w))
			}},
			{"receipt", func() ([]byte, error) {
				return Receipt(ReceiptDoc{
					Header: hdr, Number: "7F3K0042", IssuedAt: issued, Location: rome, BookingCode: "ARN-7F3K",
					Lines: []Line{{Label: "Acconto prenotazione ARN-7F3K", Amount: 15000}}, Total: 15000,
					VAT: []VATLine{{RateBP: 2200, Taxable: 12295, Tax: 2705}}, PaymentMethod: "Carta", Operator: "Anna",
				}, layout(w))
			}},
			{"z_closed", func() ([]byte, error) {
				return ZSummary(ZSummaryDoc{
					Header: hdr, Register: "Cassa bar", SessionID: "0192f1a2-7b3c-7def-8123-456789abcdef",
					OpenedAt: issued, ClosedAt: &closed, Location: rome, Operators: []string{"Anna", "Marco"},
					OpeningFloat: 10000,
					ByMethod:     []Line{{Label: "Contanti", Amount: 45000}, {Label: "Carta", Amount: 123450}},
					Movements:    []Line{{Label: "Prelievo per banca", Amount: -20000}},
					Counted:      []Line{{Label: "Contanti", Amount: 44800}, {Label: "Carta", Amount: 123450}},
					Difference:   -200,
				}, layout(w))
			}},
			{"receipt_copies", func() ([]byte, error) {
				l := layout(w)
				l.Copies, l.KickDrawerPin, l.Lang = 2, 2, "en"
				return Receipt(ReceiptDoc{
					Header: hdr, Number: "7F3K0042", IssuedAt: issued, Location: rome,
					Lines: []Line{{Label: "Deposit ARN-7F3K", Amount: 15000}}, Total: 15000, PaymentMethod: "Cash",
				}, l)
			}},
			{"z_draft", func() ([]byte, error) {
				return ZSummary(ZSummaryDoc{Header: hdr, Register: "Cassa principale", OpenedAt: issued, Location: rome, OpeningFloat: 5000}, layout(w))
			}},
			{"test_page", func() ([]byte, error) {
				return TestPage(TestPageDoc{Header: hdr, PrinterName: "Banco bar", Host: "192.168.1.50", Port: 9100, PrintedAt: issued, Location: rome}, layout(w))
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name+"_"+ws, func(t *testing.T) {
				b, err := tt.render()
				if err != nil {
					t.Fatal(err)
				}
				golden(t, tt.name+"_"+ws, b)
				assertWidth(t, b, w)
			})
		}
	}
}

// assertWidth checks that no plain text line (font A, size 1) is longer than
// the paper allows: commands are stripped before measuring.
func assertWidth(t *testing.T, b []byte, w int) {
	t.Helper()
	limit := DefaultCharsPerLine(w)
	for _, line := range strings.Split(string(stripCommands(b)), "\n") {
		if len(line) > limit {
			t.Fatalf("line longer than %d: %q", limit, line)
		}
	}
}

// stripCommands removes the ESC/POS commands emitted by the templates.
func stripCommands(b []byte) []byte {
	var out []byte
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case escpos.ESC:
			switch b[i+1] {
			case '@':
				i++
			case 'p':
				i += 4
			default:
				i += 2
			}
		case escpos.GS:
			switch b[i+1] {
			case '(':
				n := int(b[i+3]) | int(b[i+4])<<8
				i += 4 + n
			case 'v':
				wb := int(b[i+4]) | int(b[i+5])<<8
				h := int(b[i+6]) | int(b[i+7])<<8
				i += 7 + wb*h
			default:
				i += 2
			}
		default:
			out = append(out, b[i])
		}
	}
	return out
}

func TestTemplates_Content(t *testing.T) {
	b, err := Ticket(ticketDoc(), layout(80))
	if err != nil {
		t.Fatal(err)
	}
	text := string(stripCommands(b))
	for _, want := range []string{"BIGLIETTO 0042", "14/08/2026", "14/08/2026 09:35", "A12", "Giornata intera", "2 x Lettino extra", "TOTALE", "IVA 22% su \xd5 31,15", "ARN-7F3K", "Arenile"} {
		if !strings.Contains(text, want) {
			t.Errorf("ticket misses %q:\n%s", want, text)
		}
	}
	if !bytes.Contains(b, []byte{escpos.GS, '(', 'k', 11, 0, 49, 80, 48, 'A', 'R', 'N'}) {
		t.Error("QR store command for the booking code not found")
	}
	if !bytes.HasSuffix(b, []byte{escpos.ESC, 'd', 3, escpos.GS, 'V', 1}) {
		t.Errorf("must end with feed and partial cut: %s", escpos.Dump(b[len(b)-10:]))
	}
	d := ticketDoc()
	d.Spot = ""
	b, _ = Ticket(d, layout(58))
	if !strings.Contains(string(stripCommands(b)), "da assegnare") {
		t.Error("empty spot must print da assegnare")
	}
}

func TestTemplates_CopiesAndDrawer(t *testing.T) {
	l := layout(80)
	l.Copies, l.KickDrawerPin = 3, 5
	b, err := Receipt(ReceiptDoc{Header: hdr, IssuedAt: issued, Total: 100}, l)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(b, []byte{escpos.GS, 'V', 1}); n != 3 {
		t.Fatalf("cuts = %d, want 3", n)
	}
	if n := bytes.Count(b, []byte{escpos.ESC, 'p', 1, 25, 250}); n != 1 {
		t.Fatalf("drawer kicks = %d, want 1", n)
	}
	if n := bytes.Count(b, []byte{escpos.ESC, '@'}); n != 3 {
		t.Fatalf("inits = %d, want 3", n)
	}
}

func TestTemplates_InvalidLayout(t *testing.T) {
	bad := []Layout{
		{PaperWidthMM: 60},
		{PaperWidthMM: 80, CharsPerLine: 10},
		{PaperWidthMM: 80, Codepage: "utf8"},
		{PaperWidthMM: 80, Cut: "half"},
		{PaperWidthMM: 80, FeedLines: 11},
		{PaperWidthMM: 80, Copies: 4},
		{PaperWidthMM: 80, KickDrawerPin: 3},
	}
	for _, l := range bad {
		if _, err := TestPage(TestPageDoc{}, l); !errors.Is(err, ErrInvalidLayout) {
			t.Errorf("%+v: err = %v", l, err)
		}
	}
	// Zero values get defaults.
	if _, err := TestPage(TestPageDoc{}, Layout{PaperWidthMM: 58}); err != nil {
		t.Fatal(err)
	}
}

func TestFormat_Scenarios(t *testing.T) {
	money := []struct {
		c    int64
		want string
	}{{0, "€ 0,00"}, {5, "€ 0,05"}, {123450, "€ 1.234,50"}, {-200, "€ -2,00"}, {100000000, "€ 1.000.000,00"}}
	for _, m := range money {
		if got := FormatMoney(moneyCents(m.c)); got != m.want {
			t.Errorf("FormatMoney(%d) = %q, want %q", m.c, got, m.want)
		}
	}
	rates := map[int64]string{2200: "22%", 1000: "10%", 550: "5,5%", 0: "0%", 425: "4,25%"}
	for bp, want := range rates {
		if got := FormatRate(bp); got != want {
			t.Errorf("FormatRate(%d) = %q, want %q", bp, got, want)
		}
	}
	if got := MethodLabel("de-DE", "cash"); got != "Bar" {
		t.Errorf("MethodLabel de = %q", got)
	}
	if got := MethodLabel("xx", "voucher"); got != "voucher" {
		t.Errorf("unknown method = %q", got)
	}
	for lang := range labels {
		for key := range labels["it"] {
			if labels[lang][key] == "" {
				t.Errorf("label %s missing in %s", key, lang)
			}
		}
	}
}

func moneyCents(v int64) money.Cents { return money.Cents(v) }
