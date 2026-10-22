// Package static serves the embedded single-page applications: the admin
// panel under /admin/, the public booking page under /b/{slug} and the
// embed script at /embed.js. The build copies web/admin/dist and
// web/booking/dist into the admin/ and booking/ directories; when they only
// contain .gitkeep a small placeholder page is served instead.
package static

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:admin all:booking
var files embed.FS

// Handler serves the embedded SPAs.
type Handler struct {
	admin   fs.FS
	booking fs.FS
	modTime time.Time
}

// New returns a Handler over the embedded files.
func New() *Handler {
	a, _ := fs.Sub(files, "admin")
	b, _ := fs.Sub(files, "booking")
	return NewFromFS(a, b)
}

// NewFromFS builds a Handler over arbitrary file systems (tests).
func NewFromFS(admin, booking fs.FS) *Handler {
	return &Handler{admin: admin, booking: booking, modTime: time.Now()}
}

// HasAdmin reports whether a built admin SPA is embedded.
func (h *Handler) HasAdmin() bool { return exists(h.admin, "index.html") }

// HasBooking reports whether a built booking SPA is embedded.
func (h *Handler) HasBooking() bool { return exists(h.booking, "index.html") }

// Admin serves /admin/*: existing files as-is, anything else index.html.
func (h *Handler) Admin(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/admin")
	rel = strings.TrimPrefix(rel, "/")
	h.serveSPA(w, r, h.admin, rel, "admin")
}

// Booking serves /b/{slug} and /b/{slug}/*. Asset paths are looked up both
// relative to /b/ (Vite base "/b/") and relative to /b/{slug}/ (base "./").
func (h *Handler) Booking(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/b/")
	if rel := clean(rest); rel != "" && isFile(h.booking, rel) {
		h.serveFile(w, r, h.booking, rel)
		return
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		if rel := clean(rest[i+1:]); rel != "" && isFile(h.booking, rel) {
			h.serveFile(w, r, h.booking, rel)
			return
		}
	}
	// Missing asset files must 404 instead of returning HTML ("assets" is a
	// reserved slug).
	if strings.HasPrefix(rest, "assets/") {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r, h.booking, "booking")
}

// EmbedJS serves /embed.js.
func (h *Handler) EmbedJS(w http.ResponseWriter, r *http.Request) {
	if !isFile(h.booking, "embed.js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte("/* Arenile embed script not built: run `npm --prefix web run build`. */\n"))
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	h.serveContent(w, r, h.booking, "embed.js")
}

func (h *Handler) serveSPA(w http.ResponseWriter, r *http.Request, fsys fs.FS, rel, app string) {
	if rel = clean(rel); rel != "" && isFile(fsys, rel) {
		h.serveFile(w, r, fsys, rel)
		return
	}
	// Missing asset files must 404 instead of returning HTML.
	if strings.HasPrefix(rel, "assets/") {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r, fsys, app)
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, rel string) {
	if rel == "index.html" {
		h.serveIndex(w, r, fsys, "")
		return
	}
	if strings.HasPrefix(rel, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	h.serveContent(w, r, fsys, rel)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS, app string) {
	w.Header().Set("Cache-Control", "no-cache")
	b, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(placeholder(app)))
		return
	}
	http.ServeContent(w, r, "index.html", h.modTime, bytes.NewReader(b))
}

func (h *Handler) serveContent(w http.ResponseWriter, r *http.Request, fsys fs.FS, rel string) {
	b, err := fs.ReadFile(fsys, rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := contentType(rel); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, path.Base(rel), h.modTime, bytes.NewReader(b))
}

// contentType returns the MIME type for rel, covering PWA files that the
// platform MIME table may not know.
func contentType(rel string) string {
	switch path.Ext(rel) {
	case ".webmanifest":
		return "application/manifest+json"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	}
	return mime.TypeByExtension(path.Ext(rel))
}

func clean(rel string) string {
	if rel == "" {
		return ""
	}
	c := path.Clean("/" + rel)[1:]
	if c == "." || strings.HasPrefix(c, ".") || strings.Contains(c, "/.") {
		return ""
	}
	return c
}

func isFile(fsys fs.FS, rel string) bool {
	st, err := fs.Stat(fsys, rel)
	return err == nil && !st.IsDir()
}

func exists(fsys fs.FS, rel string) bool { return isFile(fsys, rel) }

func placeholder(app string) string {
	title := "Arenile"
	what := "l'interfaccia"
	switch app {
	case "admin":
		title, what = "Arenile — pannello", "il pannello del gestore"
	case "booking":
		title, what = "Arenile — prenotazioni", "la pagina di prenotazione"
	}
	return `<!doctype html><html lang="it"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + title + `</title><style>body{font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem;color:#1f2937}` +
		`code{background:#f3f4f6;padding:.1rem .3rem;border-radius:.25rem}</style></head><body>` +
		`<h1>Arenile è in esecuzione</h1><p>Il server funziona, ma ` + what + ` non è stato incluso in questo binario.</p>` +
		`<p>Compila il frontend con <code>npm --prefix web run build</code> (oppure <code>make build</code>) e ricompila il binario.</p>` +
		`<p>Le API sono disponibili sotto <code>/api/</code>; lo stato del servizio su <code>/healthz</code>.</p></body></html>`
}
