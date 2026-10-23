// Package desktop is the shared runtime of the desktop edition (spec 29.1),
// used by the launcher cmd/arenile-desktop. It contains no tray or webview
// code and no CGO: those live behind the "desktop" build tag in the
// launcher, so `go build ./...` and `make check` work on CI without CGO.
//
// Ownership (M6): the "desktop runtime" builder owns this package and
// internal/desktop/control; the launcher builder owns cmd/arenile-desktop.
//
// # Responsibilities
//
//   - paths.go: data directory per OS (%APPDATA%\Arenile,
//     ~/Library/Application Support/Arenile, $XDG_DATA_HOME/arenile or
//     ~/.local/share/arenile), never next to the executable.
//   - secret.go: ARENILE_SECRET_KEY generated at first run and kept in the
//     OS keyring (SecretStore strategy: keyring first, then a 0600 file in
//     the data dir as fallback).
//   - settings.go: desktop.json (control.Settings) with defaults.
//   - backup.go: consistent SQLite backups (VACUUM INTO, the same file as
//     `arenile backup`, restorable with `arenile restore` on a VPS), daily
//     and on-exit scheduling, retention, validation, restore on restart.
//   - lan.go: LAN mode. Self-signed ECDSA certificate in the data dir, its
//     SHA-256 fingerprint for the pairing QR, and a listener that accepts
//     only private-network peers (RFC 1918, RFC 4193, link-local,
//     loopback); everything else is closed before TLS.
//   - firstrun.go: the one-time setup token passed to the browser in the
//     URL fragment for the first owner creation.
//   - runtime.go: Runtime implements control.Control (and
//     meta.RuntimeProvider) and is passed to app.New with app.WithDesktop.
//   - launcher.go: the start sequence (data dir, secret, config,
//     migrations, app on 127.0.0.1:0, open browser) behind the UI port, and
//     the remote mode (pairing with an instance, fiscal bridge agent).
//
// # Invariants
//
//   - The internal server listens on 127.0.0.1 only; the LAN listener is a
//     separate TLS listener that is opened only when the operator enables
//     it and filters peers by address.
//   - /api/desktop/v1 is reachable only from loopback (middleware
//     LoopbackOnly checks the TCP peer, not X-Forwarded-For, and the Host
//     header against DNS rebinding).
//   - No network call to Coste360 or anywhere else unless the operator
//     enables the update check (ARENILE_TELEMETRY stays false).
//   - Payments: manual driver only; no public page (meta capabilities).
package desktop
