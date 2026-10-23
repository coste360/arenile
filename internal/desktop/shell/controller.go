package shell

import (
	"log/slog"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/coste360/arenile/internal/desktop"
)

// Native is the port to the platform window (window_webview.go). Every
// method may be called from any goroutine: implementations hop to the UI
// thread themselves.
type Native interface {
	Navigate(url string)
	// Show unhides, restores and focuses the window (and activates the app).
	Show()
	// Hide hides the window; the app keeps running (tray, Dock).
	Hide()
	Reload()
	SetZoom(level float64)
	// Frame returns the current geometry (UI thread only); ok is false
	// when the platform cannot tell.
	Frame() (WindowState, bool)
	// Terminate ends the window event loop.
	Terminate()
}

// CloseBehaviour is what closing the window does.
type CloseBehaviour int

// Close behaviours.
const (
	// CloseHides keeps the app running (tray / menu bar, Dock on macOS).
	CloseHides CloseBehaviour = iota
	// CloseQuits quits the app (exit backup, LAN mode stopped).
	CloseQuits
)

// CloseAction decides what closing the window does (pure). macOS follows
// the platform convention: the app stays in the menu bar and in the Dock,
// Cmd+Q / Esci quits. Elsewhere (Windows) closing the window quits, unless
// the operator enabled "keep running in tray" (keep_running_in_tray, e.g.
// a cash desk PC serving the LAN mode to the staff) and a tray icon exists
// to bring it back.
func CloseAction(goos string, keepInTray, hasTray bool) CloseBehaviour {
	if goos == "darwin" {
		return CloseHides
	}
	if keepInTray && hasTray {
		return CloseHides
	}
	return CloseQuits
}

// Options configure a Controller.
type Options struct {
	GOOS    string
	DataDir string
	Version string
	// HasTray: a tray icon can show the window again.
	HasTray bool
	// KeepInTray reads the desktop setting keep_running_in_tray.
	KeepInTray func() bool
	// Quit quits the app (the launcher's shutdown runs after the window
	// event loop returns).
	Quit func()
	// OpenBrowser opens a web link in the default browser (desktop.OpenURL).
	OpenBrowser func(url string) error
	// Notify shows a system notification.
	Notify func(title, message string)
	// DownloadsDir is where downloads are saved (DownloadsDir(home)).
	DownloadsDir string
	// SettingsPath is the panel path of "Impostazioni" (desktop settings).
	SettingsPath string
	Log          *slog.Logger
}

// Controller is the behaviour of the panel window over Native (functional
// core of the window UI, tested with a fake Native): the URL shown, the
// close behaviour, the menu commands, the window state file, downloads and
// external links.
type Controller struct {
	o Options

	mu      sync.Mutex
	native  Native
	current string // last URL navigated to (the SPA may have moved since)
	pending string // URL asked before the window existed
	state   WindowState
}

// NewController returns a Controller; the window state is loaded from the
// data dir.
func NewController(o Options) *Controller {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.SettingsPath == "" {
		o.SettingsPath = "/admin/desktop"
	}
	return &Controller{o: o, state: LoadState(o.DataDir)}
}

// State returns the window state to apply when the window is created.
func (c *Controller) State() WindowState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Attach connects the native window and navigates to the URL asked before
// it existed. It returns that URL ("" if none yet).
func (c *Controller) Attach(n Native) string {
	c.mu.Lock()
	c.native = n
	u := c.pending
	c.pending = ""
	if u != "" {
		c.current = u
	}
	c.mu.Unlock()
	if u != "" {
		n.Navigate(u)
	}
	return u
}

// Detach disconnects the native window (after its event loop returned).
func (c *Controller) Detach() {
	c.mu.Lock()
	c.native = nil
	c.mu.Unlock()
}

// ShowPanel shows target in the window: a URL already shown only brings
// the window to the front (the operator's page in the SPA is kept); a new
// one (first run, LAN QR page, settings) navigates. Before the window
// exists the URL is remembered for Attach.
func (c *Controller) ShowPanel(target string) {
	c.mu.Lock()
	n := c.native
	if n == nil {
		c.pending = target
		c.mu.Unlock()
		return
	}
	same := target == c.current
	c.current = target
	c.mu.Unlock()
	if !same {
		n.Navigate(target)
	}
	n.Show()
}

// Current is the URL last navigated to.
func (c *Controller) Current() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == "" {
		return c.pending
	}
	return c.current
}

// OnClose handles the window's close button (and Cmd+W): it saves the
// geometry, then hides the window or quits (CloseAction). The native
// window is never destroyed by the close itself: the event loop ends only
// through Terminate, after the state is saved.
func (c *Controller) OnClose() CloseBehaviour {
	c.SaveFrame()
	keep := c.o.KeepInTray != nil && c.o.KeepInTray()
	b := CloseAction(c.o.GOOS, keep, c.o.HasTray)
	switch b {
	case CloseHides:
		if n := c.nativeWindow(); n != nil {
			n.Hide()
		}
	case CloseQuits:
		c.quit()
	}
	return b
}

// OnReopen handles a click on the Dock icon (or a second launch) while the
// window is hidden.
func (c *Controller) OnReopen() {
	if n := c.nativeWindow(); n != nil {
		n.Show()
	}
}

// Commands wires the menu actions to the controller.
func (c *Controller) Commands() Commands {
	return Commands{
		Quit: c.quit,
		OpenSettings: func() {
			if o := c.origin(); o != "" {
				c.ShowPanel(o + c.o.SettingsPath)
			}
		},
		OpenInBrowser: func() { c.openBrowser(c.Current()) },
		Reload: func() {
			if n := c.nativeWindow(); n != nil {
				n.Reload()
			}
		},
		Zoom: c.Zoom,
	}
}

// OnMenuAction runs a Go-handled menu action (from the native menu).
func (c *Controller) OnMenuAction(a Action) {
	if !c.Commands().Dispatch(a) {
		c.o.Log.Warn("desktop window: unknown menu action", slog.String("action", string(a)))
	}
}

// Zoom changes the page zoom by one step (dir>0 in, <0 out, 0 reset) and
// remembers it.
func (c *Controller) Zoom(dir int) {
	c.mu.Lock()
	c.state.Zoom = NextZoom(c.state.Zoom, dir)
	z := c.state.Zoom
	n := c.native
	s := c.state
	c.mu.Unlock()
	if n != nil {
		n.SetZoom(z)
	}
	c.save(s)
}

// SaveFrame reads the geometry from the native window (UI thread) and
// writes window.json.
func (c *Controller) SaveFrame() {
	n := c.nativeWindow()
	if n == nil {
		return
	}
	f, ok := n.Frame()
	if !ok {
		return
	}
	c.mu.Lock()
	f.Zoom = c.state.Zoom
	c.state = f.Normalize()
	s := c.state
	c.mu.Unlock()
	c.save(s)
}

func (c *Controller) save(s WindowState) {
	if c.o.DataDir == "" {
		return
	}
	if err := SaveState(c.o.DataDir, s); err != nil {
		c.o.Log.Warn("desktop window state not saved", slog.String("error", err.Error()))
	}
}

// OpenExternal is the arenileOpenExternal binding and the native handler of
// links to other origins: AllowExternal decides, then the default browser.
func (c *Controller) OpenExternal(target string) {
	if !AllowExternal(c.Current(), target) {
		c.o.Log.Warn("desktop window: external link refused")
		return
	}
	c.openBrowser(target)
}

// DownloadDestination chooses the file of a download (native callback):
// the sanitized suggested name in the downloads folder, never overwriting.
func (c *Controller) DownloadDestination(suggested string) (string, error) {
	return DownloadPath(c.o.DownloadsDir, suggested)
}

// DownloadFinished notifies the operator of a finished or failed download.
// The message names the file only (never the full path in a log line).
func (c *Controller) DownloadFinished(path string, failed bool) {
	name := filepath.Base(path)
	if failed {
		c.o.Log.Warn("desktop window: download failed", slog.String("file", name))
		c.notify(desktop.T("notify.download_fail", name))
		return
	}
	c.o.Log.Info("desktop window: download saved", slog.String("file", name))
	c.notify(desktop.T("notify.download_ok", name))
}

func (c *Controller) notify(msg string) {
	if c.o.Notify != nil {
		c.o.Notify("Arenile", msg)
	}
}

func (c *Controller) openBrowser(u string) {
	if u == "" || c.o.OpenBrowser == nil {
		return
	}
	if err := c.o.OpenBrowser(u); err != nil {
		c.o.Log.Warn("desktop window: browser not opened", slog.String("error", err.Error()))
	}
}

func (c *Controller) quit() {
	if c.o.Quit != nil {
		c.o.Quit()
	}
}

func (c *Controller) nativeWindow() Native {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.native
}

// origin is scheme://host of the panel URL.
func (c *Controller) origin() string {
	u, err := url.Parse(c.Current())
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
