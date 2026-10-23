package desktop

import (
	"context"
	"log/slog"
)

// Mode is the desktop app mode chosen at first start (spec 29.1).
type Mode string

// Modes.
const (
	// ModeLocal runs the internal server (all-in-one, no internet needed).
	ModeLocal Mode = "local"
	// ModeRemote connects to an existing instance with a device token and
	// runs the fiscal bridge agent; no internal server.
	ModeRemote Mode = "remote"
)

// MenuItem is an entry of the tray menu (labels are Italian source strings
// resolved by the UI through its own i18n).
type MenuItem struct {
	ID      string // "open", "backup", "data_dir", "autostart", "lan", "restore", "quit"
	Label   string
	Checked bool
	OnClick func()
}

// UI is the port to the system tray / window (adapter in cmd/arenile-desktop:
// systray + webview behind the "desktop" build tag, a headless adapter
// otherwise).
type UI interface {
	// Run shows the tray icon with the menu and blocks until Quit or ctx is done.
	Run(ctx context.Context, menu []MenuItem) error
	// SetMenu replaces the menu (e.g. after toggling LAN mode).
	SetMenu(menu []MenuItem)
	// OpenURL opens the default browser (local mode) or the app window
	// (remote mode, kiosk optional).
	OpenURL(url string) error
	// OpenFolder opens a folder in the file manager.
	OpenFolder(path string) error
	// Notify shows a system notification.
	Notify(title, message string)
	// Quit ends Run.
	Quit()
}

// Options configure the Launcher.
type Options struct {
	Version string
	// DataDir overrides the per-OS data directory (tests, portable installs).
	DataDir string
	Log     *slog.Logger
}

// Launcher runs the desktop edition:
//
//	local:  data dir -> secret key (keyring) -> config (127.0.0.1:0,
//	        SQLite, auto-migrate) -> app.New(cfg, app.WithDesktop(runtime))
//	        -> serve -> open /admin/ (or /admin/first-run#token=... when no
//	        users exist) -> tray menu, daily/exit backups, LAN listener on demand,
//	        restore on restart.
//	remote: device token from the keyring (or pairing from a QR / code) ->
//	        window on the instance URL -> fiscalbridge.Agent with the local
//	        FiscalDriver.
type Launcher struct{ o Options }

// NewLauncher returns a Launcher.
func NewLauncher(o Options) *Launcher {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return &Launcher{o: o}
}

// Run blocks until the user quits from the UI or ctx is cancelled.
func (l *Launcher) Run(ctx context.Context, ui UI) error {
	return ErrNotImplemented
}
