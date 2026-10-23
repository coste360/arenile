//go:build desktop && (linux || !cgo)

package main

import (
	"log/slog"

	"github.com/coste360/arenile/internal/desktop"
)

// uiKind "tray": Linux (CGO-free tray over D-Bus, no WebKitGTK window) and
// CGO-less builds; the panel opens in the default browser.
const uiKind = "tray"

func newUI(log *slog.Logger, o uiOptions) desktop.UI { return newTrayUI(log, o) }
