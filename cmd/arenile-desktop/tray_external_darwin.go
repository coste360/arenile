//go:build desktop && cgo && darwin

package main

import (
	"fyne.io/systray"

	"github.com/coste360/arenile/internal/desktop"
)

// startExternal shows the menu bar icon inside the event loop of the panel
// window (webview_go owns NSApp.run): systray only adds its NSStatusItem.
// Call on the main thread after the window exists; stop removes the
// callbacks once the window loop has returned.
func (t *trayUI) startExternal(menu []desktop.MenuItem) (stop func()) {
	t.mu.Lock()
	t.pending = menu
	t.mu.Unlock()
	start, end := systray.RunWithExternalLoop(t.onReady, func() {})
	start()
	return end
}
