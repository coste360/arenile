package escpos

import (
	"image"
	"image/color"
)

// MaxDots returns the printable width in dots of a paper width in mm
// (384 on 58 mm, 576 otherwise).
func MaxDots(paperWidthMM int) int {
	if paperWidthMM == 58 {
		return MaxDots58
	}
	return MaxDots80
}

// Dither converts img to a 1-bit Raster no wider than maxWidth and no
// taller than maxHeight dots (aspect ratio kept, never upscaled), with
// Floyd-Steinberg error diffusion. Transparent pixels are white. The width
// is padded with white to a multiple of 8. A nil or empty image gives the
// zero Raster.
func Dither(img image.Image, maxWidth, maxHeight int) Raster {
	if img == nil {
		return Raster{}
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || maxWidth < 8 || maxHeight < 1 {
		return Raster{}
	}
	w, h := sw, sh
	if w > maxWidth {
		h = max(1, h*maxWidth/w)
		w = maxWidth
	}
	if h > maxHeight {
		w = max(1, w*maxHeight/h)
		h = maxHeight
	}
	// Grayscale (0 = black, 255 = white) by box sampling of the source.
	gray := make([]float64, w*h)
	for y := range h {
		y0, y1 := b.Min.Y+y*sh/h, b.Min.Y+max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := b.Min.X+x*sw/w, b.Min.X+max((x+1)*sw/w, x*sw/w+1)
			var sum float64
			var n int
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					sum += luminance(img.At(sx, sy))
					n++
				}
			}
			gray[y*w+x] = sum / float64(n)
		}
	}
	widthBytes := (w + 7) / 8
	data := make([]byte, widthBytes*h)
	for y := range h {
		for x := range w {
			i := y*w + x
			old := gray[i]
			var nv float64
			if old < 128 {
				data[y*widthBytes+x/8] |= 0x80 >> (x % 8)
			} else {
				nv = 255
			}
			e := old - nv
			if x+1 < w {
				gray[i+1] += e * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					gray[i+w-1] += e * 3 / 16
				}
				gray[i+w] += e * 5 / 16
				if x+1 < w {
					gray[i+w+1] += e * 1 / 16
				}
			}
		}
	}
	return Raster{WidthBytes: widthBytes, Height: h, Data: data}
}

// luminance is the gray level of c composited over white (0-255).
func luminance(c color.Color) float64 {
	r, g, b, a := c.RGBA() // premultiplied, 0-65535
	white := float64(0xffff - a)
	lum := 0.299*(float64(r)+white) + 0.587*(float64(g)+white) + 0.114*(float64(b)+white)
	return lum / 257
}

// EncodeRaster serializes r for storage: 2 bytes width, 2 bytes height
// (big endian), then the data.
func EncodeRaster(r Raster) []byte {
	if r.WidthBytes <= 0 || r.Height <= 0 || len(r.Data) != r.WidthBytes*r.Height {
		return nil
	}
	out := make([]byte, 4, 4+len(r.Data))
	out[0], out[1] = byte(r.WidthBytes>>8), byte(r.WidthBytes)
	out[2], out[3] = byte(r.Height>>8), byte(r.Height)
	return append(out, r.Data...)
}

// DecodeRaster reverses EncodeRaster; ok is false for malformed input.
func DecodeRaster(b []byte) (Raster, bool) {
	if len(b) < 4 {
		return Raster{}, false
	}
	wb, h := int(b[0])<<8|int(b[1]), int(b[2])<<8|int(b[3])
	if wb <= 0 || h <= 0 || len(b)-4 != wb*h {
		return Raster{}, false
	}
	return Raster{WidthBytes: wb, Height: h, Data: b[4:]}, true
}
