package static

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler_SPAFallbacks(t *testing.T) {
	admin := fstest.MapFS{
		"index.html":           {Data: []byte("<html>admin</html>")},
		"assets/app-1.js":      {Data: []byte("console.log(1)")},
		"manifest.webmanifest": {Data: []byte("{}")},
	}
	booking := fstest.MapFS{
		"index.html":     {Data: []byte("<html>booking</html>")},
		"assets/main.js": {Data: []byte("b()")},
		"embed.js":       {Data: []byte("embed()")},
	}
	h := NewFromFS(admin, booking)
	tests := []struct {
		name, path, handler string
		status              int
		contains, cache     string
	}{
		{"admin root", "/admin/", "admin", 200, "admin</html>", "no-cache"},
		{"admin deep link", "/admin/bookings/123", "admin", 200, "admin</html>", "no-cache"},
		{"admin asset", "/admin/assets/app-1.js", "admin", 200, "console.log", "immutable"},
		{"admin missing asset", "/admin/assets/missing.js", "admin", 404, "", ""},
		{"admin traversal", "/admin/../../etc/passwd", "admin", 200, "admin</html>", "no-cache"},
		{"booking page", "/b/lido-demo", "booking", 200, "booking</html>", "no-cache"},
		{"booking deep link", "/b/lido-demo/checkout", "booking", 200, "booking</html>", "no-cache"},
		{"booking asset base /b/", "/b/assets/main.js", "booking", 200, "b()", "immutable"},
		{"booking asset relative", "/b/lido-demo/assets/main.js", "booking", 200, "b()", "immutable"},
		{"booking missing asset", "/b/assets/missing.js", "booking", 404, "", ""},
		{"embed", "/embed.js", "embed", 200, "embed()", "max-age=300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			switch tt.handler {
			case "admin":
				h.Admin(w, r)
			case "booking":
				h.Booking(w, r)
			default:
				h.EmbedJS(w, r)
			}
			if w.Code != tt.status {
				t.Fatalf("status %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tt.contains) {
				t.Fatalf("body %q", w.Body.String())
			}
			if tt.cache != "" && !strings.Contains(w.Header().Get("Cache-Control"), tt.cache) {
				t.Fatalf("cache-control %q", w.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestHandler_ManifestContentType(t *testing.T) {
	h := NewFromFS(fstest.MapFS{
		"index.html":           {Data: []byte("<html>admin</html>")},
		"manifest.webmanifest": {Data: []byte("{}")},
	}, fstest.MapFS{})
	r := httptest.NewRequest(http.MethodGet, "/admin/manifest.webmanifest", nil)
	w := httptest.NewRecorder()
	h.Admin(w, r)
	if got := w.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Fatalf("content-type %q", got)
	}
}

func TestHandler_PlaceholderWithoutBuild(t *testing.T) {
	h := New() // embedded dirs contain only .gitkeep in a fresh checkout
	if h.HasAdmin() {
		t.Skip("frontend build embedded")
	}
	for _, p := range []string{"/admin/", "/b/lido-demo"} {
		r := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		if strings.HasPrefix(p, "/admin") {
			h.Admin(w, r)
		} else {
			h.Booking(w, r)
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Arenile è in esecuzione") {
			t.Fatalf("%s: %d %q", p, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.Admin(w, httptest.NewRequest(http.MethodGet, "/admin/.gitkeep", nil))
	if strings.Contains(w.Body.String(), "gitkeep") {
		t.Fatal("dotfiles must not be served")
	}
}

func TestHandler_GzipText(t *testing.T) {
	js := []byte(strings.Repeat("console.log('arenile');\n", 200))
	booking := fstest.MapFS{
		"index.html":       {Data: []byte("<html>" + strings.Repeat("<p>booking</p>", 200) + "</html>")},
		"assets/main.js":   {Data: js},
		"assets/font.woff": {Data: []byte(strings.Repeat("x", 4096))},
		"assets/tiny.js":   {Data: []byte("t()")},
	}
	h := NewFromFS(fstest.MapFS{}, booking)
	tests := []struct {
		name, path, accept string
		gzip               bool
	}{
		{"script with gzip", "/b/assets/main.js", "gzip, deflate, br", true},
		{"index with gzip", "/b/lido-demo", "br;q=1, gzip;q=0.8", true},
		{"gzip refused", "/b/assets/main.js", "gzip;q=0", false},
		{"no Accept-Encoding", "/b/assets/main.js", "", false},
		{"fonts are not recompressed", "/b/assets/font.woff", "gzip", false},
		{"tiny files stay plain", "/b/assets/tiny.js", "gzip", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				r.Header.Set("Accept-Encoding", tt.accept)
			}
			w := httptest.NewRecorder()
			h.Booking(w, r)
			if w.Code != 200 {
				t.Fatalf("status %d", w.Code)
			}
			got := w.Header().Get("Content-Encoding") == "gzip"
			if got != tt.gzip {
				t.Fatalf("gzip %v, want %v", got, tt.gzip)
			}
			if !got {
				return
			}
			zr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			plain, _ := io.ReadAll(zr)
			if tt.path == "/b/assets/main.js" && !bytes.Equal(plain, js) {
				t.Fatal("gzip body does not round-trip")
			}
			if w.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatalf("Vary %q", w.Header().Get("Vary"))
			}
		})
	}
}
