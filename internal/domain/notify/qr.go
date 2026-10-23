package notify

import (
	"fmt"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// QR formats.
const (
	QRSVG = "svg"
	QRPNG = "png"
)

// QR content types.
const (
	contentTypeSVG = "image/svg+xml"
	contentTypePNG = "image/png"
)

// qrModulePixels is the size of one module in the PNG rendering: a version
// 2-3 symbol (booking codes) becomes ~260-300 px with the quiet zone, crisp
// on high-density screens and small enough to inline in an email.
const qrModulePixels = 8

// maxQRPayload bounds the encoded text (booking payloads are ~25 bytes).
const maxQRPayload = 256

// ErrInvalidQR: unknown format or empty / oversized payload.
var ErrInvalidQR = fmt.Errorf("%w: qr", configerr.ErrValidation)

// QRPayload is the text encoded in the QR of a booking (check-in at the desk).
func QRPayload(bookingCode string) string { return "ARENILE:" + bookingCode }

// QR renders payload as SVG or PNG and returns the content type. Error
// correction level M (15%), 4-module quiet zone (ISO/IEC 18004). It needs
// no state and is safe for concurrent use. Implements checkout.Notifier.
func (s *Service) QR(payload, format string) ([]byte, string, error) {
	return RenderQR(payload, format)
}

// RenderQR is the stateless QR renderer behind Service.QR.
func RenderQR(payload, format string) ([]byte, string, error) {
	if payload == "" || len(payload) > maxQRPayload {
		return nil, "", ErrInvalidQR
	}
	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return nil, "", fmt.Errorf("notify: qr: %w", err)
	}
	switch format {
	case QRSVG, "":
		return qrSVG(code.Bitmap()), contentTypeSVG, nil
	case QRPNG:
		bm := code.Bitmap()
		png, err := code.PNG(len(bm) * qrModulePixels)
		if err != nil {
			return nil, "", fmt.Errorf("notify: qr png: %w", err)
		}
		return png, contentTypePNG, nil
	default:
		return nil, "", ErrInvalidQR
	}
}

// qrSVG renders the module matrix (quiet zone included) as a compact SVG:
// one path of unit squares, merged horizontally per row, on a white
// background so that the code scans also on dark mail clients.
func qrSVG(bm [][]bool) []byte {
	n := len(bm)
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
			path.WriteString("M" + strconv.Itoa(start) + " " + strconv.Itoa(y) + "h" + strconv.Itoa(x-start) + "v1h-" + strconv.Itoa(x-start) + "z")
		}
	}
	size := strconv.Itoa(n)
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + size + ` ` + size + `" shape-rendering="crispEdges" role="img">`)
	b.WriteString(`<rect width="` + size + `" height="` + size + `" fill="#fff"/>`)
	b.WriteString(`<path fill="#000" d="` + path.String() + `"/></svg>`)
	return []byte(b.String())
}
