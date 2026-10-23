// Package remote is the remote mode of the desktop app (spec 29.1,
// "Modalità remota"): no internal server; the app is a client of an
// existing Arenile instance (VPS, private server, also behind a VPN, or a
// desktop edition in LAN mode) and acts as the bridge towards the fiscal
// printer / registratore telematico attached to the cash desk PC. It
// implements desktop.RemoteMode and is wired by cmd/arenile-desktop
// through desktop.Options.Remote.
//
// Ownership (M6): the "LAN and remote mode" builder.
//
// # Pairing (same mechanism as the mobile app, spec 29.2)
//
// The panel (Configurazione -> Dispositivi) creates a one-time code and a
// QR text arenile://pair?d=... (internal/domain/device). Pairer checks
// GET /api/admin/v1/meta (product, min_client_versions.desktop) and
// exchanges the code at POST /api/device/v1/pair for a device token bound
// to the user and the establishment, revocable from the panel. The token
// is kept in the OS keyring (TokenStore, 0600 file fallback), never in
// remote.json and never in logs. Plain http is refused except on loopback:
// the token must not cross any network in clear. A QR from a desktop LAN
// instance carries the certificate fingerprint, which every client of this
// package pins (HTTPClient). Input: the pairing form of the "desktop"
// build (webview) or Mode.PairAndSave (launcher flags).
//
// # Window
//
// Window is a port: the system webview through github.com/webview/webview_go
// (MIT) behind the "desktop" build tag and CGO on macOS and Windows
// (window_webview.go; not on Linux, where WebKitGTK 4.0 is no longer
// shipped by current distributions), else BrowserWindow (default browser,
// or a Chromium-family browser in --kiosk --app mode for the kiosk option). The webview gets InitScript, which sets
// window.arenileDesktop = {mode: "remote"} for the panel (lib/meta.ts) and
// routes links to other origins to the default browser. For an instance
// with a self-signed certificate the window loads the panel through Proxy,
// a loopback reverse proxy that uses the pinned client, because system
// webviews cannot pin certificates. The tray menu (builds without the
// webview) offers: open panel, bridge state, data folder, switch to local
// mode at the next start, unpair, quit.
//
// # Fiscal bridge
//
// With Config.FiscalBridge the mode runs fiscalbridge.Agent with the local
// driver chosen in remote.json (fiscalbridge.LocalDrivers: "noop" today;
// real drivers register a factory) and a fiscalbridge.FileResults in
// <data dir>/bridge, so a receipt is never printed twice across lost acks
// and restarts. A 401 (device revoked) removes the token and the pairing
// and closes the app with a notification.
//
// # Settings (remote.json)
//
// mode local|remote (the launcher setting "Locale | Remota"; the two modes
// are alternatives, never synchronised), instance URL, fingerprint, device
// id and name, kiosk, autostart (applied with desktop.NewAutostart),
// fiscal_bridge, fiscal_driver and its configuration.
//
// # Offline
//
// The panel applies the offline rules of section 21.2 by itself (cache,
// queued check-ins, notes and cash takings); the remote mode adds nothing:
// bookings are created and moved only online. The bridge queues on the
// instance: jobs wait there while the cash desk PC is offline.
//
// # Extension points (local periphery)
//
//   - Fiscal printer / registratore telematico: register a
//     fiscalbridge.LocalDriverFactory (one per protocol family, spec 14.2).
//   - Receipt printer and QR reader: not bridged yet. They will follow the
//     same pattern (job kind on the instance queue, a local adapter here);
//     a USB QR reader in keyboard mode already works in the panel window.
package remote
