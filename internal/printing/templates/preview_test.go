//go:build preview

package templates

// Visual preview of the golden ESC/POS fixtures: decodes the bytes the
// printer would receive (text, styles, alignment, raster logo, QR, cut,
// drawer kick) into one HTML page per fixture, laid out on the dot grid of
// the paper (576 dots on 80 mm, 384 on 58 mm, font A 12x24, font B 9x17).
// Not part of the normal test run:
//
//	ESCPOS_PREVIEW_DIR=/tmp/escpos go test -tags preview -run TestPreview ./internal/printing/templates/
//
// Open the pages in a browser (or screenshot them) to see what prints.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/text/encoding/charmap"
)

// tables maps the ESC t table numbers used by the encoder to charmaps.
var tables = map[byte]*charmap.Charmap{
	0: charmap.CodePage437, 2: charmap.CodePage850, 16: charmap.Windows1252,
	18: charmap.CodePage852, 19: charmap.CodePage858,
}

type pvSpan struct {
	text       string
	bold, b    bool // emphasized, font B
	w, h       int  // magnification
	underlined bool
}

type pvLine struct {
	align int
	spans []pvSpan
	html  string // block content (image, QR, cut, note) instead of spans
}

type pvState struct {
	align, w, h   int
	bold, fontB   bool
	underline     bool
	cm            *charmap.Charmap
	cur           []pvSpan
	lines         []pvLine
	qrData        string
	qrSize        int
	lineAlign     int
	lineHasOutput bool
}

func (s *pvState) text(b byte) {
	if !s.lineHasOutput {
		s.lineAlign = s.align
		s.lineHasOutput = true
	}
	r := string(rune(b))
	if b >= 0x80 && s.cm != nil {
		r = string(s.cm.DecodeByte(b))
	}
	n := len(s.cur)
	if n > 0 {
		last := &s.cur[n-1]
		if last.bold == s.bold && last.b == s.fontB && last.w == s.w && last.h == s.h && last.underlined == s.underline {
			last.text += r
			return
		}
	}
	s.cur = append(s.cur, pvSpan{text: r, bold: s.bold, b: s.fontB, w: s.w, h: s.h, underlined: s.underline})
}

func (s *pvState) newline() {
	align := s.align
	if s.lineHasOutput {
		align = s.lineAlign
	}
	s.lines = append(s.lines, pvLine{align: align, spans: s.cur})
	s.cur = nil
	s.lineHasOutput = false
}

func (s *pvState) block(content string) {
	if len(s.cur) > 0 {
		s.newline()
	}
	s.lines = append(s.lines, pvLine{align: s.align, html: content})
}

func pngURI(img image.Image) string {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodePreview(t *testing.T, data []byte) []pvLine {
	s := &pvState{w: 1, h: 1, cm: charmap.CodePage437}
	for i := 0; i < len(data); i++ {
		c := data[i]
		arg := func(k int) byte {
			if i+k >= len(data) {
				t.Fatalf("truncated command at %d", i)
			}
			return data[i+k]
		}
		switch {
		case c == 0x0a:
			s.newline()
		case c == 0x1b: // ESC
			switch arg(1) {
			case '@':
				*s = pvState{w: 1, h: 1, cm: s.cm, lines: s.lines}
				i++
			case 't':
				if cm, ok := tables[arg(2)]; ok {
					s.cm = cm
				}
				i += 2
			case 'a':
				s.align = int(arg(2))
				i += 2
			case 'E':
				s.bold = arg(2) == 1
				i += 2
			case '-':
				s.underline = arg(2) != 0
				i += 2
			case 'M':
				s.fontB = arg(2) == 1
				i += 2
			case 'd':
				if len(s.cur) > 0 {
					s.newline()
				}
				for range int(arg(2)) {
					s.lines = append(s.lines, pvLine{})
				}
				i += 2
			case 'p':
				pin := 2
				if arg(2) == 1 {
					pin = 5
				}
				s.block(fmt.Sprintf(`<div class="note">[impulso cassetto, pin %d]</div>`, pin))
				i += 4
			default:
				t.Fatalf("unknown ESC %q at %d", arg(1), i)
			}
		case c == 0x1d: // GS
			switch arg(1) {
			case '!':
				n := arg(2)
				s.w, s.h = int(n>>4)+1, int(n&0x0f)+1
				i += 2
			case 'V':
				label := "taglio completo"
				if arg(2) == 1 {
					label = "taglio parziale"
				}
				s.block(`<div class="cut">✂ ` + label + `</div>`)
				i += 2
			case '(':
				if arg(2) != 'k' {
					t.Fatalf("unknown GS ( %q", arg(2))
				}
				n := int(arg(3)) | int(arg(4))<<8
				payload := data[i+5 : i+5+n]
				switch payload[1] {
				case 67:
					s.qrSize = int(payload[2])
				case 80:
					s.qrData = string(payload[3:])
				case 81:
					q, err := qrcode.New(s.qrData, qrcode.Medium)
					if err != nil {
						t.Fatal(err)
					}
					q.DisableBorder = true
					bm := q.Bitmap()
					px := len(bm) * s.qrSize
					s.block(fmt.Sprintf(`<img class="px" width="%d" height="%d" src="%s" alt="QR %s">`,
						px, px, pngURI(scaleBitmap(bm, s.qrSize)), html.EscapeString(s.qrData)))
				}
				i += 4 + n
			case 'v':
				wb := int(arg(4)) | int(arg(5))<<8
				h := int(arg(6)) | int(arg(7))<<8
				bits := data[i+8 : i+8+wb*h]
				img := image.NewGray(image.Rect(0, 0, wb*8, h))
				for y := range h {
					for x := range wb * 8 {
						on := bits[y*wb+x/8]&(0x80>>(x%8)) != 0
						v := uint8(255)
						if on {
							v = 0
						}
						img.SetGray(x, y, color.Gray{Y: v})
					}
				}
				s.block(fmt.Sprintf(`<img class="px" width="%d" height="%d" src="%s" alt="logo">`, wb*8, h, pngURI(img)))
				i += 7 + wb*h
			default:
				t.Fatalf("unknown GS %q at %d", arg(1), i)
			}
		case c < 0x20:
			t.Fatalf("unexpected control byte %#x at %d", c, i)
		default:
			s.text(c)
		}
	}
	if len(s.cur) > 0 {
		s.newline()
	}
	return s.lines
}

func scaleBitmap(bm [][]bool, k int) image.Image {
	img := image.NewGray(image.Rect(0, 0, len(bm)*k, len(bm)*k))
	for y, row := range bm {
		for x, on := range row {
			v := uint8(255)
			if on {
				v = 0
			}
			for dy := range k {
				for dx := range k {
					img.SetGray(x*k+dx, y*k+dy, color.Gray{Y: v})
				}
			}
		}
	}
	return img
}

func renderPreviewHTML(name string, dots int, lines []pvLine) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title><style>
body{margin:0;padding:24px;background:#d6d3ce;font-family:Menlo,Consolas,monospace}
h1{font:600 14px system-ui;margin:0 0 12px;color:#333}
.paper{width:%dpx;padding:16px 12px;background:#fff;box-shadow:0 2px 10px rgba(0,0,0,.25);color:#111}
.l{min-height:24px;white-space:pre;line-height:24px}
.a1{text-align:center}.a2{text-align:right}
.s{display:inline-block;font-size:20px;transform-origin:left bottom;vertical-align:bottom}
.fb{font-size:15px}
.bold{font-weight:700}.u{text-decoration:underline}
.cut{border-top:2px dashed #999;margin:10px -12px 4px;padding-top:2px;font:11px system-ui;color:#888;text-align:center}
.note{font:11px system-ui;color:#b45309;text-align:center}
img.px{image-rendering:pixelated;display:inline-block}
</style></head><body><h1>%s</h1><div class="paper">`, html.EscapeString(name), dots, html.EscapeString(name))
	for _, l := range lines {
		fmt.Fprintf(&b, `<div class="l a%d">`, l.align)
		if l.html != "" {
			b.WriteString(l.html)
		}
		for _, sp := range l.spans {
			cls := "s"
			if sp.bold {
				cls += " bold"
			}
			if sp.b {
				cls += " fb"
			}
			if sp.underlined {
				cls += " u"
			}
			cw := 12.04
			if sp.b {
				cw = 9.03
			}
			n := len([]rune(sp.text))
			style := ""
			if sp.w != 1 || sp.h != 1 {
				// Magnify on the dot grid: width w x, height h x.
				style = fmt.Sprintf(` style="width:%.0fpx;height:%dpx;line-height:%dpx;transform:scale(%d,%d);margin-right:%.0fpx;margin-top:%dpx"`,
					cw*float64(n), 24, 24, sp.w, sp.h, cw*float64(n*(sp.w-1)), 24*(sp.h-1))
			}
			fmt.Fprintf(&b, `<span class="%s"%s>%s</span>`, cls, style, html.EscapeString(sp.text))
		}
		b.WriteString("</div>")
	}
	b.WriteString("</div></body></html>")
	return b.String()
}

func TestPreview(t *testing.T) {
	out := os.Getenv("ESCPOS_PREVIEW_DIR")
	if out == "" {
		t.Skip("set ESCPOS_PREVIEW_DIR")
	}
	files, err := filepath.Glob(filepath.Join("testdata", "*.bin"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: fixed test fixture path
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".bin")
		dots := 576
		if strings.HasSuffix(name, "_58") {
			dots = 384
		}
		page := renderPreviewHTML(name, dots, decodePreview(t, data))
		if err := os.WriteFile(filepath.Join(out, "preview-"+name+".html"), []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
