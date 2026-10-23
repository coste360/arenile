package remote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/fiscalbridge"
)

// ErrNotPaired: remote mode chosen but this PC is not paired (or its token
// was revoked) and no pairing form is available in this build.
var ErrNotPaired = errors.New("modalità remota: questo PC non è abbinato a un'istanza (avvia con -pair oppure usa la build con finestra)")

// Options configure the remote mode.
type Options struct {
	Version string
	GOOS    string
	Clock   clock.Clock
	// Drivers is the registry of local fiscal drivers (default: noop only).
	Drivers *fiscalbridge.LocalDrivers
	// Tokens overrides the keyring token store (tests).
	Tokens func(dataDir string) TokenStore
	// Window overrides the panel window (default: the webview of the
	// "desktop" build, else BrowserWindow).
	Window Window
	// Pairing overrides the pairing form (default: the webview form of the
	// "desktop" build, else none).
	Pairing PairingForm
	// HTTPClient overrides the pinned client (tests).
	HTTPClient func(fingerprint string) *http.Client
	// Autostart applies Config.Autostart (default: desktop.NewAutostart).
	Autostart desktop.Autostart
}

// PairingForm asks the operator for the pairing input and calls submit
// until it succeeds (the form shows submit's error) or the operator closes
// it (ErrNotPaired).
type PairingForm interface {
	Ask(ctx context.Context, submit func(PairInput) error) error
}

// nativePairing builds the webview pairing form (window_webview.go).
var nativePairing func() PairingForm

// Mode implements desktop.RemoteMode: no internal server; the panel of the
// instance in a dedicated window, and the fiscal bridge agent towards the
// local fiscal driver.
type Mode struct {
	o Options

	mu    sync.Mutex
	agent *fiscalbridge.Agent
}

var _ desktop.RemoteMode = (*Mode)(nil)

// New returns the remote mode for desktop.Options.Remote.
func New(o Options) *Mode {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	if o.Drivers == nil {
		o.Drivers = fiscalbridge.NewLocalDrivers()
	}
	if o.Tokens == nil {
		o.Tokens = func(dataDir string) TokenStore { return NewTokenStore(dataDir) }
	}
	if o.HTTPClient == nil {
		o.HTTPClient = HTTPClient
	}
	return &Mode{o: o}
}

// Configured implements desktop.RemoteMode: the operator chose the remote
// mode (paired or not: an unpaired remote mode asks to pair).
func (m *Mode) Configured(dataDir string) bool {
	c, err := LoadConfig(dataDir)
	return err == nil && c.Mode == desktop.ModeRemote
}

// PairAndSave pairs this PC and stores the token and the configuration
// (launcher flag -pair, pairing form). It keeps the window and bridge
// preferences of an earlier pairing.
func (m *Mode) PairAndSave(ctx context.Context, dataDir string, in PairInput) (Config, error) {
	p := Pairer{Version: m.o.Version, GOOS: m.o.GOOS, Clock: m.o.Clock, Client: m.o.HTTPClient}
	res, err := p.Pair(ctx, in)
	if err != nil {
		return Config{}, err
	}
	if err := m.o.Tokens(dataDir).Set(ctx, res.Token); err != nil {
		return Config{}, fmt.Errorf("remote: store device token: %w", err)
	}
	cfg := res.Config
	if prev, err := LoadConfig(dataDir); err == nil {
		cfg.Kiosk, cfg.Autostart, cfg.FiscalDriver, cfg.FiscalDriverConfig = prev.Kiosk, prev.Autostart, prev.FiscalDriver, prev.FiscalDriverConfig
		if !prev.FiscalBridge && prev.Paired() {
			cfg.FiscalBridge = false
		}
	}
	if cfg.FiscalDriver == "" {
		cfg.FiscalDriver = fiscalbridge.NoopCode
	}
	if err := SaveConfig(dataDir, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Run implements desktop.RemoteMode.
func (m *Mode) Run(ctx context.Context, ui desktop.UI, dataDir string, log *slog.Logger) error {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	tokens := m.o.Tokens(dataDir)
	cfg, err := LoadConfig(dataDir)
	if err != nil {
		return err
	}
	token, terr := tokens.Get(ctx)
	if !cfg.Paired() || terr != nil {
		if cfg, err = m.pair(ctx, ui, dataDir); err != nil {
			return err
		}
		if token, err = tokens.Get(ctx); err != nil {
			return err
		}
	}
	m.applyAutostart(cfg, log)
	hc := m.o.HTTPClient(cfg.Fingerprint)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	revoked := make(chan struct{})
	if cfg.FiscalBridge {
		agent, err := m.newAgent(cfg, token, hc, dataDir, log)
		if err != nil {
			ui.Notify("Arenile", "Ponte fiscale non avviato: "+err.Error())
			log.Error("fiscal bridge not started", slog.String("error", err.Error()))
		} else {
			wg.Go(func() {
				if err := agent.Run(ctx); errors.Is(err, fiscalbridge.ErrDeviceRevoked) {
					close(revoked)
				}
			})
		}
	}
	forgotten := cfg
	forgotten.DeviceID = ""
	wg.Go(func() {
		select {
		case <-ctx.Done():
		case <-revoked:
			log.Warn("remote mode: device token refused, pairing removed")
			_ = tokens.Delete(context.Background())
			_ = SaveConfig(dataDir, forgotten)
			ui.Notify("Arenile", "Questo PC è stato scollegato dal pannello dell'istanza: abbinalo di nuovo.")
			cancel()
			ui.Quit()
		}
	})

	panel := cfg.InstanceURL + "/admin/"
	if cfg.Fingerprint != "" {
		px, err := StartProxy(ctx, cfg.InstanceURL, hc.Transport, log)
		if err != nil {
			return fmt.Errorf("remote: panel proxy: %w", err)
		}
		panel = px.URL + "/admin/"
	}
	opts := WindowOptions{
		URL: panel, Title: windowTitle(cfg), Kiosk: cfg.Kiosk, Init: InitScript(m.o.Version, cfg.Kiosk),
		OpenExternal: ui.OpenURL, Width: 1366, Height: 860,
	}
	var runErr error
	if w := m.window(ui, dataDir); w != nil {
		runErr = w.Open(ctx, opts)
	} else {
		if err := (BrowserWindow{OpenURL: ui.OpenURL, ProfileDir: filepath.Join(dataDir, "kiosk-profile"), GOOS: m.o.GOOS}).open(ctx, opts); err != nil {
			log.Warn("remote mode: browser not opened", slog.String("error", err.Error()))
		}
		runErr = ui.Run(ctx, m.menu(ctx, ui, dataDir, cfg, token, hc, panel))
	}
	cancel()
	wg.Wait()
	return runErr
}

// window returns the native panel window, or nil to use the browser and
// the tray (headless/CGO-free builds).
func (m *Mode) window(desktop.UI, string) Window {
	if m.o.Window != nil {
		return m.o.Window
	}
	if nativeWindow != nil {
		return nativeWindow()
	}
	return nil
}

func (m *Mode) pair(ctx context.Context, ui desktop.UI, dataDir string) (Config, error) {
	form := m.o.Pairing
	if form == nil && nativePairing != nil {
		form = nativePairing()
	}
	if form == nil {
		ui.Notify("Arenile", "Modalità remota: abbina questo PC con il codice del pannello (Configurazione → Dispositivi).")
		return Config{}, ErrNotPaired
	}
	var cfg Config
	err := form.Ask(ctx, func(in PairInput) error {
		c, err := m.PairAndSave(ctx, dataDir, in)
		cfg = c
		return err
	})
	return cfg, err
}

func (m *Mode) newAgent(cfg Config, token string, hc *http.Client, dataDir string, log *slog.Logger) (*fiscalbridge.Agent, error) {
	drv, err := m.o.Drivers.New(cfg.FiscalDriver, cfg.FiscalDriverConfig, m.o.Clock)
	if err != nil {
		return nil, err
	}
	results, err := fiscalbridge.OpenFileResults(filepath.Join(dataDir, "bridge", "results.json"))
	if err != nil {
		return nil, err
	}
	a := fiscalbridge.NewAgent(fiscalbridge.AgentDeps{
		BaseURL: cfg.InstanceURL, Token: token, HTTPClient: hc, Driver: drv, Results: results,
		Clock: m.o.Clock, Log: log, UserAgent: "arenile-desktop/" + m.o.Version,
	})
	m.mu.Lock()
	m.agent = a
	m.mu.Unlock()
	return a, nil
}

// BridgeState returns the state of the fiscal bridge agent (zero when it
// is not running).
func (m *Mode) BridgeState() fiscalbridge.AgentState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.agent == nil {
		return fiscalbridge.AgentState{}
	}
	return m.agent.State()
}

func (m *Mode) applyAutostart(cfg Config, log *slog.Logger) {
	a := m.o.Autostart
	if a == nil {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		home, _ := os.UserHomeDir()
		if a, err = desktop.NewAutostart(m.o.GOOS, home, exe, os.Getenv, nil); err != nil {
			return
		}
	}
	if on, err := a.Enabled(); err == nil && on == cfg.Autostart {
		return
	}
	if err := a.SetEnabled(cfg.Autostart); err != nil {
		log.Warn("remote mode: autostart not applied", slog.String("error", err.Error()))
	}
}

// menu is the tray menu of the remote mode in builds without the native
// window.
func (m *Mode) menu(ctx context.Context, ui desktop.UI, dataDir string, cfg Config, token string, hc *http.Client, panel string) []desktop.MenuItem {
	bridge := "Ponte fiscale: disattivato"
	if cfg.FiscalBridge {
		bridge = "Ponte fiscale: attivo (" + cfg.FiscalDriver + ")"
	}
	return []desktop.MenuItem{
		{ID: desktop.MenuOpen, Label: "Apri pannello", OnClick: func() { _ = ui.OpenURL(panel) }},
		{ID: "bridge", Label: bridge, Disabled: true},
		{ID: desktop.MenuDataDir, Label: "Apri cartella dati", OnClick: func() { _ = ui.OpenFolder(dataDir) }},
		{ID: "local", Label: "Passa alla modalità locale (al prossimo avvio)", OnClick: func() {
			if err := SetMode(dataDir, desktop.ModeLocal); err == nil {
				ui.Notify("Arenile", "Al prossimo avvio Arenile userà il server interno di questo PC.")
			}
		}},
		{ID: "unpair", Label: "Scollega questo PC dall'istanza", OnClick: func() {
			go func() {
				if err := m.Unpair(ctx, dataDir, cfg, token, hc); err != nil {
					ui.Notify("Arenile", "Scollegamento non riuscito: "+err.Error())
					return
				}
				ui.Notify("Arenile", "PC scollegato dall'istanza.")
				ui.Quit()
			}()
		}},
		{ID: desktop.MenuQuit, Label: "Esci", OnClick: ui.Quit},
	}
}

// Unpair revokes the device token on the instance (DELETE
// /api/device/v1/self), removes it from the keyring and forgets the
// pairing. The token is removed locally even when the instance is not
// reachable (the operator can revoke it from the panel).
func (m *Mode) Unpair(ctx context.Context, dataDir string, cfg Config, token string, hc *http.Client) error {
	status, _, err := doJSON(ctx, hc, http.MethodDelete, cfg.InstanceURL+"/api/device/v1/self", token, nil, nil)
	if derr := m.o.Tokens(dataDir).Delete(ctx); derr != nil {
		return derr
	}
	cfg.DeviceID, cfg.Mode = "", desktop.ModeLocal
	if serr := SaveConfig(dataDir, cfg); serr != nil {
		return serr
	}
	if err != nil {
		return fmt.Errorf("istanza non raggiungibile, revoca il dispositivo dal pannello: %w", err)
	}
	if status != http.StatusNoContent && status != http.StatusUnauthorized {
		return fmt.Errorf("risposta inattesa dell'istanza (%d)", status)
	}
	return nil
}

func windowTitle(c Config) string {
	if c.EstablishmentName != "" {
		return "Arenile — " + c.EstablishmentName
	}
	return "Arenile"
}
