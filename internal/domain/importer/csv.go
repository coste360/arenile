package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/coste360/arenile/internal/domain/customer"
)

// Pure CSV decoding (functional core): no I/O, no clock.

// Limits of an uploaded file (api createImportJob). Preview and apply run
// the whole file in one transaction inside the request (60 s write
// timeout, single writer on SQLite): measured on SQLite a customer row
// costs about 1.6 ms and a season pass row about 4.5 ms (allocations,
// pricing, audit), so the limits keep a full file well under 10 s.
const (
	MaxFileBytes = 5 << 20
	// MaxRows bounds a customer file.
	MaxRows = 2000
	// MaxPassRows bounds a season pass file.
	MaxPassRows = 500
	// maxIssues caps the errors and the warnings of a report (api ImportReport).
	maxIssues = 500
)

// table is a decoded CSV file: the recognised columns (canonical names, in
// file order) and the data rows. Values are never logged nor stored in the
// report.
type table struct {
	// columns[i] is the canonical column of field i ("" = not recognised).
	columns []string
	rows    []record
	// converted is true when the file was not UTF-8 and was read as
	// Windows-1252 (the default of Excel on Italian Windows).
	converted bool
	// unknown lists the header names that were not recognised.
	unknown []string
	// text is the content as UTF-8 without BOM (what is stored).
	text []byte
}

// record is one data row: its line in the file (1 = header) and its values
// by canonical column (trimmed; absent columns are missing from the map).
type record struct {
	line   int
	values map[string]string
}

// decodeError is a file-level problem (row 1, the header, or the whole file).
type decodeError struct {
	code    string
	message string
}

func (e *decodeError) Error() string { return e.code + ": " + e.message }

// decodeCSV parses content for kind: BOM stripped, UTF-8 or Windows-1252,
// separator ';', ',' or tab detected on the header line, header names
// matched case- and accent-insensitively against the columns of kind.
func decodeCSV(kind string, content []byte) (*table, error) {
	if len(content) > MaxFileBytes {
		return nil, &decodeError{"file_too_large", "Il file supera i 5 MB."}
	}
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	t := &table{}
	if !utf8.Valid(content) {
		content = fromWindows1252(content)
		t.converted = true
	}
	t.text = content
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, &decodeError{"empty_file", "Il file è vuoto."}
	}
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = detectSeparator(content)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = false
	header, err := r.Read()
	if err != nil {
		return nil, &decodeError{"invalid_csv", "Impossibile leggere l'intestazione del file."}
	}
	defs := columnsOf(kind)
	seen := map[string]bool{}
	t.columns = make([]string, len(header))
	for i, h := range header {
		c := canonicalColumn(defs, h)
		switch {
		case c == "":
			if n := strings.TrimSpace(h); n != "" {
				t.unknown = append(t.unknown, truncate(n, 40))
			}
		case seen[c]:
			return nil, &decodeError{"duplicate_column", "La colonna " + c + " compare più di una volta."}
		default:
			seen[c] = true
			t.columns[i] = c
		}
	}
	for _, d := range defs {
		if d.required && !seen[d.name] {
			return nil, &decodeError{"missing_column", "Manca la colonna obbligatoria " + d.name + "."}
		}
	}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, &decodeError{"invalid_csv", "Riga " + strconv.Itoa(pe.Line) + " non leggibile: controlla le virgolette."}
			}
			return nil, &decodeError{"invalid_csv", "File CSV non leggibile."}
		}
		line, _ := r.FieldPos(0)
		row := record{line: line, values: map[string]string{}}
		blank := true
		for i, v := range rec {
			if i >= len(t.columns) || t.columns[i] == "" {
				continue
			}
			v = strings.TrimSpace(v)
			if v != "" {
				blank = false
			}
			row.values[t.columns[i]] = v
		}
		if blank {
			continue
		}
		if limit := MaxRowsFor(kind); len(t.rows) == limit {
			return nil, &decodeError{"too_many_rows", "Il file supera le " + strconv.Itoa(limit) + " righe."}
		}
		t.rows = append(t.rows, row)
	}
	if len(t.rows) == 0 {
		return nil, &decodeError{"empty_file", "Il file non contiene righe da importare."}
	}
	return t, nil
}

// recognised returns the canonical columns present in the file, in order.
func (t *table) recognised() []string {
	out := []string{}
	for _, c := range t.columns {
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// has reports whether the file has column c.
func (t *table) has(c string) bool {
	for _, x := range t.columns {
		if x == c {
			return true
		}
	}
	return false
}

// detectSeparator picks ';', ',' or tab: the most frequent outside quotes
// on the first line (';' wins ties: Italian Excel exports use it).
func detectSeparator(content []byte) rune {
	counts := map[rune]int{}
	inQuotes := false
	for _, r := range string(firstLine(content)) {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case !inQuotes && (r == ';' || r == ',' || r == '\t'):
			counts[r]++
		}
	}
	best := ';'
	for _, r := range []rune{',', '\t'} {
		if counts[r] > counts[best] {
			best = r
		}
	}
	return best
}

func firstLine(b []byte) []byte {
	if i := bytes.IndexAny(b, "\r\n"); i >= 0 {
		return b[:i]
	}
	return b
}

// canonicalColumn maps a header name to the canonical column of defs ("" if
// unknown): lowercase, accents removed, spaces, dots and dashes as "_".
func canonicalColumn(defs []column, h string) string {
	k := normalizeHeader(h)
	for _, d := range defs {
		if k == d.name {
			return d.name
		}
		for _, a := range d.aliases {
			if k == a {
				return d.name
			}
		}
	}
	return ""
}

func normalizeHeader(h string) string {
	h = customer.Fold(strings.TrimSpace(h))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore && b.Len() > 0:
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}

// cp1252 maps the bytes 0x80-0x9F of Windows-1252 that differ from Latin-1.
var cp1252 = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰',
	0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•',
	0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

// fromWindows1252 decodes b as Windows-1252 (a superset of Latin-1).
func fromWindows1252(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/8)
	for _, c := range b {
		switch {
		case c < 0x80:
			out.WriteByte(c)
		case cp1252[c] != 0:
			out.WriteRune(cp1252[c])
		default:
			out.WriteRune(rune(c))
		}
	}
	return out.Bytes()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
