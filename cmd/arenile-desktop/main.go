// Command arenile-desktop is the launcher of the Arenile desktop edition
// (spec 29.1): the same core as `arenile serve` on 127.0.0.1 with a system
// tray icon, or, in remote mode, a client of an existing instance with the
// fiscal bridge towards the local printer / registratore telematico.
//
// Build (see also `make build-desktop`):
//
//	go build ./cmd/arenile-desktop
//	    no CGO, headless UI (prints the panel URL, opens the browser):
//	    what `go build ./...` and CI compile.
//	CGO_ENABLED=1 go build -tags desktop ./cmd/arenile-desktop
//	    tray icon (and remote-mode window), built on the native runner of
//	    each OS; packaging and signing in .github/workflows/release.yml
//	    (portal-bundle-<tag> artifact, never a GitHub release asset).
//
// The runtime (data dir, keyring, backups, LAN TLS, first run) is
// internal/desktop; this package only adapts the UI port.
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
	"syscall"

	"github.com/coste360/arenile/internal/desktop"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("arenile-desktop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data-dir", "", "cartella dati (default: cartella utente del sistema operativo)")
	showVersion := fs.Bool("version", false, "mostra la versione")
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
	log := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	l := desktop.NewLauncher(desktop.Options{Version: version, DataDir: *dataDir, Log: log})
	if err := l.Run(ctx, newUI(log)); err != nil {
		fmt.Fprintf(stderr, "arenile-desktop: %v\n", err)
		return 1
	}
	return 0
}
