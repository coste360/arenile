package printout

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"sync"
)

// Printable booking summary for the customer (spec 30.9): establishment,
// summary rows, totals and the booking QR, A4 or 80 mm receipt paper,
// self-contained (no script, data: images only). The texts arrive already
// formatted in the customer's language (internal/domain/notify).

//go:embed booking_summary.html.tmpl
var bookingSummaryTmplSrc string

var (
	bsOnce sync.Once
	bsTmpl *template.Template
	bsErr  error
)

// SummaryRow is a label/value line.
type SummaryRow struct{ Label, Value string }

// BookingSummary is the input of RenderBookingSummary.
type BookingSummary struct {
	Locale, Paper              string
	Establishment, Title, Hint string
	BookingCode, QRNote        string
	QRSVG                      []byte
	Summary, Totals            []SummaryRow
	Contacts                   []string
}

type bsView struct {
	BookingSummary
	Lang    string
	Logo    template.URL
	QR      template.URL
	Receipt bool
	PageCSS template.CSS
}

// RenderBookingSummary writes the summary as a complete HTML document.
// Pure: no clock, no I/O besides w.
func RenderBookingSummary(w io.Writer, in BookingSummary) error {
	if err := load(); err != nil { // logo
		return err
	}
	bsOnce.Do(func() { bsTmpl, bsErr = template.New("booking_summary").Parse(bookingSummaryTmplSrc) })
	if bsErr != nil {
		return bsErr
	}
	v := bsView{BookingSummary: in, Lang: NormalizeLocale(in.Locale), Logo: logoURI, Receipt: in.Paper == PaperReceipt80}
	if len(in.QRSVG) > 0 {
		v.QR = template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(in.QRSVG)) //nolint:gosec // generated QR
	}
	if v.Receipt {
		v.PageCSS = "@page { size: 80mm auto; margin: 3mm; }"
	} else {
		v.PageCSS = "@page { size: A4; margin: 14mm 14mm 16mm; }"
	}
	var buf bytes.Buffer
	if err := bsTmpl.Execute(&buf, v); err != nil {
		return fmt.Errorf("printout: render booking summary: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}
