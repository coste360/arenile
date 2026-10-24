package escpos

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Config is the printer side of an Encoder.
type Config struct {
	// CharsPerLine is the number of font A columns at normal size (24-64).
	CharsPerLine int
	// Codepage is the character table text is encoded to (default CP858).
	Codepage Codepage
	// CodepageTable overrides the ESC t table number (clones); nil = the
	// Epson number of Codepage.
	CodepageTable *int
}

// Encoder builds an ESC/POS byte stream. Methods return the encoder so that
// calls chain; it never fails: invalid arguments are clamped. Not safe for
// concurrent use.
type Encoder struct {
	cfg   Config
	cs    charset
	buf   bytes.Buffer
	width int // current horizontal magnification (1-8)
	fontB bool
}

// New returns an Encoder for cfg. Unknown codepages fall back to CP858 and
// CharsPerLine is clamped to 24-64.
func New(cfg Config) *Encoder {
	if _, ok := charsets[cfg.Codepage]; !ok {
		cfg.Codepage = CP858
	}
	cfg.CharsPerLine = clamp(cfg.CharsPerLine, 24, 64)
	return &Encoder{cfg: cfg, cs: charsets[cfg.Codepage], width: 1}
}

// Bytes returns the encoded stream (a copy).
func (e *Encoder) Bytes() []byte { return bytes.Clone(e.buf.Bytes()) }

// Raw appends bytes as they are (escape hatch for tests and extensions).
func (e *Encoder) Raw(b ...byte) *Encoder {
	e.buf.Write(b)
	return e
}

// Init resets the printer (ESC @) and selects the codepage (ESC t n).
func (e *Encoder) Init() *Encoder {
	e.width, e.fontB = 1, false
	table := e.cs.table
	if e.cfg.CodepageTable != nil {
		table = byte(clamp(*e.cfg.CodepageTable, 0, 255))
	}
	return e.Raw(ESC, '@', ESC, 't', table)
}

// Align sets the justification of the following lines (ESC a n).
func (e *Encoder) Align(a Align) *Encoder {
	if a > Right {
		a = Left
	}
	return e.Raw(ESC, 'a', byte(a))
}

// Bold switches emphasized mode (ESC E n).
func (e *Encoder) Bold(on bool) *Encoder { return e.Raw(ESC, 'E', b2b(on)) }

// Underline switches 1-dot underline (ESC - n).
func (e *Encoder) Underline(on bool) *Encoder { return e.Raw(ESC, '-', b2b(on)) }

// FontB selects the small font B (ESC M 1) or back to font A (ESC M 0).
// Font B has about 4/3 the columns of font A; wrapping accounts for it.
func (e *Encoder) FontB(on bool) *Encoder {
	e.fontB = on
	return e.Raw(ESC, 'M', b2b(on))
}

// Size sets the character magnification (GS ! n), width and height 1-8.
func (e *Encoder) Size(w, h int) *Encoder {
	w, h = clamp(w, 1, 8), clamp(h, 1, 8)
	e.width = w
	return e.Raw(GS, '!', byte((w-1)<<4|(h-1)))
}

// Width is the number of characters per line with the current font and
// magnification.
func (e *Encoder) Width() int {
	n := e.cfg.CharsPerLine
	if e.fontB {
		n = n * 4 / 3
	}
	return max(1, n/e.width)
}

// Text encodes s to the codepage, word-wraps it to the current columns and
// ends every line with LF. Embedded newlines start a new line.
func (e *Encoder) Text(s string) *Encoder {
	for _, para := range strings.Split(s, "\n") {
		for _, l := range wrap(e.cs.encode(para), e.Width()) {
			e.buf.Write(l)
			e.buf.WriteByte(LF)
		}
	}
	return e
}

// Line prints s on one line, truncated to the current columns, then LF.
func (e *Encoder) Line(s string) *Encoder {
	b := e.cs.encode(s)
	if n := e.Width(); len(b) > n {
		b = b[:n]
	}
	e.buf.Write(b)
	e.buf.WriteByte(LF)
	return e
}

// Columns prints a two-column row: left wrapped on the left, right
// right-aligned on the first line. A right part wider than half the line
// goes on its own line.
func (e *Encoder) Columns(left, right string) *Encoder {
	n := e.Width()
	l, r := e.cs.encode(left), e.cs.encode(right)
	if len(r) > n {
		r = r[:n]
	}
	if len(r) > n/2 && len(l)+1+len(r) > n {
		for _, line := range wrap(l, n) {
			e.buf.Write(line)
			e.buf.WriteByte(LF)
		}
		e.buf.Write(bytes.Repeat([]byte{' '}, n-len(r)))
		e.buf.Write(r)
		e.buf.WriteByte(LF)
		return e
	}
	lines := wrap(l, n-len(r)-1)
	if len(lines) == 0 {
		lines = [][]byte{nil}
	}
	for i, line := range lines {
		e.buf.Write(line)
		if i == 0 {
			e.buf.Write(bytes.Repeat([]byte{' '}, n-len(line)-len(r)))
			e.buf.Write(r)
		}
		e.buf.WriteByte(LF)
	}
	return e
}

// Rule prints a full-width line of c (ASCII, default '-').
func (e *Encoder) Rule(c byte) *Encoder {
	if c < 0x20 || c > 0x7e {
		c = '-'
	}
	e.buf.Write(bytes.Repeat([]byte{c}, e.Width()))
	e.buf.WriteByte(LF)
	return e
}

// Feed prints and feeds n lines (ESC d n), n 0-255.
func (e *Encoder) Feed(n int) *Encoder {
	if n <= 0 {
		return e
	}
	return e.Raw(ESC, 'd', byte(clamp(n, 0, 255)))
}

// Cut cuts the paper (GS V 0 full, GS V 1 partial); CutNone writes nothing.
func (e *Encoder) Cut(m CutMode) *Encoder {
	switch m {
	case CutFull:
		return e.Raw(GS, 'V', 0)
	case CutPartial:
		return e.Raw(GS, 'V', 1)
	default:
		return e
	}
}

// KickDrawer pulses the cash drawer connector pin 2 or 5 (ESC p m t1 t2,
// 50 ms on, 500 ms off). Other pins write nothing.
func (e *Encoder) KickDrawer(pin int) *Encoder {
	switch pin {
	case 2:
		return e.Raw(ESC, 'p', 0, 25, 250)
	case 5:
		return e.Raw(ESC, 'p', 1, 25, 250)
	default:
		return e
	}
}

// ECC is the QR error correction level.
type ECC byte

// QR error correction levels (GS ( k function 169).
const (
	ECCL ECC = 48
	ECCM ECC = 49
	ECCQ ECC = 50
	ECCH ECC = 51
)

// QROptions are the QR code parameters: Size is the module size in dots
// (1-16, default 6), ECC the error correction (default M).
type QROptions struct {
	Size int
	ECC  ECC
}

// MaxQRData is the largest payload accepted by QR (model 2, bytes).
const MaxQRData = 7089

// QR prints data as a QR code, model 2 (GS ( k: select model, module size,
// error correction, store, print). Empty or oversized data writes nothing.
func (e *Encoder) QR(data string, o QROptions) *Encoder {
	if data == "" || len(data) > MaxQRData {
		return e
	}
	size := o.Size
	if size == 0 {
		size = 6
	}
	size = clamp(size, 1, 16)
	ecc := o.ECC
	if ecc < ECCL || ecc > ECCH {
		ecc = ECCM
	}
	e.Raw(GS, '(', 'k', 4, 0, 49, 65, 50, 0) // function 165: model 2
	e.Raw(GS, '(', 'k', 3, 0, 49, 67, byte(size))
	e.Raw(GS, '(', 'k', 3, 0, 49, 69, byte(ecc))
	n := len(data) + 3
	e.Raw(GS, '(', 'k', byte(n&0xff), byte(n>>8), 49, 80, 48)
	e.buf.WriteString(data)
	return e.Raw(GS, '(', 'k', 3, 0, 49, 81, 48)
}

// Raster prints a 1-bit image (GS v 0, normal density). A Raster whose Data
// length does not match its size writes nothing.
func (e *Encoder) Raster(r Raster) *Encoder {
	if r.WidthBytes <= 0 || r.Height <= 0 || r.WidthBytes > 0xffff || r.Height > 0xffff ||
		len(r.Data) != r.WidthBytes*r.Height {
		return e
	}
	e.Raw(GS, 'v', '0', 0, byte(r.WidthBytes&0xff), byte(r.WidthBytes>>8), byte(r.Height&0xff), byte(r.Height>>8))
	e.buf.Write(r.Data)
	return e
}

// String renders the stream for debugging: printable ASCII as is, other
// bytes as <hex>.
func (e *Encoder) String() string { return Dump(e.buf.Bytes()) }

// Dump renders b with printable ASCII as is, LF as a newline and other
// bytes as <hex> (golden test diffs, debugging).
func Dump(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c == LF:
			sb.WriteString("<0a>\n")
		case c >= 0x20 && c < 0x7f:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "<%02x>", c)
		}
	}
	return sb.String()
}

// wrap splits b into lines of at most n bytes, breaking at spaces when
// possible; words longer than n are split.
func wrap(b []byte, n int) [][]byte {
	if n < 1 {
		n = 1
	}
	b = bytes.TrimRight(b, " ")
	var out [][]byte
	for len(b) > n {
		cut := bytes.LastIndexByte(b[:n+1], ' ')
		if cut <= 0 {
			out = append(out, b[:n])
			b = b[n:]
			continue
		}
		out = append(out, bytes.TrimRight(b[:cut], " "))
		b = bytes.TrimLeft(b[cut:], " ")
	}
	if len(b) > 0 || len(out) == 0 {
		out = append(out, b)
	}
	return out
}

// TextWidth is the number of columns s takes once encoded to cp.
func TextWidth(cp Codepage, s string) int {
	cs, ok := charsets[cp]
	if !ok {
		return utf8.RuneCountInString(s)
	}
	return len(cs.encode(s))
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

func b2b(on bool) byte {
	if on {
		return 1
	}
	return 0
}
