package desktop

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Tray icon colours (the Arenile mark: sea blue tile, sand umbrella).
var (
	iconSea  = color.NRGBA{0x0e, 0x6b, 0xa8, 0xff}
	iconSand = color.NRGBA{0xf2, 0xb1, 0x34, 0xff}
	iconPole = color.NRGBA{0xff, 0xff, 0xff, 0xff}
)

// TrayIconPNG draws the tray icon (a beach umbrella) at size×size pixels
// (pure, deterministic). template=true draws a black silhouette on
// transparent background, the macOS menu bar "template" image that the
// system tints for light and dark mode. Drawn in code: no binary asset to
// keep in sync with packaging/desktop/icons.
func TrayIconPNG(size int, template bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	black := color.NRGBA{0, 0, 0, 0xff}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := (float64(x)+0.5)/s, (float64(y)+0.5)/s
			canopy := fy <= 0.52 && math.Hypot(fx-0.5, (fy-0.52)*1.15) <= 0.42 && fy >= 0.14
			pole := math.Abs(fx-0.5) <= 0.045 && fy > 0.5 && fy <= 0.86
			sand := fy > 0.86 && fy <= 0.94 && fx >= 0.18 && fx <= 0.82
			switch {
			case template && (canopy || pole || sand):
				img.SetNRGBA(x, y, black)
			case template:
			case canopy:
				img.SetNRGBA(x, y, iconSand)
			case pole || sand:
				img.SetNRGBA(x, y, iconPole)
			default:
				img.SetNRGBA(x, y, iconSea)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ICOFromPNG wraps PNG images in an .ico container (PNG-compressed icon
// entries, Windows Vista and later), the format the Windows tray expects.
func ICOFromPNG(pngs ...[]byte) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(pngs))}) //nolint:gosec // G115: a handful of entries
	offset := 6 + 16*len(pngs)
	for _, p := range pngs {
		w, h := 0, 0
		if cfg, err := png.DecodeConfig(bytes.NewReader(p)); err == nil {
			w, h = cfg.Width, cfg.Height
		}
		dim := func(v int) uint8 {
			if v >= 256 {
				return 0
			}
			return uint8(v) //nolint:gosec // G115: checked above
		}
		buf.Write([]byte{dim(w), dim(h), 0, 0})
		_ = binary.Write(&buf, le, [2]uint16{1, 32})
		_ = binary.Write(&buf, le, [2]uint32{uint32(len(p)), uint32(offset)}) //nolint:gosec // G115: icon sizes are tiny
		offset += len(p)
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes()
}
