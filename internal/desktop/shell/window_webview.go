//go:build desktop && cgo && !linux

package shell

import (
	"errors"
	"runtime"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

// ErrUnavailable: no system webview on this PC (Windows without the
// WebView2 runtime): the launcher falls back to the browser.
var ErrUnavailable = errors.New("shell: system webview not available")

// Window is the native panel window: webview_go plus the platform layer
// (cocoa_darwin.go, win_windows.go). Create it with Open on the main
// thread, then Run blocks there until Terminate.
type Window struct {
	w webview.WebView
	c *Controller
	p *platform
}

var _ Native = (*Window)(nil)

// WindowOptions configure Open.
type WindowOptions struct {
	Title   string
	Version string
	// Debug enables the web inspector (development builds only).
	Debug bool
}

// Available reports whether the system webview can be used (WebView2
// runtime on Windows; always on macOS).
func Available() bool { return platformAvailable() }

// Open creates the window on the calling (main) thread, applies the
// remembered geometry and zoom, installs the menu (macOS), the close and
// download handling, the bindings and the init script, and attaches the
// controller (which navigates to the panel asked before).
func Open(c *Controller, o WindowOptions) (*Window, error) {
	if !platformAvailable() {
		return nil, ErrUnavailable
	}
	w := webview.New(o.Debug)
	if w == nil {
		return nil, ErrUnavailable
	}
	win := &Window{w: w, c: c}
	w.SetTitle(o.Title)
	st := c.State()
	w.SetSize(MinWidth, MinHeight, webview.HintMin)
	w.SetSize(st.Width, st.Height, webview.HintNone)
	_ = w.Bind("arenileOpenExternal", func(u string) { c.OpenExternal(u) })
	nativePrint := runtime.GOOS != "darwin"
	if !nativePrint {
		_ = w.Bind("arenilePrint", func() { win.p.print() })
	}
	w.Init(InitScript(o.Version, nativePrint))
	win.p = newPlatform(w, c)
	win.p.install(st)
	c.Attach(win)
	return win, nil
}

// Adopt adds the platform layer (application menu, close to hide, Dock
// reopen, downloads, printing) to a webview created elsewhere: the remote
// mode window, which has its own init script and bindings. Call on the main
// thread before Navigate; the controller navigates to the URL given to
// ShowPanel before Adopt.
func Adopt(w webview.WebView, c *Controller) *Window {
	win := &Window{w: w, c: c}
	if ps := PrintScript(runtime.GOOS != "darwin"); ps != "" {
		_ = w.Bind("arenilePrint", func() { win.p.print() })
		w.Init(ps)
	}
	win.p = newPlatform(w, c)
	win.p.install(c.State())
	c.Attach(win)
	return win
}

// Run runs the event loop until Terminate.
func (w *Window) Run() { w.w.Run() }

// Detach removes the platform layer and the controller link (after Run
// returned) without destroying the webview (Adopt's owner does that).
func (w *Window) Detach() {
	w.c.Detach()
	w.p.uninstall()
}

// Destroy releases the window (after Run returned).
func (w *Window) Destroy() {
	w.Detach()
	w.w.Destroy()
}

// Handle is the NSWindow* / HWND.
func (w *Window) Handle() unsafe.Pointer { return w.w.Window() }

// Dispatch runs f on the UI thread.
func (w *Window) Dispatch(f func()) { w.w.Dispatch(f) }

// Navigate implements Native.
func (w *Window) Navigate(u string) { w.w.Dispatch(func() { w.w.Navigate(u) }) }

// Show implements Native.
func (w *Window) Show() { w.w.Dispatch(w.p.show) }

// Hide implements Native.
func (w *Window) Hide() { w.w.Dispatch(w.p.hide) }

// Reload implements Native.
func (w *Window) Reload() { w.w.Dispatch(func() { w.w.Eval("location.reload()") }) }

// SetZoom implements Native.
func (w *Window) SetZoom(level float64) { w.w.Dispatch(func() { w.p.setZoom(level) }) }

// Frame implements Native (UI thread).
func (w *Window) Frame() (WindowState, bool) { return w.p.frame() }

// unloadGrace lets the panel page unload before the event loop stops.
const unloadGrace = 400 * time.Millisecond

// Terminate implements Native: the geometry is saved, the panel page is
// unloaded (about:blank closes its live-update stream, which would
// otherwise keep the internal server's graceful shutdown waiting for its
// whole timeout, the webview being destroyed only at process exit), then
// the event loop ends.
func (w *Window) Terminate() {
	w.w.Dispatch(func() {
		w.c.SaveFrame()
		w.p.hide()
		w.w.Navigate("about:blank")
		time.AfterFunc(unloadGrace, func() { w.w.Dispatch(w.w.Terminate) })
	})
}
