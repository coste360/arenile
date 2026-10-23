package device

import (
	"fmt"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// maxQRText bounds the text rendered as QR (a pairing QR is ~150 bytes).
const maxQRText = 1024

// RenderQRSVG renders text as a compact SVG QR code: error correction M
// (15%), 4-module quiet zone (ISO/IEC 18004), black modules on a white
// background so that phone cameras read it also in dark mode. Pure.
//
// The same renderer exists in internal/domain/notify (M4, booking QR);
// the integrator may merge the two into one shared helper.
func RenderQRSVG(text string) (string, error) {
	if text == "" || len(text) > maxQRText {
		return "", fmt.Errorf("device: qr text length %d out of range", len(text))
	}
	code, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("device: qr: %w", err)
	}
	return svgFromBitmap(code.Bitmap()), nil
}

// svgFromBitmap draws one path of unit squares merged horizontally per row.
func svgFromBitmap(bm [][]bool) string {
	var path strings.Builder
	for y, row := range bm {
		for x := 0; x < len(row); {
			if !row[x] {
				x++
				continue
			}
			start := x
			for x < len(row) && row[x] {
				x++
			}
			w := strconv.Itoa(x - start)
			path.WriteString("M" + strconv.Itoa(start) + " " + strconv.Itoa(y) + "h" + w + "v1h-" + w + "z")
		}
	}
	n := strconv.Itoa(len(bm))
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + n + ` ` + n +
		`" shape-rendering="crispEdges" role="img"><rect width="` + n + `" height="` + n +
		`" fill="#fff"/><path fill="#000" d="` + path.String() + `"/></svg>`
}
