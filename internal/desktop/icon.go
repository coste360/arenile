package desktop

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"image/png"
)

// Tray icons from the Arenile brand kit (assets/brand/arenile/png, rendered by
// scripts/brand/build-brand.mjs): the mono "A" in black for the macOS menu bar
// template image, the app tile in colour for the Windows and Linux trays.
//
//go:embed trayicons/*.png
var trayIcons embed.FS

var (
	trayTemplateSizes = []int{18, 36}
	trayColorSizes    = []int{16, 32, 64}
)

// TrayIconPNG returns the tray icon closest to size×size pixels (the smallest
// embedded rendering at least that large, else the largest one); pure and
// deterministic. template=true returns the black silhouette on transparent
// background, the macOS menu bar "template" image that the system tints for
// light and dark mode.
func TrayIconPNG(size int, template bool) []byte {
	kind, sizes := "color", trayColorSizes
	if template {
		kind, sizes = "template", trayTemplateSizes
	}
	pick := sizes[len(sizes)-1]
	for _, s := range sizes {
		if s >= size {
			pick = s
			break
		}
	}
	b, err := trayIcons.ReadFile(fmt.Sprintf("trayicons/tray-%s-%d.png", kind, pick))
	if err != nil {
		panic(err) // embedded at build time: unreachable
	}
	return b
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
