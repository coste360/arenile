//go:build desktop && cgo

package remote

import (
	"context"
	"html"
	"sync"

	webview "github.com/webview/webview_go"
)

// The native panel window of the remote mode: the system webview (WebKit
// on macOS, WebView2 on Windows, WebKitGTK on Linux) through
// github.com/webview/webview_go (MIT), compiled only with the "desktop"
// build tag and CGO on the native runner of each OS. webview_go locks the
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
		_ = w.Bind("arenileOpenExternal", func(u string) error {
			if !IsExternal(o.URL, u) {
				return nil
			}
			return o.OpenExternal(u)
		})
	}
	w.Init(o.Init)
	w.Navigate(o.URL)
	stop := terminateOnDone(ctx, w)
	defer stop()
	w.Run()
	return nil
}

type webviewPairing struct{}

// Ask implements PairingForm with a plain HTML form (no styling: native
// controls follow the OS theme).
func (webviewPairing) Ask(ctx context.Context, submit func(PairInput) error) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("Arenile — collega questo PC a un'istanza")
	w.SetSize(560, 520, webview.HintNone)
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
	w.SetHtml(pairingHTML)
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

const pairingHTML = `<!doctype html>
<html lang="it"><head><meta charset="utf-8"><meta name="color-scheme" content="light dark">
<title>Collega a un'istanza</title></head>
<body>
<h1>Collega questo PC</h1>
<p>Nel pannello dell'istanza apri <b>Configurazione → Dispositivi</b>, crea un codice di abbinamento e inseriscilo qui insieme all'indirizzo dell'istanza (oppure incolla il testo del QR).</p>
<form id="f">
<p><label>Indirizzo dell'istanza<br><input name="url" type="url" placeholder="https://lido.example.it" size="40"></label></p>
<p><label>Codice di abbinamento<br><input name="code" placeholder="ABCD-EFGH" size="12" autocapitalize="characters"></label></p>
<p><label>Oppure testo del QR<br><input name="qr" placeholder="arenile://pair?d=…" size="40"></label></p>
<p><label>Nome di questo PC<br><input name="name" value="PC cassa" size="30"></label></p>
<p><button type="submit">Collega</button></p>
<p id="err" role="alert"></p>
</form>
<script>
document.getElementById("f").addEventListener("submit", function (e) {
  e.preventDefault();
  var f = e.target, b = f.querySelector("button"), err = document.getElementById("err");
  b.disabled = true; err.textContent = "Collegamento in corso…";
  window.arenilePair(f.qr.value, f.url.value, f.code.value, f.name.value).then(function (msg) {
    err.textContent = msg || ""; b.disabled = false;
  });
});
</script>
</body></html>`
