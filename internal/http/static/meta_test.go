package static

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const indexWithDefaults = `<!doctype html>
<html lang="it">
  <head>
    <meta charset="UTF-8" />
    <meta name="description" content="Prenota ombrellone e lettini online, senza commissioni." />
    <meta property="og:type" content="website" />
    <meta property="og:site_name" content="Arenile" />
    <meta property="og:title" content="Prenota il tuo posto in spiaggia" />
    <meta property="og:image" content="/b/og-image.png" />
    <meta property="og:image:width" content="1200" />
    <meta property="og:image:height" content="630" />
    <meta name="twitter:card" content="summary_large_image" />
    <title>Prenota il tuo posto in spiaggia</title>
    <script type="module" crossorigin src="/b/assets/index.js"></script>
  </head>
  <body><div id="root"></div></body>
</html>`

func TestRenderMeta_EscapesAndReplacesDefaults(t *testing.T) {
	m := PageMeta{
		Title: `Lido "Sole" & <Mare> – Prenota`, Description: `Ombrelloni <script>alert(1)</script> 'fronte' mare`,
		SiteName: "Lido Sole", URL: "https://x.test/b/lido-sole", Image: `https://cdn.test/logo.png?a=1&b="2"`,
		Locale: "de_DE", Lang: "de",
	}
	got := string(renderMeta([]byte(indexWithDefaults), m))
	for _, bad := range []string{"<script>alert", `"Sole"`, "<Mare>", `b="2"`, "Prenota il tuo posto in spiaggia", "/b/og-image.png", "og:image:width"} {
		if strings.Contains(got, bad) {
			t.Errorf("output contains %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{
		`<title>Lido &#34;Sole&#34; &amp; &lt;Mare&gt; – Prenota</title>`,
		`<meta property="og:title" content="Lido &#34;Sole&#34; &amp; &lt;Mare&gt; – Prenota" />`,
		`<meta name="description" content="Ombrelloni &lt;script&gt;alert(1)&lt;/script&gt; &#39;fronte&#39; mare" />`,
		`<meta property="og:image" content="https://cdn.test/logo.png?a=1&amp;b=&#34;2&#34;" />`,
		`<meta property="og:url" content="https://x.test/b/lido-sole" />`,
		`<meta property="og:locale" content="de_DE" />`,
		`<meta name="twitter:card" content="summary" />`,
		`<html lang="de"`,
		`<script type="module" crossorigin src="/b/assets/index.js"></script>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `property="og:title"`); n != 1 {
		t.Errorf("og:title %d times", n)
	}
	if n := strings.Count(got, "<title>"); n != 1 {
		t.Errorf("title %d times", n)
	}
}

func TestRenderMeta_DefaultImageKeepsSize(t *testing.T) {
	got := string(renderMeta([]byte(indexWithDefaults), PageMeta{
		Title: "Lido – Prenota", Image: "https://x.test/b/og-image.png", ImageWidth: 1200, ImageHeight: 630,
	}))
	for _, want := range []string{`og:image:width" content="1200"`, `og:image:height" content="630"`, `twitter:card" content="summary_large_image"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `name="description"`) {
		t.Error("empty description rendered")
	}
}

func TestBooking_ServerRenderedMeta(t *testing.T) {
	booking := fstest.MapFS{
		"index.html":    {Data: []byte(indexWithDefaults)},
		"assets/app.js": {Data: []byte("app()")},
	}
	h := NewFromFS(fstest.MapFS{}, booking)
	var calls []string
	h.SetBookingMeta(func(_ *http.Request, slug string) (PageMeta, bool) {
		calls = append(calls, slug)
		if slug != "lido-sole" {
			return PageMeta{}, false
		}
		return PageMeta{Title: "Lido Sole – Prenota", Description: "Al mare", Image: "https://x.test/logo.png"}, true
	})
	serve := func(path string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.Booking(rec, req)
		return rec
	}

	rec := serve("/b/lido-sole/manage", nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "<title>Lido Sole – Prenota</title>") || strings.Contains(body, "Prenota il tuo posto") {
		t.Fatalf("rendered page %d:\n%s", rec.Code, body)
	}
	etag := rec.Header().Get("ETag")
	if rec.Header().Get("Cache-Control") != "no-cache" || etag == "" {
		t.Fatalf("headers %v", rec.Header())
	}
	if rec := serve("/b/lido-sole", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Fatalf("revalidation = %d", rec.Code)
	}
	// Unknown establishment: the default page, unchanged.
	if rec := serve("/b/sconosciuto", nil); !strings.Contains(rec.Body.String(), "Prenota il tuo posto in spiaggia") {
		t.Fatalf("fallback:\n%s", rec.Body.String())
	}
	// Assets never reach the resolver.
	calls = nil
	if rec := serve("/b/assets/app.js", nil); rec.Body.String() != "app()" || len(calls) != 0 {
		t.Fatalf("asset %q calls %v", rec.Body.String(), calls)
	}
	if rec := serve("/b/assets/missing.js", nil); rec.Code != 404 || len(calls) != 0 {
		t.Fatalf("missing asset %d calls %v", rec.Code, calls)
	}
}

func TestSlugOf_Scenarios(t *testing.T) {
	for in, want := range map[string]string{
		"lido-sole": "lido-sole", "lido-sole/checkout": "lido-sole", "": "", "assets/x.js": "", "icon.svg": "", "a%2Fb": "",
	} {
		if got := slugOf(in); got != want {
			t.Errorf("slugOf(%q) = %q, want %q", in, got, want)
		}
	}
}
