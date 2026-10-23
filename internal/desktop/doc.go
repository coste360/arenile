// Package desktop is the shared runtime of the desktop edition (spec 29.1),
// used by the launcher cmd/arenile-desktop. It contains no tray or webview
// code and no CGO: those live behind the "desktop" build tag in the
// launcher, so `go build ./...` and `make check` work on CI without CGO.
//
// Ownership (M6): the desktop launcher builder owns this package,
// internal/desktop/control and cmd/arenile-desktop; the LAN and remote
// modes are the adapters internal/desktop/lan and internal/desktop/remote,
// plugged in through the LANMode and RemoteMode ports.
//
// # Architecture (ports and adapters, functional core / imperative shell)
//
// Pure functions hold the decisions and are table-tested on every OS:
// DataDir, DesktopConfig, BackupFileName/ParseBackupFileName,
// PruneCandidates, DailyDue, CompareVersions, LaunchAgentPlist,
// XDGAutostartEntry, WindowsRunValue, OpenCommand, NotifyCommand,
// TrayIconPNG. The imperative shell around them is small and injected:
//
//   - UI (tray or headless, cmd/arenile-desktop), LANMode, RemoteMode;
//   - SecretStore (strategy: KeyringSecretStore via go-keyring, then
//     FileSecretStore 0600), Autostart (strategy per OS: LaunchAgent plist,
//     HKCU Run value through RunKey, XDG autostart entry), BackupStore
//     (Backuper on the shared store.BackupSQLite/RestoreSQLiteFile);
//   - clock.Clock for the scheduler, http.Client for the update check.
//
// # Files
//
//   - launcher.go: Launcher.Run, the start sequence: data dir -> rotated
//     log -> single-instance lock -> secret key -> 127.0.0.1:<free port,
//     reused> -> app.New(cfg, app.WithDesktop(runtime)) (the same bootstrap
//     as `arenile serve`, migrations included) -> panel (app window of the
//     shell build, else browser) on /admin/ or /admin/first-run#token=...
//     -> tray menu, daily backups, update check, restore = stop, replace,
//     start -> exit backup after the stop.
//   - activate.go: the single-instance activation listener (a second
//     launch brings the running window to the front).
//   - shell/: the native panel window (webview, macOS menu, downloads).
//   - runtime.go: Runtime implements control.Control (and
//     meta.RuntimeProvider): settings with side effects, backups, restore
//     requests, first-run token, LAN state for /meta.
//   - backup.go: Backuper, retention, Scheduler (daily, retried hourly
//     after a failure, aware of the last daily backup across restarts).
//   - instance.go, lock_*.go: InstanceLock, instance.json (the running
//     panel URL for a second launch), the remembered port.
//   - keyring.go, secret.go: secret key stores and ResolveSecretKey (never
//     generates a new key for an existing database).
//   - autostart*.go, open.go, icon.go, logfile.go, updates.go, paths.go,
//     settings.go, firstrun.go, lan.go: see each type.
//
// # Invariants (docs/invariants.md INV-51, INV-53, INV-54)
//
//   - The internal server listens on 127.0.0.1 only; the LAN listener is a
//     separate TLS listener opened only when the operator enables it.
//   - /api/desktop/v1 is reachable only from loopback (middleware
//     LoopbackOnly checks the TCP peer, not X-Forwarded-For, and the Host
//     header against DNS rebinding); first-run needs the one-time token.
//   - No network call to Coste360 or anywhere else unless the operator
//     enables the update check (ARENILE_TELEMETRY stays false).
//   - Payments: manual driver only; no public page (meta capabilities and
//     the panel's DesktopOnlineNotice).
//   - Logs never contain the first-run token, the secret key or personal
//     data.
package desktop
