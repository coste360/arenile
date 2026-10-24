package escpos

import (
	"strings"
	"unicode"

	"golang.org/x/text/encoding/charmap"
)

// charset encodes UTF-8 text to one codepage.
type charset struct {
	cm    *charmap.Charmap
	table byte // Epson ESC t number
}

// Epson ESC t numbers: PC437 0, PC850 2, WPC1252 16, PC852 18, PC858 19.
var charsets = map[Codepage]charset{
	CP858:  {cm: charmap.CodePage858, table: 19},
	CP437:  {cm: charmap.CodePage437, table: 0},
	CP850:  {cm: charmap.CodePage850, table: 2},
	CP852:  {cm: charmap.CodePage852, table: 18},
	CP1252: {cm: charmap.Windows1252, table: 16},
}

// Valid reports whether cp is a supported codepage.
func (cp Codepage) Valid() bool {
	_, ok := charsets[cp]
	return ok
}

// EpsonTable is the ESC t number of cp on Epson printers.
func (cp Codepage) EpsonTable() int { return int(charsets[cp].table) }

// encode converts s to the codepage: control characters become spaces,
// characters missing from the table are transliterated (fallbacks) or
// stripped of their diacritics, and as a last resort printed as '?':
// nothing is dropped silently.
func (c charset) encode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			out = append(out, ' ')
			continue
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			continue // control characters never reach the printer
		case r < 0x80:
			out = append(out, byte(r))
			continue
		}
		if b, ok := c.cm.EncodeRune(r); ok {
			out = append(out, b)
			continue
		}
		if fb, ok := fallbacks[r]; ok {
			out = append(out, c.encodeASCIIish(fb)...)
			continue
		}
		if base := stripDiacritic(r); base != r {
			if b, ok := c.cm.EncodeRune(base); ok {
				out = append(out, b)
				continue
			}
		}
		out = append(out, '?')
	}
	return out
}

// encodeASCIIish encodes a fallback string (ASCII or runes of the table).
func (c charset) encodeASCIIish(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x80 {
			out = append(out, byte(r))
		} else if b, ok := c.cm.EncodeRune(r); ok {
			out = append(out, b)
		} else {
			out = append(out, '?')
		}
	}
	return out
}

// fallbacks transliterate common characters absent from some tables.
var fallbacks = map[rune]string{
	'€': "EUR",
	'‘': "'", '’': "'", '‚': "'", '′': "'",
	'“': `"`, '”': `"`, '„': `"`, '″': `"`,
	'«': `"`, '»': `"`,
	'–': "-", '—': "-", '−': "-", '‐': "-", '‑': "-",
	'…': "...",
	' ': " ", ' ': " ", ' ': " ", ' ': " ",
	'•': "*", '·': ".",
	'ß': "ss", 'ẞ': "SS",
	'Œ': "OE", 'œ': "oe", 'Æ': "AE", 'æ': "ae",
	'©': "(c)", '®': "(R)", '™': "TM",
	'×': "x",
	'→': "->", '←': "<-",
	'½': "1/2", '¼': "1/4", '¾': "3/4",
	'°': "o",
}

// stripDiacritic maps a Latin letter with a diacritic to its base letter
// (à → a, Ž → Z), or returns r.
func stripDiacritic(r rune) rune {
	if b, ok := baseLetters[r]; ok {
		return b
	}
	return r
}

// baseLetters covers Latin-1 Supplement and Latin Extended-A letters.
var baseLetters = func() map[rune]rune {
	groups := map[rune]string{
		'A': "ÀÁÂÃÄÅĀĂĄ", 'a': "àáâãäåāăą",
		'C': "ÇĆĈĊČ", 'c': "çćĉċč",
		'D': "ĎĐ", 'd': "ďđ",
		'E': "ÈÉÊËĒĔĖĘĚ", 'e': "èéêëēĕėęě",
		'G': "ĜĞĠĢ", 'g': "ĝğġģ",
		'H': "ĤĦ", 'h': "ĥħ",
		'I': "ÌÍÎÏĨĪĬĮİ", 'i': "ìíîïĩīĭįı",
		'J': "Ĵ", 'j': "ĵ",
		'K': "Ķ", 'k': "ķ",
		'L': "ĹĻĽĿŁ", 'l': "ĺļľŀł",
		'N': "ÑŃŅŇ", 'n': "ñńņň",
		'O': "ÒÓÔÕÖØŌŎŐ", 'o': "òóôõöøōŏő",
		'R': "ŔŖŘ", 'r': "ŕŗř",
		'S': "ŚŜŞŠ", 's': "śŝşš",
		'T': "ŢŤŦ", 't': "ţťŧ",
		'U': "ÙÚÛÜŨŪŬŮŰŲ", 'u': "ùúûüũūŭůűų",
		'W': "Ŵ", 'w': "ŵ",
		'Y': "ÝŶŸ", 'y': "ýÿŷ",
		'Z': "ŹŻŽ", 'z': "źżž",
	}
	m := make(map[rune]rune)
	for base, letters := range groups {
		for _, r := range letters {
			m[r] = base
		}
	}
	return m
}()

// Upper returns s in upper case for headings (locale-independent).
func Upper(s string) string {
	return strings.Map(unicode.ToUpper, s)
}
