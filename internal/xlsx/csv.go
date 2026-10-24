package xlsx

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// FromCSV converts an Arenile CSV export (UTF-8 BOM, ';' separator, Italian
// number and date formats, see report.CSVWriter) into an .xlsx sheet, row by
// row. The first row is the header. Converting the CSV keeps the two
// formats identical by construction: same columns, same rows, no data that
// the CSV does not already have.
func FromCSV(dst io.Writer, src io.Reader, sheet string) error {
	br := bufio.NewReader(src)
	if b, err := br.Peek(3); err == nil && string(b) == "\xEF\xBB\xBF" {
		_, _ = br.Discard(3)
	}
	r := csv.NewReader(br)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	w, err := NewWriter(dst, Options{Sheet: sheet, Header: true})
	if err != nil {
		return err
	}
	first := true
	cells := make([]Cell, 0, 16)
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xlsx: read csv: %w", err)
		}
		cells = cells[:0]
		for _, f := range rec {
			if first {
				cells = append(cells, Text(f))
				continue
			}
			cells = append(cells, ParseCell(f))
		}
		first = false
		if err := w.Row(cells...); err != nil {
			return err
		}
	}
	return w.Close()
}

// ParseCell types one CSV field of the Italian export format:
//
//	""             -> empty
//	"'=..."        -> text without the anti-formula apostrophe (=, +, -, @, tab, CR)
//	"dd/mm/yyyy"   -> date
//	"-1234"        -> integer (no leading zero: codes like "00123" stay text)
//	"-1234,56"     -> two-decimal number (money, percentages)
//	anything else  -> text
func ParseCell(s string) Cell {
	if s == "" {
		return Cell{}
	}
	if len(s) >= 2 && s[0] == '\'' && strings.ContainsRune("=+-@\t\r", rune(s[1])) {
		return Text(s[1:])
	}
	if c, ok := parseDate(s); ok {
		return c
	}
	digits := strings.TrimPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(digits, ",")
	if !allDigits(intPart) || len(intPart) > 15 || (len(intPart) > 1 && intPart[0] == '0') {
		return Text(s)
	}
	if !hasFrac {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return Text(s)
		}
		return Int(n)
	}
	if len(frac) != 2 || !allDigits(frac) {
		return Text(s)
	}
	n, err := strconv.ParseInt(intPart+frac, 10, 64)
	if err != nil {
		return Text(s)
	}
	if strings.HasPrefix(s, "-") {
		n = -n
	}
	return Decimal2(n)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseDate reads dd/mm/yyyy (a real calendar day).
func parseDate(s string) (Cell, bool) {
	if len(s) != 10 || s[2] != '/' || s[5] != '/' {
		return Cell{}, false
	}
	t, err := time.Parse("02/01/2006", s)
	if err != nil {
		return Cell{}, false
	}
	return Date(t.Year(), t.Month(), t.Day()), true
}
