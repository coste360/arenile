//go:build desktop

package main

import (
	"log/slog"

	"github.com/coste360/arenile/internal/desktop"
)

const uiKind = "tray"

// newUI returns the tray UI. Builder (M6 launcher): implement trayUI with a
// systray library (CGO on macOS and Linux, native runners) and, for the
// remote mode, a webview window; add the dependency with its license
// (Apache-2.0 / MIT / BSD only) and fall back to newHeadlessUI when the
// tray cannot start.
func newUI(log *slog.Logger) desktop.UI { return newHeadlessUI(log) }
