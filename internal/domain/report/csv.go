package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/money"
)

// CSVWriter writes report exports in the shared format: UTF-8 BOM, ';'
// separator, CRLF line endings, quoted fields only when needed. Use
// FormatEuro for money and FormatBP for ratios so that numbers use the
// decimal comma.
type CSVWriter struct {
	w   *csv.Writer
	err error
}

// NewCSVWriter writes the BOM and returns a writer.
func NewCSVWriter(w io.Writer) *CSVWriter {
	cw := &CSVWriter{}
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil {
		cw.err = fmt.Errorf("report: write csv: %w", err)
	}
	cw.w = csv.NewWriter(w)
	cw.w.Comma = ';'
	cw.w.UseCRLF = true
	return cw
}

// Row writes one record; the first error is kept and returned by Flush.
func (c *CSVWriter) Row(fields ...string) {
	if c.err != nil {
		return
	}
	if err := c.w.Write(fields); err != nil {
		c.err = fmt.Errorf("report: write csv: %w", err)
	}
}

// Flush flushes the buffer and returns the first error.
func (c *CSVWriter) Flush() error {
	if c.err != nil {
		return c.err
	}
	c.w.Flush()
	if err := c.w.Error(); err != nil {
		return fmt.Errorf("report: write csv: %w", err)
	}
	return nil
}

// FormatEuro formats cents with a decimal comma ("1234,56").
func FormatEuro(c money.Cents) string { return fiscal.FormatEuro(c) }

// FormatBP formats basis points as a percentage with two decimals and a
// decimal comma (1234 -> "12,34").
func FormatBP(bp int64) string {
	sign := ""
	if bp < 0 {
		sign, bp = "-", -bp
	}
	frac := strconv.FormatInt(bp%100, 10)
	if len(frac) == 1 {
		frac = "0" + frac
	}
	return sign + strconv.FormatInt(bp/100, 10) + "," + frac
}

// FormatCount formats an integer count.
func FormatCount[T ~int | ~int64](n T) string { return strconv.FormatInt(int64(n), 10) }
