//go:build desktop && cgo && windows

package main

import (
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/coste360/arenile/internal/desktop"
)

// startExternal runs the notification area icon on its own locked OS
// thread with its own message loop (Win32 message queues are per thread),
// while the panel window owns the main thread.
func (t *trayUI) startExternal(menu []desktop.MenuItem) (stop func()) {
	t.mu.Lock()
	t.pending = menu
	t.mu.Unlock()
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		systray.Run(t.onReady, func() {})
	}()
	return func() {
		systray.Quit()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}
