// Package xlsx writes single-sheet Office Open XML spreadsheets (.xlsx) in
// streaming, with the standard library only (archive/zip, encoding/xml).
//
// Decision (ADR 0019 F): the exports need write-only, one table per file,
// with text, integers, two-decimal numbers and dates. A focused writer of
// ~300 lines replaces excelize (BSD-3) and its transitive dependencies
// (efp, nfp, mscfb, msoleps, x/crypto, x/net, x/text), keeps memory flat
// (rows go straight to the zip stream, no shared-string table, no DOM) and
// adds no runtime dependency. Readers: Excel, LibreOffice, Numbers, Google
// Sheets (inline strings, one styles part, ISO/IEC 29500 transitional).
//
// Cells never contain formulas: a text cell is always an inline string, so
// a value starting with "=" stays text (no spreadsheet formula injection).
package xlsx

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MediaType is the Content-Type of an .xlsx file.
const MediaType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// Limits of the format.
const (
	maxRows      = 1_048_576
	maxCols      = 16_384
	maxCellChars = 32_767
	maxSheetName = 31
)

// ErrTooManyRows is returned past the 1,048,576 rows of a sheet.
var ErrTooManyRows = errors.New("xlsx: too many rows")

// Kind is the type of a cell.
type Kind uint8

// Cell kinds.
const (
	KindEmpty Kind = iota
	KindText
	KindInt
	// KindDecimal2 is a number with two decimals stored as hundredths
	// (money in cents, percentages in basis points).
	KindDecimal2
	KindDate
)

// Cell is one value. Build it with Text, Int, Decimal2 or Date.
type Cell struct {
	Kind Kind
	S    string
	N    int64
	T    time.Time
}

// Text is a text cell.
func Text(s string) Cell { return Cell{Kind: KindText, S: s} }

// Int is an integer cell.
func Int(n int64) Cell { return Cell{Kind: KindInt, N: n} }

// Decimal2 is a number with two decimals given in hundredths (1234 -> 12,34).
func Decimal2(hundredths int64) Cell { return Cell{Kind: KindDecimal2, N: hundredths} }

// Date is a calendar date cell (shown dd/mm/yyyy).
func Date(year int, month time.Month, day int) Cell {
	return Cell{Kind: KindDate, T: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// Style ids of styles.xml (cellXfs order).
const (
	styleDefault  = 0
	styleHeader   = 1
	styleInt      = 2
	styleDecimal2 = 3
	styleDate     = 4
)

// Writer streams one sheet. Call Row for each row (the first row is styled
// as a header when Options.Header) and Close once.
type Writer struct {
	zw     *zip.Writer
	sheet  *bufio.Writer
	rows   int
	header bool
	err    error
	closed bool
}

// Options configure a Writer.
type Options struct {
	// Sheet is the sheet name (sanitised, at most 31 characters).
	Sheet string
	// Header styles the first row in bold and freezes it.
	Header bool
}

// NewWriter writes the static parts of the package to w and opens the sheet.
func NewWriter(w io.Writer, o Options) (*Writer, error) {
	zw := zip.NewWriter(w)
	x := &Writer{zw: zw, header: o.Header}
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"docProps/app.xml", appProps},
		{"xl/workbook.xml", fmt.Sprintf(workbookXML, escapeAttr(SheetName(o.Sheet)))},
		{"xl/_rels/workbook.xml.rels", workbookRels},
		{"xl/styles.xml", stylesXML},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %w", err)
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return nil, fmt.Errorf("xlsx: %w", err)
		}
	}
	f, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return nil, fmt.Errorf("xlsx: %w", err)
	}
	x.sheet = bufio.NewWriterSize(f, 32<<10)
	x.write(xml.Header)
	x.write(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	if o.Header {
		x.write(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	}
	x.write(`<sheetFormatPr defaultRowHeight="15" baseColWidth="18"/><sheetData>`)
	return x, x.err
}

func (x *Writer) write(s string) {
	if x.err == nil {
		_, x.err = x.sheet.WriteString(s)
	}
}

// Row appends a row. The first error is kept and returned by Close.
func (x *Writer) Row(cells ...Cell) error {
	if x.err != nil {
		return x.err
	}
	if x.closed {
		return errors.New("xlsx: row after close")
	}
	if x.rows >= maxRows {
		x.err = ErrTooManyRows
		return x.err
	}
	if len(cells) > maxCols {
		cells = cells[:maxCols]
	}
	x.rows++
	r := strconv.Itoa(x.rows)
	var b strings.Builder
	b.WriteString(`<row r="` + r + `">`)
	for i, c := range cells {
		ref := ColumnName(i) + r
		style := styleDefault
		if x.header && x.rows == 1 {
			style = styleHeader
		}
		switch c.Kind {
		case KindEmpty:
			continue
		case KindText:
			b.WriteString(`<c r="` + ref + `" t="inlineStr"` + styleAttr(style) + `><is><t xml:space="preserve">`)
			b.WriteString(escapeText(truncate(c.S)))
			b.WriteString(`</t></is></c>`)
		case KindInt:
			if style == styleDefault {
				style = styleInt
			}
			b.WriteString(`<c r="` + ref + `"` + styleAttr(style) + `><v>` + strconv.FormatInt(c.N, 10) + `</v></c>`)
		case KindDecimal2:
			if style == styleDefault {
				style = styleDecimal2
			}
			b.WriteString(`<c r="` + ref + `"` + styleAttr(style) + `><v>` + hundredths(c.N) + `</v></c>`)
		case KindDate:
			if style == styleDefault {
				style = styleDate
			}
			b.WriteString(`<c r="` + ref + `"` + styleAttr(style) + `><v>` + strconv.FormatInt(serial(c.T), 10) + `</v></c>`)
		}
	}
	b.WriteString(`</row>`)
	x.write(b.String())
	return x.err
}

// Close finishes the sheet and the zip container. It does not close the
// underlying writer.
func (x *Writer) Close() error {
	if x.closed {
		return x.err
	}
	x.closed = true
	x.write(`</sheetData><pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>`)
	if x.err == nil {
		x.err = x.sheet.Flush()
	}
	if err := x.zw.Close(); x.err == nil && err != nil {
		x.err = fmt.Errorf("xlsx: %w", err)
	}
	return x.err
}

func styleAttr(s int) string {
	if s == styleDefault {
		return ""
	}
	return ` s="` + strconv.Itoa(s) + `"`
}

// ColumnName is the letter name of a 0-based column (0 -> A, 26 -> AA).
func ColumnName(i int) string {
	var buf [4]byte
	n := len(buf)
	for i++; i > 0; i = (i - 1) / 26 {
		n--
		buf[n] = byte('A' + (i-1)%26)
	}
	return string(buf[n:])
}

// SheetName sanitises a sheet name: no []:*?/\ characters, no leading or
// trailing apostrophe, 1 to 31 characters ("Foglio1" when empty).
func SheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) || r < 0x20 {
			return '_'
		}
		return r
	}, strings.Trim(s, "' "))
	if utf8.RuneCountInString(s) > maxSheetName {
		s = string([]rune(s)[:maxSheetName])
	}
	if s == "" {
		return "Foglio1"
	}
	return s
}

// serial is the spreadsheet day number of t (1900 date system).
func serial(t time.Time) int64 {
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return int64(t.Sub(epoch) / (24 * time.Hour))
}

func hundredths(n int64) string {
	sign := ""
	u := uint64(n)
	if n < 0 {
		sign, u = "-", uint64(-n)
	}
	frac := u % 100
	s := sign + strconv.FormatUint(u/100, 10) + "."
	if frac < 10 {
		s += "0"
	}
	return s + strconv.FormatUint(frac, 10)
}

func truncate(s string) string {
	if utf8.RuneCountInString(s) <= maxCellChars {
		return s
	}
	return string([]rune(s)[:maxCellChars])
}

// escapeText escapes XML text; characters not allowed in XML 1.0 become
// U+FFFD (encoding/xml.EscapeText).
func escapeText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func escapeAttr(s string) string {
	return strings.ReplaceAll(escapeText(s), `"`, "&quot;")
}

const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`

const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/></Relationships>`

const appProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Arenile</Application></Properties>`

const workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="%s" sheetId="1" r:id="rId1"/></sheets></workbook>`

const workbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`

// stylesXML: cellXfs 0 default, 1 bold header, 2 integer, 3 two decimals
// with thousands separator, 4 date dd/mm/yyyy (custom format 164).
const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="1"><numFmt numFmtId="164" formatCode="dd/mm/yyyy"/></numFmts><fonts count="2"><font><sz val="11"/><name val="Calibri"/><family val="2"/></font><font><b/><sz val="11"/><name val="Calibri"/><family val="2"/></font></fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="5"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/><xf numFmtId="1" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="4" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/><xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
