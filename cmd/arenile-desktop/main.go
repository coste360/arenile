// Command arenile-desktop is the launcher of the Arenile desktop edition
// (spec 29.1): the same core as `arenile serve` on 127.0.0.1 with a system
// tray icon, or, in remote mode, a client of an existing instance with the
// fiscal bridge towards the local printer / registratore telematico.
//
// Build (see also `make build-desktop`):
//
//	go build ./cmd/arenile-desktop
//	    no CGO, headless UI (opens the browser, logs the panel URL):
//	    what `go build ./...` and CI compile.
//	CGO_ENABLED=1 go build -tags desktop ./cmd/arenile-desktop
//	    tray icon (fyne.io/systray), built on the native runner of each OS;
//	    packaging and signing in the release workflow (portal-bundle-<tag>
//	    artifact, never a GitHub release asset).
//
// The runtime (data dir, keyring, backups, first run, autostart, single
// instance) is internal/desktop; this package only adapts the UI port.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/coste360/arenile/internal/desktop"
	"github.com/coste360/arenile/internal/desktop/lan"
	"github.com/coste360/arenile/internal/desktop/remote"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

// uiOptions select the UI behaviour from the flags.
type uiOptions struct {
	browser bool
	noTray  bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("arenile-desktop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", "", "cartella dati (default: cartella utente del sistema operativo)")
	noBrowser := fs.Bool("no-browser", false, "non aprire il browser all'avvio")
	noTray := fs.Bool("no-tray", false, "nessuna icona nella barra di sistema (solo con la build tray)")
	showVersion := fs.Bool("version", false, "mostra la versione")
	pair := fs.String("pair", "", "abbina questo PC a un'istanza (modalità remota): testo del QR arenile://pair?d=... oppure URL dell'istanza")
	pairCode := fs.String("pair-code", "", "codice di abbinamento mostrato in Configurazione → Dispositivi (con -pair URL)")
	pairFingerprint := fs.String("pair-fingerprint", "", "impronta SHA-256 TLS dell'istanza (solo istanze HTTPS autofirmate)")
	deviceName := fs.String("device-name", "", "nome del dispositivo mostrato nel pannello (default: nome del computer)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stderr, "arenile-desktop %s (%s)\n", version, uiKind)
		return 0
	}
	// Console log for the UI adapter; the launcher logs to the data folder.
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rm := remote.New(remote.Options{Version: version})
	if *pair != "" {
		return runPair(ctx, rm, *dataDir, pairInput(*pair, *pairCode, *pairFingerprint, *deviceName), stderr)
	}
	l := desktop.NewLauncher(desktop.Options{
		Version: version, DataDir: *dataDir, Console: stderr,
		NewLAN: lan.New, // LAN mode adapter (internal/desktop/lan)
		Remote: rm,      // remote mode adapter (internal/desktop/remote)
	})
	ui := newUI(log, uiOptions{browser: !*noBrowser, noTray: *noTray})
	if err := l.Run(ctx, ui); err != nil {
		fmt.Fprintf(stderr, "arenile-desktop: %v\n", err)
		return 1
	}
	return 0
}

// pairInput maps the -pair flags: a QR payload (arenile://pair?d=...) or an
// instance URL plus the code shown in the panel.
func pairInput(target, code, fingerprint, name string) remote.PairInput {
	in := remote.PairInput{Code: code, Fingerprint: fingerprint, DeviceName: name}
	if strings.HasPrefix(target, "arenile://") {
		in.QR = target
	} else {
		in.URL = target
	}
	if in.DeviceName == "" {
		if h, err := os.Hostname(); err == nil {
			in.DeviceName = h
		}
	}
	return in
}

// runPair pairs this PC with an instance and switches the data folder to
// remote mode; the next launch opens the remote panel.
func runPair(ctx context.Context, m *remote.Mode, dataDir string, in remote.PairInput, stderr io.Writer) int {
	dir, err := resolveDataDir(dataDir)
	if err == nil {
		err = os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		fmt.Fprintf(stderr, "arenile-desktop: cartella dati: %v\n", err)
		return 1
	}
	cfg, err := m.PairAndSave(ctx, dir, in)
	if err != nil {
		fmt.Fprintf(stderr, "arenile-desktop: abbinamento non riuscito: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "arenile-desktop: abbinato a %s; al prossimo avvio si apre il pannello remoto\n", cfg.InstanceURL)
	return 0
}

func resolveDataDir(flagValue string) (string, error) {
	if flagValue != "" {
		return filepath.Abs(flagValue)
	}
	home, _ := os.UserHomeDir()
	return desktop.DataDir(runtime.GOOS, os.Getenv, home)
}
