//go:build desktop && cgo && !linux

package main

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/desktop/shell"
)

// uiKind "window": the panel in the app's own window (WKWebView on macOS,
// WebView2 on Windows) plus the tray icon (spec 29.1, local and LAN mode).
const uiKind = "window"

// newUI returns the window UI, or the tray UI (browser) with -browser,
// -no-browser or -no-tray, or when the system webview is missing
// (Windows without the WebView2 runtime).
func newUI(log *slog.Logger, o uiOptions) desktop.UI {
	if o.noTray || !o.browser || o.inBrowser || !shell.Available() {
		if !o.noTray && runtime.GOOS == "darwin" {
			shell.SetAccessoryActivation() // menu bar only: no Dock icon without a window
		}
		return newTrayUI(log, o)
	}
	tray := &trayUI{headlessUI: newHeadlessUI(log, true)}
	u := &windowUI{trayUI: tray, log: log, version: o.version}
	home, _ := os.UserHomeDir()
	u.ctrl = shell.NewController(shell.Options{
		GOOS: runtime.GOOS, DataDir: o.dataDir, Version: o.version, HasTray: true,
		KeepInTray: func() bool {
			s, err := desktop.LoadSettings(o.dataDir)
			return err == nil && s.KeepRunningInTray
		},
		Quit:         u.Quit,
		OpenBrowser:  u.OpenInBrowser,
		Notify:       tray.Notify,
		DownloadsDir: shell.DownloadsDir(home),
		// App menu "Verifica aggiornamenti": the launcher's tray entry.
		CheckUpdates: func() { tray.menuAction(desktop.MenuUpdates) },
		Log:          log,
	})
	// A click on a native notification (macOS) shows the window at its
	// page ("Versione e aggiornamenti" for a new version), or just the
	// window; the browser when the window could not be created.
	shell.SetNotificationHandler(func(page string) {
		u.mu.Lock()
		fb := u.fallback
		u.mu.Unlock()
		if fb {
			if v, ok := desktop.RebasePanelPage(u.ctrl.Current(), page); ok {
				_ = u.OpenInBrowser(v)
			}
			return
		}
		u.ctrl.OnNotificationClick(page)
	})
	shell.InitNotifications()
	return u
}

// windowUI implements desktop.UI with the panel window and the tray menu.
// Run must be called on the main thread (webview_go and AppKit). Before
// Run, OpenURL only remembers the panel URL (the launcher opens it before
// the event loop starts).
type windowUI struct {
	*trayUI

	log     *slog.Logger
	version string
	ctrl    *shell.Controller

	mu       sync.Mutex
	fallback bool // window not available: browser
}

var (
	_ desktop.UI            = (*windowUI)(nil)
	_ desktop.BrowserOpener = (*windowUI)(nil)
)

// Run opens the window, adds the tray icon and runs the event loop until
// Quit or ctx is done.
func (u *windowUI) Run(ctx context.Context, menu []desktop.MenuItem) error {
	win, err := shell.Open(u.ctrl, shell.WindowOptions{Title: "Arenile", Version: u.version})
	if err != nil {
		u.log.Warn("desktop window not available, using the browser", slog.String("error", err.Error()))
		u.mu.Lock()
		u.fallback = true
		u.mu.Unlock()
		if cur := u.ctrl.Current(); cur != "" {
			_ = u.OpenInBrowser(cur)
		}
		return u.trayUI.Run(ctx, menu)
	}
	stopTray := u.startExternal(menu)
	stop := context.AfterFunc(ctx, u.Quit)
	defer stop()
	go func() {
		<-u.quit
		u.stop()
		win.Terminate()
	}()
	win.Show()
	win.Run()
	win.Destroy()
	stopTray()
	return nil
}

// OpenURL shows the panel in the window (navigating only to a new URL).
func (u *windowUI) OpenURL(url string) error {
	u.mu.Lock()
	fb := u.fallback
	u.mu.Unlock()
	if fb {
		return u.OpenInBrowser(url)
	}
	u.log.Info("arenile desktop: panel", slog.String("url", stripFragment(url)))
	u.ctrl.ShowPanel(url)
	return nil
}

// OpenInBrowser implements desktop.BrowserOpener ("Apri nel browser").
func (u *windowUI) OpenInBrowser(url string) error {
	return desktop.OpenURL(context.Background(), runtime.GOOS, url)
}

// SetMenu implements desktop.UI (tray menu).
func (u *windowUI) SetMenu(menu []desktop.MenuItem) { u.trayUI.SetMenu(menu) }

// Quit implements desktop.UI: the window loop ends after saving the
// geometry; the launcher then stops the server and makes the exit backup.
func (u *windowUI) Quit() { u.headlessUI.Quit() }
