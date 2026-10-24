package escpos

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden fixtures in testdata")

// golden compares got with testdata/<name>.bin (rewritten with -update).
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".bin")
	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // G304: fixed test fixture path
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from golden\n got: %s\nwant: %s", name, Dump(got), Dump(want))
	}
}

func TestEncoder_Golden(t *testing.T) {
	table := 21
	tests := []struct {
		name  string
		build func() []byte
	}{
		{"init_cp858", func() []byte { return New(Config{CharsPerLine: 48}).Init().Bytes() }},
		{"init_override", func() []byte {
			return New(Config{CharsPerLine: 48, Codepage: CP437, CodepageTable: &table}).Init().Bytes()
		}},
		{"styles", func() []byte {
			return New(Config{CharsPerLine: 32}).Align(Center).Bold(true).Size(2, 2).Line("Lido Azzurro").
				Size(1, 1).Bold(false).Underline(true).Line("sotto").Underline(false).FontB(true).Line("Arenile").
				FontB(false).Align(Right).Line("destra").Bytes()
		}},
		{"wrap_cp858", func() []byte {
			return New(Config{CharsPerLine: 24, Codepage: CP858}).
				Text("Città: però è già l'ora del caffè € 1,50 — “ok” supercalifragilistichespiralidoso").Bytes()
		}},
		{"wrap_cp437", func() []byte {
			return New(Config{CharsPerLine: 24, Codepage: CP437}).Text("È già € 25,00 – Œuvre").Bytes()
		}},
		{"columns_80", func() []byte {
			e := New(Config{CharsPerLine: 48})
			return e.Columns("Ombrellone A12 prima fila", "€ 25,00").
				Columns("Una descrizione molto lunga che va a capo su più righe", "€ 1.234,50").
				Columns("", "solo destra").Rule('-').Rule('=').Bytes()
		}},
		{"columns_58_double", func() []byte {
			return New(Config{CharsPerLine: 32}).Size(2, 1).Columns("TOTALE", "€ 35,00").Bytes()
		}},
		{"qr", func() []byte { return New(Config{CharsPerLine: 48}).QR("ARN-7F3K", QROptions{}).Bytes() }},
		{"qr_large_h", func() []byte {
			return New(Config{CharsPerLine: 48}).QR("https://arenile.app/b/ARN-7F3K", QROptions{Size: 8, ECC: ECCH}).Bytes()
		}},
		{"raster", func() []byte {
			return New(Config{CharsPerLine: 48}).Raster(Raster{WidthBytes: 2, Height: 2, Data: []byte{0xff, 0x00, 0x0f, 0xf0}}).Bytes()
		}},
		{"feed_cut_kick", func() []byte {
			return New(Config{CharsPerLine: 48}).Feed(3).Cut(CutPartial).Cut(CutFull).Cut(CutNone).
				KickDrawer(2).KickDrawer(5).KickDrawer(3).Bytes()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { golden(t, tt.name, tt.build()) })
	}
}

func TestEncoder_Commands(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"init", New(Config{}).Init().Bytes(), []byte{ESC, '@', ESC, 't', 19}},
		{"init cp1252", New(Config{Codepage: CP1252}).Init().Bytes(), []byte{ESC, '@', ESC, 't', 16}},
		{"align center", New(Config{}).Align(Center).Bytes(), []byte{ESC, 'a', 1}},
		{"bold", New(Config{}).Bold(true).Bytes(), []byte{ESC, 'E', 1}},
		{"size 2x2", New(Config{}).Size(2, 2).Bytes(), []byte{GS, '!', 0x11}},
		{"size clamp", New(Config{}).Size(0, 9).Bytes(), []byte{GS, '!', 0x07}},
		{"cut partial", New(Config{}).Cut(CutPartial).Bytes(), []byte{GS, 'V', 1}},
		{"cut full", New(Config{}).Cut(CutFull).Bytes(), []byte{GS, 'V', 0}},
		{"kick pin 2", New(Config{}).KickDrawer(2).Bytes(), []byte{ESC, 'p', 0, 25, 250}},
		{"kick pin 5", New(Config{}).KickDrawer(5).Bytes(), []byte{ESC, 'p', 1, 25, 250}},
		{"feed", New(Config{}).Feed(4).Bytes(), []byte{ESC, 'd', 4}},
		{"euro cp858", New(Config{}).Line("€").Bytes(), []byte{0xd5, LF}},
		{"euro cp1252", New(Config{Codepage: CP1252}).Line("€").Bytes(), []byte{0x80, LF}},
		{"euro cp437", New(Config{Codepage: CP437}).Line("€").Bytes(), []byte("EUR\n")},
		{"accents cp858", New(Config{}).Line("àèéìòù").Bytes(), []byte{0x85, 0x8a, 0x82, 0x8d, 0x95, 0x97, LF}},
		{"control chars removed", New(Config{}).Line("a\x1b@b\x1dVc").Bytes(), []byte("a@bVc\n")},
		{"unknown rune", New(Config{}).Line("日").Bytes(), []byte("?\n")},
		{"qr empty", New(Config{}).QR("", QROptions{}).Bytes(), nil},
		{"raster invalid", New(Config{}).Raster(Raster{WidthBytes: 2, Height: 2, Data: []byte{1}}).Bytes(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got %s, want %s", Dump(tt.got), Dump(tt.want))
			}
		})
	}
}

func TestEncoder_QRStoreLength(t *testing.T) {
	data := string(bytes.Repeat([]byte{'x'}, 300))
	b := New(Config{}).QR(data, QROptions{}).Bytes()
	store := []byte{GS, '(', 'k', byte((300 + 3) & 0xff), byte((300 + 3) >> 8), 49, 80, 48}
	i := bytes.Index(b, store)
	if i < 0 {
		t.Fatalf("store command with pL/pH for 303 bytes not found: %s", Dump(b[:40]))
	}
	if !bytes.HasSuffix(b, []byte{GS, '(', 'k', 3, 0, 49, 81, 48}) {
		t.Fatal("missing print command")
	}
}

func TestWrap_Scenarios(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want []string
	}{
		{"", 10, []string{""}},
		{"abc def", 10, []string{"abc def"}},
		{"abc def ghi", 7, []string{"abc def", "ghi"}},
		{"abcdefghijk", 4, []string{"abcd", "efgh", "ijk"}},
		{"a  b   ", 1, []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := wrap([]byte(tt.in), tt.n)
		if len(got) != len(tt.want) {
			t.Fatalf("wrap(%q,%d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
		for i := range got {
			if string(got[i]) != tt.want[i] || len(got[i]) > tt.n && tt.n > 0 {
				t.Fatalf("wrap(%q,%d)[%d] = %q, want %q", tt.in, tt.n, i, got[i], tt.want[i])
			}
		}
	}
}

func TestColumns_FitWidth(t *testing.T) {
	for _, n := range []int{24, 32, 42, 48, 64} {
		e := New(Config{CharsPerLine: n})
		e.Columns("Un'etichetta piuttosto lunga per vedere il ritorno a capo", "€ 12.345,67")
		for _, line := range bytes.Split(bytes.TrimSuffix(e.Bytes(), []byte{LF}), []byte{LF}) {
			if len(line) > n {
				t.Fatalf("width %d: line %q is %d long", n, line, len(line))
			}
		}
	}
}

func TestDither_Scenarios(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1000, 100))
	for y := range 100 {
		for x := range 1000 {
			c := color.RGBA{A: 255}
			if x >= 500 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	r := Dither(img, MaxDots58, 200)
	if r.WidthBytes != 48 || r.Height != 38 || len(r.Data) != 48*38 {
		t.Fatalf("size %dx%d len %d", r.WidthBytes, r.Height, len(r.Data))
	}
	if r.Data[0] != 0xff || r.Data[47] != 0x00 {
		t.Fatalf("left must be black, right white: %#x %#x", r.Data[0], r.Data[47])
	}
	// Transparent = white.
	tr := Dither(image.NewNRGBA(image.Rect(0, 0, 10, 10)), 576, 200)
	if tr.WidthBytes != 2 || tr.Height != 10 || !bytes.Equal(tr.Data, make([]byte, 20)) {
		t.Fatalf("transparent: %+v", tr)
	}
	// Mid gray dithers to about half black.
	gray := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range gray.Pix {
		gray.Pix[i] = 128
	}
	g := Dither(gray, 576, 200)
	black := 0
	for _, b := range g.Data {
		for i := range 8 {
			if b&(1<<i) != 0 {
				black++
			}
		}
	}
	if black < 1700 || black > 2400 {
		t.Fatalf("mid gray: %d black dots of 4096", black)
	}
	if z := Dither(nil, 576, 100); z.WidthBytes != 0 || z.Data != nil {
		t.Fatal("nil image must give the zero raster")
	}
	enc := EncodeRaster(r)
	back, ok := DecodeRaster(enc)
	if !ok || back.WidthBytes != r.WidthBytes || back.Height != r.Height || !bytes.Equal(back.Data, r.Data) {
		t.Fatal("raster round trip")
	}
	if _, ok := DecodeRaster([]byte{0, 1, 0, 2, 9}); ok {
		t.Fatal("malformed raster accepted")
	}
}

func TestTextWidth(t *testing.T) {
	if got := TextWidth(CP437, "€ 1,00"); got != 8 {
		t.Fatalf("cp437: %d", got)
	}
	if got := TextWidth(CP858, "€ 1,00"); got != 6 {
		t.Fatalf("cp858: %d", got)
	}
}
