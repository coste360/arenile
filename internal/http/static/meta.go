package static

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Server-rendered social preview of the public page (deferred features F):
// WhatsApp, Facebook, Telegram and the other link unfurlers do not run
// JavaScript, so the establishment name, description and image must be in
// the HTML of /b/{slug}. The SPA (seo.ts) keeps updating them client side.

// PageMeta is the social preview of a public page. Values are plain text:
// the renderer escapes them. Image and URL are absolute.
type PageMeta struct {
	Title       string
	Description string
	SiteName    string
	URL         string
	Image       string
	// ImageWidth/ImageHeight describe the image when known (the Arenile
	// card is 1200x630); 0 omits the tags (establishment logos).
	ImageWidth  int
	ImageHeight int
	// Locale is the og:locale (e.g. it_IT); Lang the <html lang>.
	Locale string
	Lang   string
}

// PageMetaFunc resolves the preview of the public page of slug; false
// keeps the default index.html (unknown slug, archived establishment, DB
// error: the page itself still loads and shows its own error).
type PageMetaFunc func(r *http.Request, slug string) (PageMeta, bool)

// SetBookingMeta installs the resolver of the public page previews. Call
// it once while wiring, before serving.
func (h *Handler) SetBookingMeta(f PageMetaFunc) { h.bookingMeta = f }

// slugOf returns the slug of a /b/{slug}[/...] path ("" when there is none).
func slugOf(rest string) string {
	slug, _, _ := strings.Cut(rest, "/")
	if slug == "" || slug == "assets" || strings.ContainsAny(slug, ".%") {
		return ""
	}
	return slug
}

// metaTagRe matches the head tags replaced by the rendered preview.
var metaTagRe = regexp.MustCompile(`(?is)\s*<meta\s+(?:name|property)="(?:description|og:[a-z:_]+|twitter:[a-z:_]+)"[^>]*>|\s*<title>.*?</title>`)

var htmlLangRe = regexp.MustCompile(`(?i)<html\s+lang="[^"]*"`)

// renderMeta returns index with the preview tags of m in place of the
// default ones (inserted right before </head>).
func renderMeta(index []byte, m PageMeta) []byte {
	out := metaTagRe.ReplaceAll(index, nil)
	var b strings.Builder
	esc := html.EscapeString
	tag := func(attr, key, val string) {
		if val == "" {
			return
		}
		b.WriteString("\n    <meta " + attr + `="` + key + `" content="` + esc(val) + `" />`)
	}
	b.WriteString("\n    <title>" + esc(m.Title) + "</title>")
	tag("name", "description", m.Description)
	tag("property", "og:type", "website")
	tag("property", "og:site_name", m.SiteName)
	tag("property", "og:title", m.Title)
	tag("property", "og:description", m.Description)
	tag("property", "og:url", m.URL)
	tag("property", "og:locale", m.Locale)
	tag("property", "og:image", m.Image)
	if m.ImageWidth > 0 && m.ImageHeight > 0 {
		tag("property", "og:image:width", strconv.Itoa(m.ImageWidth))
		tag("property", "og:image:height", strconv.Itoa(m.ImageHeight))
	}
	card := "summary"
	if m.ImageWidth > m.ImageHeight {
		card = "summary_large_image"
	}
	tag("name", "twitter:card", card)
	tag("name", "twitter:title", m.Title)
	tag("name", "twitter:description", m.Description)
	tag("name", "twitter:image", m.Image)
	b.WriteString("\n  ")
	i := bytes.Index(bytes.ToLower(out), []byte("</head>"))
	if i < 0 {
		return index
	}
	res := make([]byte, 0, len(out)+b.Len())
	res = append(res, bytes.TrimRight(out[:i], " \t\r\n")...)
	res = append(res, b.String()...)
	res = append(res, out[i:]...)
	if m.Lang != "" {
		res = htmlLangRe.ReplaceAll(res, []byte(`<html lang="`+esc(m.Lang)+`"`))
	}
	return res
}

// serveBookingIndex serves the public index with the preview of slug. The
// HTML depends on the establishment, so it is revalidated on every visit
// (no-cache) with a content ETag: unchanged pages answer 304, and a
// deploy never leaves stale asset names in a cache.
func (h *Handler) serveBookingIndex(w http.ResponseWriter, r *http.Request, slug string) bool {
	if h.bookingMeta == nil || slug == "" {
		return false
	}
	index, err := fs.ReadFile(h.booking, "index.html")
	if err != nil {
		return false
	}
	m, ok := h.bookingMeta(r, slug)
	if !ok {
		return false
	}
	body := renderMeta(index, m)
	sum := sha256.Sum256(body)
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("ETag", `"`+hex.EncodeToString(sum[:12])+`"`)
	hd.Add("Vary", "Accept-Encoding")
	// Per-establishment HTML is small: compressed on the fly, not cached
	// in h.gz (which is keyed by file and would grow with the slugs).
	if len(body) >= minGzip && acceptsGzip(r) {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
		_, _ = zw.Write(body)
		_ = zw.Close()
		hd.Set("Content-Encoding", "gzip")
		body = buf.Bytes()
	}
	http.ServeContent(w, r, "index.html", h.modTime, bytes.NewReader(body))
	return true
}
