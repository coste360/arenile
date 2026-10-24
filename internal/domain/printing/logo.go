package printing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // logo formats
	_ "image/jpeg" // logo formats
	_ "image/png"  // logo formats
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/escpos"
	"github.com/coste360/arenile/internal/store"
)

// LogoLoader fetches the establishment logo image from its theme URL.
type LogoLoader interface {
	LoadLogo(ctx context.Context, logoURL string) (image.Image, error)
}

// Logo raster bounds: two thirds of the printable width, 160 dots high.
const maxLogoHeight = 160

func logoMaxWidth(paperWidthMM int) int { return escpos.MaxDots(paperWidthMM) * 2 / 3 / 8 * 8 }

// logo returns the cached raster of the establishment logo for pr,
// refreshing the cache when the logo URL or the paper width changed. Any
// failure prints without the logo (logged, never fatal).
func (s *Service) logo(ctx context.Context, pr *store.Printer, est *store.Establishment) *escpos.Raster {
	src := strings.TrimSpace(est.Theme.LogoURL)
	if src == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(src + "|" + strconv.Itoa(pr.PaperWidthMM)))
	key := hex.EncodeToString(sum[:])
	if pr.LogoSourceSHA256 != nil && *pr.LogoSourceSHA256 == key {
		if r, ok := escpos.DecodeRaster(pr.LogoRaster); ok {
			return &r
		}
	}
	if s.d.Logos == nil {
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	img, err := s.d.Logos.LoadLogo(lctx, src)
	if err != nil {
		s.logError(ctx, "printer logo not loaded", pr.ID, err)
		return nil
	}
	r := escpos.Dither(img, logoMaxWidth(pr.PaperWidthMM), maxLogoHeight)
	enc := escpos.EncodeRaster(r)
	if enc == nil {
		return nil
	}
	if err := store.SetPrinterLogo(ctx, s.d.DB, pr.ID, enc, &key); err != nil {
		s.logError(ctx, "printer logo not cached", pr.ID, err)
	} else {
		pr.LogoRaster, pr.LogoSourceSHA256 = enc, &key
	}
	return &r
}

// HTTPLogoLoader fetches logos over HTTP(S): absolute https:// URLs, or
// paths starting with "/" resolved against BaseURL (the instance itself).
// At most 2 MiB of PNG, JPEG or GIF.
type HTTPLogoLoader struct {
	Client  *http.Client
	BaseURL string
}

// maxLogoBytes bounds the downloaded logo.
const maxLogoBytes = 2 << 20

var errLogoURL = errors.New("printing: unsupported logo url")

// LoadLogo implements LogoLoader.
func (l HTTPLogoLoader) LoadLogo(ctx context.Context, logoURL string) (image.Image, error) {
	target := logoURL
	switch {
	case strings.HasPrefix(logoURL, "https://"):
	case strings.HasPrefix(logoURL, "/") && !strings.HasPrefix(logoURL, "//") && l.BaseURL != "":
		base, err := url.Parse(l.BaseURL)
		if err != nil {
			return nil, errLogoURL
		}
		ref, err := url.Parse(logoURL)
		if err != nil {
			return nil, errLogoURL
		}
		target = base.ResolveReference(ref).String()
	default:
		return nil, errLogoURL
	}
	c := l.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, errLogoURL
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("printing: fetch logo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("printing: fetch logo: status %d", resp.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, maxLogoBytes))
	if err != nil {
		return nil, fmt.Errorf("printing: decode logo: %w", err)
	}
	return img, nil
}
