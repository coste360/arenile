//go:build desktop && cgo && !linux

package remote

import (
	"bytes"
	"context"
	"html"
	"html/template"
	"os"
	"runtime"
	"sync"

	webview "github.com/webview/webview_go"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/desktop/shell"
)

// The native panel window of the remote mode: the system webview (WKWebView
// on macOS, WebView2 on Windows) through github.com/webview/webview_go
// (MIT), compiled only with the "desktop" build tag and CGO on the native
// runner of each OS. Not on Linux: webview_go links WebKitGTK 4.0, which
// Ubuntu 24.04, Debian 13 and Fedora no longer ship (they have 4.1 only),
// and a dynamically linked AppImage would fail in the loader even in local
// mode. The Linux tray build is CGO-free (fyne.io/systray speaks D-Bus in
// pure Go) and the remote mode uses BrowserWindow plus the tray menu
// there; pairing happens with `arenile-desktop -pair`. webview_go locks the
// main goroutine to the main thread in its init; the launcher calls
// Mode.Run on the main goroutine, so Open runs the event loop there.

func init() {
	nativeWindow = func() Window { return webviewWindow{} }
	nativePairing = func() PairingForm { return webviewPairing{} }
}

type webviewWindow struct{}

// Open implements Window. Kiosk prefers a Chromium-family browser in
// --kiosk mode (true full screen, which the webview cannot do) and falls
// back to a large fixed window.
func (webviewWindow) Open(ctx context.Context, o WindowOptions) error {
	if o.Kiosk {
		bw := BrowserWindow{}
		if bin := bw.kioskBrowser(); bin != "" {
			return bw.Open(ctx, o)
		}
	}
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(o.Title)
	var hint webview.Hint = webview.HintNone
	if o.Kiosk {
		hint = webview.HintFixed
	}
	w.SetSize(o.Width, o.Height, hint)
	if o.OpenExternal != nil {
		// Security boundary: see AllowExternal (window.go).
		_ = w.Bind("arenileOpenExternal", func(u string) error {
			if !AllowExternal(o.URL, u) {
				return nil
			}
			return o.OpenExternal(u)
		})
	}
	w.Init(o.Init)
	// Same native layer as the local window (internal/desktop/shell): on
	// macOS the application menu (copy and paste in the panel fields,
	// Cmd+Q), close to the Dock, downloads and printing. "Impostazioni"
	// opens the panel home of the instance (the desktop settings belong to
	// the local mode).
	home, _ := os.UserHomeDir()
	ctl := shell.NewController(shell.Options{
		GOOS: runtime.GOOS, SettingsPath: "/admin/",
		Quit:         func() { w.Dispatch(w.Terminate) },
		OpenBrowser:  o.OpenExternal,
		Notify:       func(t, m string) { _ = shell.Notify(context.Background(), runtime.GOOS, t, m, "") },
		DownloadsDir: shell.DownloadsDir(home),
	})
	ctl.ShowPanel(o.URL)
	win := shell.Adopt(w, ctl)
	defer win.Detach()
	stop := terminateOnDone(ctx, w)
	defer stop()
	w.Run()
	return nil
}

type webviewPairing struct{}

// Ask implements PairingForm with a small HTML form rendered from
// pairingTemplate in the launcher locale (desktop.T). The styling follows
// the panel's look (system font, muted labels, rounded fields, one primary
// button) with the OS light/dark scheme; it is not part of the SPA, so the
// shadcn rule of the web workspace does not apply here.
func (webviewPairing) Ask(ctx context.Context, submit func(PairInput) error) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(desktop.T("menu.pairing_title"))
	w.SetSize(560, 560, webview.HintNone)
	var mu sync.Mutex
	paired := false
	_ = w.Bind("arenilePair", func(qr, url, code, name string) (string, error) {
		if err := submit(PairInput{QR: qr, URL: url, Code: code, DeviceName: name}); err != nil {
			return html.EscapeString(err.Error()), nil
		}
		mu.Lock()
		paired = true
		mu.Unlock()
		w.Dispatch(w.Terminate)
		return "", nil
	})
	w.SetHtml(PairingHTML())
	stop := terminateOnDone(ctx, w)
	defer stop()
	w.Run()
	mu.Lock()
	defer mu.Unlock()
	if !paired {
		return ErrNotPaired
	}
	return nil
}

func terminateOnDone(ctx context.Context, w webview.WebView) (stop func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			w.Dispatch(w.Terminate)
		case <-done:
		}
	}()
	return func() { close(done) }
}

// PairingHTML renders the pairing form in the current locale (pure).
func PairingHTML() string {
	var b bytes.Buffer
	_ = pairingTemplate.Execute(&b, map[string]string{
		"lang": string(desktop.CurrentLocale()), "title": desktop.T("pair.title"), "intro": desktop.T("pair.intro"),
		"url": desktop.T("pair.url"), "code": desktop.T("pair.code"), "qr": desktop.T("pair.qr"),
		"name": desktop.T("pair.name"), "nameValue": desktop.T("pair.name_value"),
		"submit": desktop.T("pair.submit"), "inProgress": desktop.T("pair.in_progress"),
	})
	return b.String()
}

// The heading mark is the Arenile favicon (assets/brand/arenile/arenile-favicon.svg, kit v2.0).
// Colours are the Coste360 tokens of web/shared/src/styles/globals.css (light
// and dark), whose AA contrast is checked by the theme test.
var pairingTemplate = template.Must(template.New("pairing").Parse(`<!doctype html>
<html lang="{{.lang}}"><head><meta charset="utf-8"><meta name="color-scheme" content="light dark">
<title>{{.title}}</title>
<style>
  :root { color-scheme: light dark; --fg: #0f172a; --muted: #475569; --border: #74849b; --bg: #ffffff; --primary: #0b2542; --primary-fg: #ffffff; --ring: #185482; --error: #b91c1c; }
  @media (prefers-color-scheme: dark) { :root { --fg: #f8fafc; --muted: #a7b4c6; --border: #7590b0; --bg: #07192d; --primary: #38bdf8; --primary-fg: #0b2542; --ring: #38bdf8; --error: #fc8888; } }
  html, body { margin: 0; background: var(--bg); color: var(--fg); font: 14px/1.5 -apple-system, "Segoe UI", system-ui, sans-serif; }
  main { max-width: 32rem; margin: 0 auto; padding: 1.5rem 1.5rem 2rem; }
  h1 { display: flex; align-items: center; gap: .6rem; font-size: 1.25rem; font-weight: 600; margin: 0 0 .5rem; }
  h1 svg { width: 2rem; height: 2rem; flex: none; }
  p.intro { color: var(--muted); margin: 0 0 1.25rem; }
  label { display: block; font-weight: 500; margin: 0 0 .25rem; }
  .field { margin-bottom: .9rem; }
  input { box-sizing: border-box; width: 100%; height: 2.25rem; padding: 0 .65rem; border: 1px solid var(--border); border-radius: .5rem; background: transparent; color: inherit; font: inherit; }
  input:focus { outline: 2px solid var(--ring); outline-offset: 1px; }
  button { height: 2.25rem; padding: 0 1rem; border: 0; border-radius: .5rem; background: var(--primary); color: var(--primary-fg); font: inherit; font-weight: 500; cursor: pointer; }
  button:disabled { opacity: .6; cursor: default; }
  #err { min-height: 1.5rem; margin: .75rem 0 0; color: var(--error); }
  #err.busy { color: var(--muted); }
</style></head>
<body><main>
<h1><svg viewBox="0 0 128 128" aria-hidden="true"><rect width="128" height="128" rx="28" fill="#0B2542"/><g transform="translate(9 13) scale(0.15) translate(-143 -177)"><path d="M 282.8 528.6 L 460 186 L 582 186 L 784.9 637 C 765.6 644.2 748.6 641.7 730.4 632.6 C 683.4 607.5 645.4 572.3 603.5 539.9 L 509 349 L 451.2 476.7 C 388.2 473.4 332 494 282.8 528.6 Z" fill="#FFF0D1"/><path d="M 694.5 723.7 C 736.4 726.6 776.1 715.3 807 686.1 L 870.4 827 L 745.6 827 Z" fill="#FFF0D1"/><path d="M 151 827 C 236 717 353 677 445 680 C 555 678 635 748 700 827 Z" fill="#F2C893"/><path d="M 182 701 C 211 620 277 545 345 519 C 411 488 473 495 526 518 C 598 550 648 616 711 647 C 746 668 773 669 807 651 C 772 707 697 717 632 682 C 577 651 538 613 491 601 C 418 572 360 594 306 631 C 260 659 214 692 182 701 Z" fill="#38BDF8"/></g></svg>{{.title}}</h1>
<p class="intro">{{.intro}}</p>
<form id="f" autocomplete="off">
<div class="field"><label for="url">{{.url}}</label><input id="url" name="url" type="url" placeholder="https://lido.example.it"></div>
<div class="field"><label for="code">{{.code}}</label><input id="code" name="code" placeholder="ABCD-EFGH" autocapitalize="characters" spellcheck="false"></div>
<div class="field"><label for="qr">{{.qr}}</label><input id="qr" name="qr" placeholder="arenile://pair?d=…" spellcheck="false"></div>
<div class="field"><label for="name">{{.name}}</label><input id="name" name="name" value="{{.nameValue}}"></div>
<button type="submit">{{.submit}}</button>
<p id="err" role="alert"></p>
</form>
<script>
document.getElementById("f").addEventListener("submit", function (e) {
  e.preventDefault();
  var f = e.target, b = f.querySelector("button"), err = document.getElementById("err");
  b.disabled = true; err.className = "busy"; err.textContent = {{.inProgress}};
  window.arenilePair(f.qr.value, f.url.value, f.code.value, f.name.value).then(function (msg) {
    err.className = ""; err.textContent = msg || ""; b.disabled = false;
  });
});
</script>
</main></body></html>`))
