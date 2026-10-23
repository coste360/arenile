// Package meta describes the running instance for clients (spec 29.1,
// 29.2): version, contract version, edition (server or desktop), desktop
// mode (local or LAN), minimum client versions, capabilities and feature
// slugs. It backs GET /api/admin/v1/meta, which is public (no
// establishment data, no personal data) so that the mobile and desktop apps
// can check compatibility before pairing, and the panel can detect the
// desktop edition without a frontend fork.
//
// Architecture: Build is a pure function (functional core) from Input to
// Meta; Service (imperative shell) gathers the input from its ports at
// request time, because the desktop mode changes at runtime when the
// operator enables the LAN mode from the tray menu.
//
// Ports:
//   - RuntimeProvider: the edition and desktop state. ServerRuntime for the
//     server binary; internal/desktop.Runtime for the desktop launcher
//     (passed with app.WithDesktop).
//   - Drivers: registered payment providers and fiscal drivers
//     (app.Registry).
//
// Rules (Build):
//   - desktop edition: no online booking, no online payments, only the
//     "manual" payment provider (spec 29.1 "Limiti voluti");
//   - device pairing is possible on the server edition and on the desktop
//     edition in LAN mode only;
//   - the fiscal bridge is offered by server instances (a desktop app in
//     remote mode connects to them), not by the desktop edition, which
//     drives its FiscalDriver directly.
package meta
