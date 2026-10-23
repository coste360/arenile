package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/coste360/arenile/internal/desktop"
)

// headlessUI implements desktop.UI without a tray: it logs the panel URL
// and blocks until Quit or ctx is done. It is the UI of CGO-free builds and
// the fallback when the tray cannot start (e.g. Linux without a status
// notifier).
type headlessUI struct {
	log  *slog.Logger
	once sync.Once
	quit chan struct{}
}

var _ desktop.UI = (*headlessUI)(nil)

func newHeadlessUI(log *slog.Logger) *headlessUI {
	return &headlessUI{log: log, quit: make(chan struct{})}
}

func (h *headlessUI) Run(ctx context.Context, _ []desktop.MenuItem) error {
	select {
	case <-ctx.Done():
	case <-h.quit:
	}
	return nil
}

func (h *headlessUI) SetMenu([]desktop.MenuItem) {}

// OpenURL logs the URL (never with the first-run token: the launcher passes
// it in the fragment and the headless UI strips it).
func (h *headlessUI) OpenURL(url string) error {
	h.log.Info("arenile desktop: open the panel in the browser", slog.String("url", stripFragment(url)))
	return nil
}

func (h *headlessUI) OpenFolder(path string) error {
	h.log.Info("arenile desktop: data folder", slog.String("path", path))
	return nil
}

func (h *headlessUI) Notify(title, message string) {
	h.log.Info(title, slog.String("message", message))
}

func (h *headlessUI) Quit() { h.once.Do(func() { close(h.quit) }) }

func stripFragment(u string) string {
	for i := 0; i < len(u); i++ {
		if u[i] == '#' {
			return u[:i]
		}
	}
	return u
}
