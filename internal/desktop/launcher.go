package desktop

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coste360/arenile/internal/app"
	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/store"
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

// Tray menu item identifiers (MenuItem.ID).
const (
	MenuOpen      = "open"
	MenuBackup    = "backup"
	MenuDataDir   = "data_dir"
	MenuAutostart = "autostart"
	MenuLAN       = "lan"
	MenuLANQR     = "lan_qr"
	MenuRestore   = "restore"
	MenuQuit      = "quit"
)

// MenuItem is an entry of the tray menu (labels are Italian, the source
// language; the tray has no locale negotiation yet).
type MenuItem struct {
	ID       string // one of the Menu* constants
	Label    string
	Checked  bool
	Checkbox bool
	Disabled bool
	OnClick  func()
}

// UI is the port to the system tray / window (adapter in cmd/arenile-desktop:
// systray behind the "desktop" build tag, a headless adapter otherwise).
type UI interface {
	// Run shows the tray icon with the menu and blocks until Quit or ctx is
	// done. The launcher calls it on the goroutine that called Launcher.Run
	// (the main thread: required by the macOS status bar).
	Run(ctx context.Context, menu []MenuItem) error
	// SetMenu replaces the menu (e.g. after toggling LAN mode).
	SetMenu(menu []MenuItem)
	// OpenURL opens the default browser (local mode) or the app window
	// (remote mode, kiosk optional). It may be called before Run.
	OpenURL(url string) error
	// OpenFolder opens a folder in the file manager.
	OpenFolder(path string) error
	// Notify shows a system notification.
	Notify(title, message string)
	// Quit ends Run.
	Quit()
}

// LANDeps are given to the LAN adapter factory (Options.NewLAN).
type LANDeps struct {
	DataDir string
	// Serve serves the panel of the running app on ln until stop is called
	// or the app stops (App.ServeListener of the current app instance).
	Serve func(ln net.Listener) (stop func(context.Context) error)
	Log   *slog.Logger
}

// RemoteMode is the port to the remote mode (internal/desktop/remote): when
// the operator chose it, the launcher runs it instead of the local server.
type RemoteMode interface {
	Configured(dataDir string) bool
	Run(ctx context.Context, ui UI, dataDir string, log *slog.Logger) error
}

// StartInfo describes a started local server (Options.OnReady, tests).
type StartInfo struct {
	DataDir string
	BaseURL string
	// OpenURL is the page opened in the browser: /admin/ or, while no
	// owner exists, /admin/first-run#token=<one-time token>.
	OpenURL string
	Runtime *Runtime
}

// Options configure the Launcher.
type Options struct {
	Version string
	// DataDir overrides the per-OS data directory (tests, portable installs).
	DataDir string
	// Log overrides the logger (tests). By default the launcher logs JSON to
	// <data dir>/logs/arenile-desktop.log (rotated) and to Console.
	Log     *slog.Logger
	Console io.Writer
	// GOOS overrides runtime.GOOS (tests).
	GOOS  string
	Clock clock.Clock
	// SecretStores override the keyring + file chain (tests).
	SecretStores []SecretStore
	// Autostart overrides the per-OS autostart strategy (tests).
	Autostart Autostart
	// NewLAN builds the LAN mode adapter (internal/desktop/lan); nil hides
	// the LAN menu entry.
	NewLAN func(LANDeps) LANMode
	// Remote is the remote mode (internal/desktop/remote), optional.
	Remote RemoteMode
	// UpdateURL and HTTPClient configure the opt-in update check.
	UpdateURL  string
	HTTPClient *http.Client
	// ScheduleTick and UpdateTick drive the background loops (tests).
	ScheduleTick <-chan time.Time
	UpdateTick   <-chan time.Time
	// OnReady is called once the local server listens (tests).
	OnReady func(StartInfo)
}

// Launcher runs the desktop edition:
//
//	local:  data dir -> instance lock -> secret key (keyring, file fallback)
//	        -> config (127.0.0.1:<free port>, SQLite, auto-migrate)
//	        -> app.New(cfg, app.WithDesktop(runtime)) -> serve -> open
//	        /admin/ (or /admin/first-run#token=... when no users exist)
//	        -> tray menu, daily/exit backups, LAN listener on demand,
//	        restore = stop, replace the database, start again.
//	remote: delegated to Options.Remote.
type Launcher struct {
	o Options

	log     *slog.Logger
	dataDir string
	ui      UI
	rt      *Runtime
	cfg     config.Config

	mu       sync.Mutex
	srv      *server
	setupURL string // first-run URL while no owner exists
	fatal    error
}

// NewLauncher returns a Launcher.
func NewLauncher(o Options) *Launcher {
	if o.GOOS == "" {
		o.GOOS = goruntime.GOOS
	}
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	if o.UpdateURL == "" {
		o.UpdateURL = DefaultUpdateURL
	}
	return &Launcher{o: o}
}

// Run blocks until the user quits from the UI or ctx is cancelled. A
// second launch on the same data directory opens the panel of the running
// one and returns nil.
func (l *Launcher) Run(ctx context.Context, ui UI) error {
	dataDir, err := l.resolveDataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("desktop: data dir: %w", err)
	}
	log, closeLog, err := l.logger(dataDir)
	if err != nil {
		return err
	}
	defer closeLog()
	l.log, l.dataDir, l.ui = log, dataDir, ui

	lock, err := AcquireInstanceLock(dataDir)
	if errors.Is(err, ErrAlreadyRunning) {
		return l.activateRunning(ctx)
	}
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	if l.o.Remote != nil && l.o.Remote.Configured(dataDir) {
		log.Info("arenile desktop: remote mode")
		return l.o.Remote.Run(ctx, ui, dataDir, log)
	}
	return l.runLocal(ctx)
}

func (l *Launcher) resolveDataDir() (string, error) {
	if l.o.DataDir != "" {
		return filepath.Abs(l.o.DataDir)
	}
	home, _ := os.UserHomeDir()
	return DataDir(l.o.GOOS, os.Getenv, home)
}

func (l *Launcher) logger(dataDir string) (*slog.Logger, func(), error) {
	if l.o.Log != nil {
		return l.o.Log, func() {}, nil
	}
	rf, err := OpenRotatingFile(filepath.Join(dataDir, "logs", LogFileName), DefaultLogMaxBytes, DefaultLogKeep)
	if err != nil {
		return nil, nil, fmt.Errorf("desktop: log file: %w", err)
	}
	var w io.Writer = rf
	if l.o.Console != nil {
		w = io.MultiWriter(rf, l.o.Console)
	}
	return app.NewLogger(w, slog.LevelInfo), func() { _ = rf.Close() }, nil
}

// activateRunning opens the panel of the instance that holds the lock.
func (l *Launcher) activateRunning(ctx context.Context) error {
	l.log.Info("arenile desktop: already running, opening its panel")
	deadline := time.Now().Add(15 * time.Second)
	for {
		info, err := ReadInstanceInfo(l.dataDir)
		if err == nil && info.BaseURL != "" {
			target := info.OpenURL
			if target == "" {
				target = info.BaseURL + "/admin/"
			}
			return l.ui.OpenURL(target)
		}
		if time.Now().After(deadline) {
			return ErrAlreadyRunning
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// DesktopConfig is the core configuration of the desktop edition (pure
// apart from config.Parse): loopback HTTP on port, SQLite in dataDir,
// automatic migrations, no telemetry. It is parsed from a fixed map, not
// from the process environment, so the desktop behaves the same on every
// PC. ARENILE_EDITION=desktop is the loopback profile of the core: cookies
// without the Secure attribute on http://127.0.0.1 (browsers would drop
// them, Safari first) while TLS requests on the LAN listener still get it
// (middleware.CookieSecure). ARENILE_DEV stays false: no development
// behaviour ever ships on a desktop install.
func DesktopConfig(dataDir string, port int, secretKey []byte) (config.Config, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	env := map[string]string{
		config.EnvAddr:        addr,
		config.EnvBaseURL:     "http://" + addr,
		config.EnvDataDir:     dataDir,
		config.EnvSecretKey:   base64.StdEncoding.EncodeToString(secretKey),
		config.EnvAutoMigrate: "true",
		config.EnvTelemetry:   "false",
		config.EnvEdition:     config.EditionDesktop,
		config.EnvLogLevel:    "info",
	}
	return config.Parse(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
}

func (l *Launcher) secretStores() []SecretStore {
	if l.o.SecretStores != nil {
		return l.o.SecretStores
	}
	return []SecretStore{NewKeyringSecretStore(l.dataDir), FileSecretStore{Path: filepath.Join(l.dataDir, "secret.key")}}
}

func (l *Launcher) autostart() Autostart {
	if l.o.Autostart != nil {
		return l.o.Autostart
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	home, _ := os.UserHomeDir()
	a, err := NewAutostart(l.o.GOOS, home, exe, os.Getenv, nil)
	if err != nil {
		l.log.Warn("desktop autostart unavailable", slog.String("error", err.Error()))
		return nil
	}
	return a
}

func (l *Launcher) runLocal(ctx context.Context) error {
	log := l.log
	dbPath := filepath.Join(l.dataDir, "arenile.db")
	_, statErr := os.Stat(dbPath)
	key, source, err := ResolveSecretKey(ctx, statErr == nil, l.secretStores()...)
	if err != nil {
		switch {
		case errors.Is(err, ErrSecretStoreUnavailable):
			l.ui.Notify("Arenile", "Portachiavi del sistema non disponibile o bloccato: sbloccalo (o consenti l'accesso) e riapri Arenile. I dati non sono stati toccati.")
		case errors.Is(err, ErrSecretKeyMissing):
			l.ui.Notify("Arenile", "Chiave segreta dell'archivio non trovata: Arenile non riparte con una chiave nuova. Dettagli nella cartella dati (logs).")
		}
		return err
	}
	log.Info("arenile desktop: secret key ready", slog.String("store", source))

	st := loadState(l.dataDir)
	port, err := ReserveLoopbackPort(st.Port)
	if err != nil {
		return err
	}
	if st.Port != port {
		st.Port = port
		if err := saveState(l.dataDir, st); err != nil {
			log.Warn("desktop state not saved", slog.String("error", err.Error()))
		}
	}
	if l.cfg, err = DesktopConfig(l.dataDir, port, key); err != nil {
		return err
	}

	deps := RuntimeDeps{
		DataDir:           l.dataDir,
		Backuper:          &Backuper{DBPath: l.cfg.SQLitePath(), Clock: l.o.Clock, Log: log},
		Clock:             l.o.Clock,
		Log:               log,
		Autostart:         l.autostart(),
		OnSettingsChanged: func(control.Settings) { l.refreshMenu() },
		OnSetupCompleted:  l.setupCompleted,
	}
	if l.o.NewLAN != nil {
		deps.LAN = l.o.NewLAN(LANDeps{DataDir: l.dataDir, Serve: l.serveOnCurrent, Log: log})
	}
	l.rt = NewRuntime(deps)

	if err := l.start(ctx); err != nil {
		return err
	}
	settings, _ := l.rt.Settings(ctx)
	if settings.LANEnabled {
		if err := l.rt.StartLAN(ctx, settings.LANPort); err != nil {
			log.Warn("desktop LAN mode not started", slog.String("error", err.Error()))
			l.ui.Notify("Arenile", "Modalità rete locale non avviata: controlla le impostazioni.")
		} else {
			l.warnCertRenewal()
		}
	}
	if err := l.ui.OpenURL(l.panelURL()); err != nil {
		log.Warn("desktop browser not opened", slog.String("error", err.Error()))
	}
	if l.o.OnReady != nil {
		l.o.OnReady(StartInfo{DataDir: l.dataDir, BaseURL: l.cfg.BaseURL, OpenURL: l.panelURL(), Runtime: l.rt})
	}

	bgCtx, stopBg := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() {
		sched := &Scheduler{Runtime: l.rt, Clock: l.o.Clock, Log: log, Tick: l.o.ScheduleTick, OnFailure: func(error) {
			l.ui.Notify("Arenile", "Backup giornaliero non riuscito: controlla la cartella dei backup.")
		}}
		_ = sched.Run(bgCtx)
	})
	wg.Go(func() {
		u := UpdateChecker{URL: l.o.UpdateURL, Current: l.o.Version, Client: l.o.HTTPClient, Log: log}
		RunUpdateChecks(bgCtx, u, func() bool {
			s, _ := l.rt.Settings(bgCtx)
			return s.UpdateCheck
		}, func(latest string) {
			l.ui.Notify("Arenile", "È disponibile la versione "+latest+": scaricala dal portale Coste360.")
		}, l.o.UpdateTick)
	})
	wg.Go(func() { l.supervise(bgCtx) })
	uiDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.ui.Quit()
		case <-uiDone:
		}
	}()

	uiErr := l.ui.Run(ctx, l.menu())
	close(uiDone)
	stopBg()
	wg.Wait()
	l.shutdown()
	l.mu.Lock()
	fatal := l.fatal
	l.mu.Unlock()
	return errors.Join(fatal, uiErr)
}

// server is one running app instance.
type server struct {
	app      *app.App
	cancel   context.CancelFunc
	exited   chan struct{}
	err      error
	stopping atomic.Bool
}

func (s *server) stop() error {
	s.stopping.Store(true)
	s.cancel()
	<-s.exited
	return s.err
}

// start builds and serves a new app instance (first start and after a
// restore) and refreshes the first-run state.
func (l *Launcher) start(ctx context.Context) error {
	a, err := app.New(l.cfg,
		app.WithDesktop(l.rt), app.WithLogger(l.log), app.WithVersion(l.o.Version),
		app.WithClock(l.o.Clock), app.WithoutSignalHandling())
	if err != nil {
		return fmt.Errorf("desktop: start the server: %w", err)
	}
	hasUsers, err := store.HasUsers(ctx, a.DB())
	if err != nil {
		_ = a.Close()
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &server{app: a, cancel: cancel, exited: make(chan struct{})}
	go func() {
		s.err = a.Run(runCtx)
		close(s.exited)
	}()
	select {
	case <-a.Ready():
	case <-s.exited:
		cancel()
		return fmt.Errorf("desktop: start the server: %w", s.err)
	}
	var setupURL string
	if !hasUsers {
		tok, err := NewSetupToken()
		if err != nil {
			_ = s.stop()
			return err
		}
		l.rt.SetSetupToken(tok)
		setupURL = l.cfg.BaseURL + "/admin/first-run#token=" + tok.Value()
	} else {
		l.rt.SetSetupToken(nil)
	}
	l.mu.Lock()
	l.srv, l.setupURL = s, setupURL
	l.mu.Unlock()
	l.writeInstanceInfo()
	l.log.Info("arenile desktop ready", slog.String("url", l.cfg.BaseURL+"/admin/"), slog.Bool("first_run", !hasUsers))
	return nil
}

// panelURL is the page "Apri pannello" opens.
func (l *Launcher) panelURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.setupURL != "" {
		return l.setupURL
	}
	return l.cfg.BaseURL + "/admin/"
}

func (l *Launcher) writeInstanceInfo() {
	info := InstanceInfo{PID: os.Getpid(), BaseURL: l.cfg.BaseURL, OpenURL: l.panelURL()}
	if err := WriteInstanceInfo(l.dataDir, info); err != nil {
		l.log.Warn("desktop instance file not written", slog.String("error", err.Error()))
	}
}

func (l *Launcher) setupCompleted() {
	l.mu.Lock()
	l.setupURL = ""
	l.mu.Unlock()
	l.writeInstanceInfo()
}

func (l *Launcher) current() *server {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.srv
}

// serveOnCurrent implements LANDeps.Serve on the running app instance.
func (l *Launcher) serveOnCurrent(ln net.Listener) func(context.Context) error {
	s := l.current()
	if s == nil {
		_ = ln.Close()
		return func(context.Context) error { return nil }
	}
	return s.app.ServeListener(ln)
}

// supervise restarts the server for restores and quits when it crashes.
func (l *Launcher) supervise(ctx context.Context) {
	for {
		s := l.current()
		select {
		case <-ctx.Done():
			return
		case name := <-l.rt.Restarts():
			if err := l.restore(ctx, name); err != nil {
				l.fail(err)
				return
			}
		case <-s.exited:
			if s.stopping.Load() {
				continue
			}
			err := s.err
			if err == nil {
				err = errors.New("desktop: the server stopped unexpectedly")
			}
			l.fail(err)
			return
		}
	}
}

func (l *Launcher) fail(err error) {
	l.log.Error("arenile desktop: fatal error", slog.String("error", err.Error()))
	l.mu.Lock()
	l.fatal = err
	l.mu.Unlock()
	l.ui.Notify("Arenile", "Arenile si è fermato per un errore: riaprilo. Dettagli nella cartella dati (logs).")
	l.ui.Quit()
}

// restore stops the server, restores the backup and starts it again. A
// failed restore keeps the current database (the server restarts on it);
// only a failed restart is fatal.
func (l *Launcher) restore(ctx context.Context, name string) error {
	l.log.Info("arenile desktop: restoring", slog.String("name", name))
	settings, _ := l.rt.Settings(ctx)
	_ = l.rt.StopLAN(ctx)
	if s := l.current(); s != nil {
		if err := s.stop(); err != nil {
			l.log.Warn("desktop server stop", slog.String("error", err.Error()))
		}
	}
	restoreErr := l.rt.RestoreNamed(context.WithoutCancel(ctx), name)
	if err := l.start(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	if settings.LANEnabled {
		if err := l.rt.StartLAN(ctx, settings.LANPort); err != nil {
			l.log.Warn("desktop LAN mode not restarted", slog.String("error", err.Error()))
		}
	}
	if restoreErr != nil {
		l.log.Error("desktop restore failed", slog.String("error", restoreErr.Error()))
		l.ui.Notify("Arenile", "Ripristino non riuscito: i dati attuali non sono stati modificati.")
		return nil
	}
	l.ui.Notify("Arenile", "Ripristino completato: ricarica la pagina del pannello.")
	return nil
}

// shutdown stops the server and makes the on-exit backup (after the stop,
// so it contains every write).
func (l *Launcher) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = l.rt.StopLAN(ctx)
	if s := l.current(); s != nil {
		if err := s.stop(); err != nil {
			l.log.Warn("desktop server stop", slog.String("error", err.Error()))
		}
	}
	if settings, _ := l.rt.Settings(ctx); settings.BackupOnExit {
		if _, err := l.rt.BackupNow(ctx, control.BackupExit); err != nil {
			l.log.Error("desktop exit backup failed", slog.String("error", err.Error()))
		}
	}
	RemoveInstanceInfo(l.dataDir)
	l.log.Info("arenile desktop stopped")
}

// menu builds the tray menu from the current settings (spec 29.1).
func (l *Launcher) menu() []MenuItem {
	s, _ := l.rt.Settings(context.Background())
	items := []MenuItem{
		{ID: MenuOpen, Label: "Apri pannello", OnClick: func() { _ = l.ui.OpenURL(l.panelURL()) }},
		{ID: MenuBackup, Label: "Backup ora", OnClick: func() { go l.backupNow() }},
		{ID: MenuDataDir, Label: "Apri cartella dati", OnClick: func() { _ = l.ui.OpenFolder(l.dataDir) }},
	}
	if l.rt.d.Autostart != nil {
		items = append(items, MenuItem{
			ID: MenuAutostart, Label: "Avvio automatico all'accensione", Checkbox: true, Checked: s.Autostart, //nolint:misspell // Italian label (spec 29.1)
			OnClick: func() { go l.toggle(func(s *control.Settings) { s.Autostart = !s.Autostart }) },
		})
	}
	if l.rt.LANAvailable() {
		label := "Modalità rete locale"
		if d := l.rt.Runtime().Desktop; d != nil && d.LANURL != "" {
			label += " (" + d.LANURL + ")"
			if soon, at := l.certRenewalSoon(); soon {
				label += " — certificato in rinnovo il " + at
			}
		}
		items = append(items, MenuItem{
			ID: MenuLAN, Label: label, Checkbox: true, Checked: s.LANEnabled,
			OnClick: func() { go l.toggle(func(s *control.Settings) { s.LANEnabled = !s.LANEnabled }) },
		}, MenuItem{
			// Always present (disabled while off): the tray keeps the item order.
			ID: MenuLANQR, Label: "Mostra QR rete locale", Disabled: !s.LANEnabled,
			OnClick: func() { _ = l.ui.OpenURL(l.cfg.BaseURL + "/admin/rete-locale") },
		})
	}
	items = append(items,
		MenuItem{ID: MenuRestore, Label: "Ripristina un backup…", OnClick: func() { _ = l.ui.OpenURL(l.cfg.BaseURL + "/admin/desktop") }},
		MenuItem{ID: MenuQuit, Label: "Esci", OnClick: l.ui.Quit},
	)
	return items
}

func (l *Launcher) refreshMenu() { l.ui.SetMenu(l.menu()) }

// CertRenewWarnBefore mirrors lan.RenewWarnBefore (the launcher does not
// import the adapter): how long before the LAN certificate regeneration
// the tray and a notification warn the operator.
const CertRenewWarnBefore = 60 * 24 * time.Hour

// certRenewalSoon reports whether the LAN certificate will be regenerated
// within CertRenewWarnBefore and the local date of that renewal.
func (l *Launcher) certRenewalSoon() (bool, string) {
	d := l.rt.Runtime().Desktop
	if d == nil || d.LANCertRenewsAt.IsZero() {
		return false, ""
	}
	if d.LANCertRenewsAt.Sub(l.o.Clock.Now()) > CertRenewWarnBefore {
		return false, ""
	}
	return true, d.LANCertRenewsAt.Local().Format("02/01/2006")
}

// warnCertRenewal notifies once per start when the LAN certificate is
// about to be regenerated: paired devices and browsers must then accept
// the new one, so the operator plans it (spec 29.1).
func (l *Launcher) warnCertRenewal() {
	if soon, at := l.certRenewalSoon(); soon {
		l.ui.Notify("Arenile", "Il certificato della rete locale sarà rinnovato il "+at+": dopo quel giorno riabbina le app e riaccetta il certificato nei browser.")
	}
}

func (l *Launcher) backupNow() {
	b, err := l.rt.BackupNow(context.Background(), control.BackupManual)
	if err != nil {
		l.log.Error("desktop manual backup failed", slog.String("error", err.Error()))
		l.ui.Notify("Arenile", "Backup non riuscito: controlla la cartella dei backup.")
		return
	}
	l.ui.Notify("Arenile", "Backup completato: "+b.Name)
}

func (l *Launcher) toggle(change func(*control.Settings)) {
	ctx := context.Background()
	s, _ := l.rt.Settings(ctx)
	change(&s)
	if _, err := l.rt.UpdateSettings(ctx, s); err != nil {
		l.log.Warn("desktop setting not changed", slog.String("error", err.Error()))
		l.ui.Notify("Arenile", "Impostazione non modificata: "+err.Error())
		l.refreshMenu()
	}
}
