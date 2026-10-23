package notify

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/store"
)

// Establishment branding of the emails (spec 12.5 theme): logo, primary
// colour with a text colour computed for WCAG AA contrast (spec 12.6), and
// a link colour readable on the white body. Pure functions.

// DefaultPrimaryColor is used when the theme has no valid primary colour:
// the Coste360 navy, the primary of the admin theme
// (web/shared/src/styles/globals.css --primary); white text on it is AAA.
const DefaultPrimaryColor = "#0B2542"

// Branding is the visual identity of one establishment in an email.
type Branding struct {
	Name      string
	LogoURL   string
	Primary   string // #RRGGBB, header and button background
	OnPrimary string // #FFFFFF or #111111, text on Primary
	Link      string // #RRGGBB, links on white with contrast >= 4.5
}

// BrandingFor builds the branding of an establishment from its theme.
// Only https logo URLs are used (no mixed content, no tracking schemes).
func BrandingFor(est *store.Establishment) Branding {
	primary := normalizeHex(est.Theme.PrimaryColor)
	if primary == "" {
		primary = DefaultPrimaryColor
	}
	b := Branding{Name: est.Name, Primary: primary, OnPrimary: TextColorOn(primary), Link: ReadableOnWhite(primary)}
	if strings.HasPrefix(est.Theme.LogoURL, "https://") {
		b.LogoURL = est.Theme.LogoURL
	}
	return b
}

// normalizeHex returns "#RRGGBB" upper-case, or "" when s is not a colour.
func normalizeHex(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 4 && s[0] == '#' { // #RGB
		s = "#" + strings.Repeat(s[1:2], 2) + strings.Repeat(s[2:3], 2) + strings.Repeat(s[3:4], 2)
	}
	if len(s) != 7 || s[0] != '#' {
		return ""
	}
	if _, err := strconv.ParseUint(s[1:], 16, 32); err != nil {
		return ""
	}
	return strings.ToUpper(s)
}

func parseRGB(hex string) (r, g, b float64) {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return float64(v >> 16 & 0xFF), float64(v >> 8 & 0xFF), float64(v & 0xFF)
}

// RelativeLuminance is the WCAG 2.x relative luminance of #RRGGBB.
func RelativeLuminance(hex string) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := parseRGB(hex)
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// ContrastRatio is the WCAG contrast ratio between two colours (1..21).
func ContrastRatio(a, b string) float64 {
	la, lb := RelativeLuminance(a), RelativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TextColorOn picks white or near-black text for background bg, whichever
// contrasts more (spec 12.6: AA with any primary colour).
func TextColorOn(bg string) string {
	const white, dark = "#FFFFFF", "#111111"
	if ContrastRatio(bg, white) >= ContrastRatio(bg, dark) {
		return white
	}
	return dark
}

// ReadableOnWhite darkens c until it reaches a 4.5:1 contrast on white
// (links in the email body keep the brand hue).
func ReadableOnWhite(c string) string {
	r, g, b := parseRGB(c)
	for range 40 {
		hex := fmt.Sprintf("#%02X%02X%02X", int(r), int(g), int(b))
		if ContrastRatio(hex, "#FFFFFF") >= 4.5 {
			return hex
		}
		r, g, b = r*0.9, g*0.9, b*0.9
	}
	return "#111111"
}
