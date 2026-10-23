package notify

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// The QR round trip is checked with a minimal decoder written for the test
// (ISO/IEC 18004, versions 1-3, error correction M, numeric /
// alphanumeric / byte segments): it locates
// the function patterns, finds the mask by trying all eight, reads the
// codewords in the standard zig-zag order, verifies the Reed-Solomon
// syndromes and the format information, and decodes the payload. No
// third-party decoder is needed.

func TestRenderQR_RoundTrip(t *testing.T) {
	for _, payload := range []string{
		QRPayload("LD-2027-000001"),
		QRPayload("LIDODELFINO-2026-123456"),
		"ARENILE:X",
		"arenile:lido-2026-è", // byte mode
	} {
		t.Run(payload, func(t *testing.T) {
			code, err := qrcode.New(payload, qrcode.Medium)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeQR(code.Bitmap())
			if err != nil {
				t.Fatal(err)
			}
			if got != payload {
				t.Fatalf("decoded %q, want %q", got, payload)
			}
		})
	}
}

func TestRenderQR_PNG(t *testing.T) {
	b, ct, err := RenderQR(QRPayload("LD-2027-000001"), QRPNG)
	if err != nil || ct != "image/png" {
		t.Fatalf("%v %q", err, ct)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	// Version 2 (25 modules) + 2*4 quiet zone = 33 modules * 8 px.
	if w := img.Bounds().Dx(); w != 33*qrModulePixels || img.Bounds().Dy() != w {
		t.Fatalf("png size %v", img.Bounds())
	}
	// Quiet zone is white, top-left finder corner is black.
	if r, _, _, _ := img.At(2, 2).RGBA(); r != 0xffff {
		t.Fatal("quiet zone not white")
	}
	if r, _, _, _ := img.At(4*qrModulePixels+2, 4*qrModulePixels+2).RGBA(); r != 0 {
		t.Fatal("finder corner not black")
	}
}

func TestRenderQR_SVGMatchesMatrix(t *testing.T) {
	payload := QRPayload("LD-2027-000001")
	b, ct, err := RenderQR(payload, QRSVG)
	if err != nil || ct != "image/svg+xml" {
		t.Fatalf("%v %q", err, ct)
	}
	if err := xml.Unmarshal(b, new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	code, _ := qrcode.New(payload, qrcode.Medium)
	bm := code.Bitmap()
	// Rebuild the matrix from the path and decode it.
	start := strings.Index(string(b), ` d="`) + 4
	path := string(b)[start : start+strings.Index(string(b)[start:], `"`)]
	got := make([][]bool, len(bm))
	for i := range got {
		got[i] = make([]bool, len(bm))
	}
	for _, seg := range strings.Split(strings.TrimSuffix(path, "z"), "z") {
		var x, y, w int
		if _, err := fmtSscanf(seg, &x, &y, &w); err != nil {
			t.Fatalf("segment %q: %v", seg, err)
		}
		for i := x; i < x+w; i++ {
			got[y][i] = true
		}
	}
	for y := range bm {
		for x := range bm {
			if got[y][x] != bm[y][x] {
				t.Fatalf("module (%d,%d) differs", x, y)
			}
		}
	}
	if s, err := decodeQR(got); err != nil || s != payload {
		t.Fatalf("decoded %q %v", s, err)
	}
}

func TestRenderQR_Invalid(t *testing.T) {
	for _, tt := range []struct{ payload, format string }{
		{"", QRSVG}, {strings.Repeat("x", maxQRPayload+1), QRPNG}, {"ok", "gif"},
	} {
		if _, _, err := RenderQR(tt.payload, tt.format); !errors.Is(err, ErrInvalidQR) {
			t.Errorf("RenderQR(%d bytes, %q) err = %v", len(tt.payload), tt.format, err)
		}
	}
}

// ---- test decoder ---------------------------------------------------------------

func fmtSscanf(seg string, x, y, w *int) (int, error) {
	var w2 int
	n, err := sscan(seg, x, y, w, &w2)
	return n, err
}

// sscan parses "M<x> <y>h<w>v1h-<w>".
func sscan(seg string, x, y, w, w2 *int) (int, error) {
	var rest string
	if !strings.HasPrefix(seg, "M") {
		return 0, errors.New("no M")
	}
	parts := strings.SplitN(seg[1:], " ", 2)
	if len(parts) != 2 {
		return 0, errors.New("no space")
	}
	*x = atoi(parts[0])
	ys, rest, _ := strings.Cut(parts[1], "h")
	*y = atoi(ys)
	ws, rest, _ := strings.Cut(rest, "v1h-")
	*w = atoi(ws)
	*w2 = atoi(rest)
	if *w != *w2 {
		return 0, errors.New("asymmetric run")
	}
	return 4, nil
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

var qrBlocksM = map[int]struct{ total, data int }{1: {26, 16}, 2: {44, 28}, 3: {70, 44}}

func decodeQR(withBorder [][]bool) (string, error) {
	const border = 4
	n := len(withBorder) - 2*border
	m := make([][]bool, n) // m[y][x]
	for y := range n {
		m[y] = withBorder[y+border][border : border+n]
	}
	version := (n - 17) / 4
	blk, ok := qrBlocksM[version]
	if !ok || n != 17+4*version {
		return "", errors.New("unsupported version")
	}
	fn := functionModules(n, version)
	for mask := range 8 {
		cw := readCodewords(m, fn, mask, blk.total)
		if !rsValid(cw, blk.total-blk.data) {
			continue
		}
		if !formatMatches(m, n, mask) {
			return "", errors.New("format information does not match mask/level M")
		}
		return decodeSegments(cw[:blk.data])
	}
	return "", errors.New("no mask yields a valid Reed-Solomon codeword")
}

func functionModules(n, version int) [][]bool {
	fn := make([][]bool, n)
	for i := range fn {
		fn[i] = make([]bool, n)
	}
	mark := func(x0, y0, x1, y1 int) {
		for y := max(y0, 0); y <= min(y1, n-1); y++ {
			for x := max(x0, 0); x <= min(x1, n-1); x++ {
				fn[y][x] = true
			}
		}
	}
	mark(0, 0, 8, 8)     // top-left finder, separator, format
	mark(n-8, 0, n-1, 8) // top-right
	mark(0, n-8, 8, n-1) // bottom-left (+ dark module)
	mark(6, 0, 6, n-1)   // timing column
	mark(0, 6, n-1, 6)   // timing row
	if version >= 2 {
		c := n - 7
		mark(c-2, c-2, c+2, c+2) // the single alignment pattern of v2-v6
	}
	return fn
}

func maskBit(mask, y, x int) bool {
	switch mask {
	case 0:
		return (y+x)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (y+x)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (y*x)%2+(y*x)%3 == 0
	case 6:
		return ((y*x)%2+(y*x)%3)%2 == 0
	default:
		return ((y+x)%2+(y*x)%3)%2 == 0
	}
}

func readCodewords(m, fn [][]bool, mask, total int) []byte {
	n := len(m)
	out := make([]byte, 0, total)
	var cur byte
	bits := 0
	up := true
	for x := n - 1; x > 0; x -= 2 {
		if x == 6 {
			x--
		}
		for i := range n {
			y := i
			if up {
				y = n - 1 - i
			}
			for _, xx := range []int{x, x - 1} {
				if fn[y][xx] {
					continue
				}
				bit := m[y][xx] != maskBit(mask, y, xx)
				cur <<= 1
				if bit {
					cur |= 1
				}
				bits++
				if bits == 8 {
					if len(out) < total {
						out = append(out, cur)
					}
					cur, bits = 0, 0
				}
			}
		}
		up = !up
	}
	return out
}

// GF(256) with the QR polynomial x^8+x^4+x^3+x^2+1.
var gfExp, gfLog = func() ([512]byte, [256]int) {
	var exp [512]byte
	var log [256]int
	x := 1
	for i := range 255 {
		exp[i] = byte(x)
		log[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < 512; i++ {
		exp[i] = exp[i-255]
	}
	return exp, log
}()

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

// rsValid checks that the codeword evaluates to 0 at alpha^0..alpha^(ec-1).
func rsValid(cw []byte, ec int) bool {
	for i := range ec {
		var s byte
		for _, c := range cw {
			s = gfMul(s, gfExp[i]) ^ c
		}
		if s != 0 {
			return false
		}
	}
	return true
}

// formatMatches reads both copies of the 15-bit format information and
// compares them with the BCH codeword of level M and mask.
func formatMatches(m [][]bool, n, mask int) bool {
	data := 0b00<<3 | mask // level M = 00
	rem := data
	for range 10 {
		rem = rem<<1 ^ ((rem>>9)&1)*0x537
	}
	want := (data<<10 | rem) ^ 0x5412
	get := func(x, y int) int {
		if m[y][x] {
			return 1
		}
		return 0
	}
	var a, b int
	// Copy 1 around the top-left finder, bit 14 first.
	for _, p := range [][2]int{{8, 0}, {8, 1}, {8, 2}, {8, 3}, {8, 4}, {8, 5}, {8, 7}, {8, 8}, {7, 8}, {5, 8}, {4, 8}, {3, 8}, {2, 8}, {1, 8}, {0, 8}} {
		a = a<<1 | get(p[0], p[1])
	}
	// Copy 2: bottom-left column then top-right row.
	for y := n - 1; y >= n-7; y-- {
		b = b<<1 | get(8, y)
	}
	for x := n - 8; x < n; x++ {
		b = b<<1 | get(x, 8)
	}
	rev := func(v int) int {
		r := 0
		for range 15 {
			r = r<<1 | v&1
			v >>= 1
		}
		return r
	}
	return (a == want || rev(a) == want) && (b == want || rev(b) == want)
}

// decodeSegments decodes numeric, alphanumeric and byte segments (the
// encoder picks the most compact modes, e.g. alphanumeric for upper-case
// booking codes) until the terminator.
func decodeSegments(data []byte) (string, error) {
	const alnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
	total := len(data) * 8
	bit := func(i int) int { return int(data[i/8]>>(7-uint(i%8))) & 1 }
	pos := 0
	read := func(n int) (int, error) {
		if pos+n > total {
			return 0, errors.New("segment overflows data")
		}
		v := 0
		for i := range n {
			v = v<<1 | bit(pos+i)
		}
		pos += n
		return v, nil
	}
	var out strings.Builder
	for pos+4 <= total {
		mode, _ := read(4)
		switch mode {
		case 0b0000:
			return out.String(), nil
		case 0b0001: // numeric
			n, err := read(10)
			if err != nil {
				return "", err
			}
			for n > 0 {
				k := min(n, 3)
				v, err := read(map[int]int{3: 10, 2: 7, 1: 4}[k])
				if err != nil {
					return "", err
				}
				fmt.Fprintf(&out, "%0*d", k, v)
				n -= k
			}
		case 0b0010: // alphanumeric
			n, err := read(9)
			if err != nil {
				return "", err
			}
			for ; n >= 2; n -= 2 {
				v, err := read(11)
				if err != nil || v/45 >= 45 {
					return "", errors.New("bad alphanumeric pair")
				}
				out.WriteByte(alnum[v/45])
				out.WriteByte(alnum[v%45])
			}
			if n == 1 {
				v, err := read(6)
				if err != nil || v >= 45 {
					return "", errors.New("bad alphanumeric char")
				}
				out.WriteByte(alnum[v])
			}
		case 0b0100: // byte
			n, err := read(8)
			if err != nil {
				return "", err
			}
			for range n {
				v, err := read(8)
				if err != nil {
					return "", err
				}
				out.WriteByte(byte(v))
			}
		default:
			return "", fmt.Errorf("unsupported mode %04b", mode)
		}
	}
	return out.String(), nil
}
