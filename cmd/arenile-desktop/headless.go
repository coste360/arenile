package main

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"

	"github.com/coste360/arenile/internal/desktop"
)

// headlessUI implements desktop.UI without a tray: it opens the panel in
// the default browser (unless disabled), logs the URL and blocks until
// Quit or ctx is done. It is the UI of CGO-free builds and of servers
// started with -no-tray; the tray UI reuses it for browser, folder and
// notification handling.
type headlessUI struct {
	log     *slog.Logger
	browser bool
	once    sync.Once
	quit    chan struct{}
}

var _ desktop.UI = (*headlessUI)(nil)

func newHeadlessUI(log *slog.Logger, browser bool) *headlessUI {
	return &headlessUI{log: log, browser: browser, quit: make(chan struct{})}
}

func (h *headlessUI) Run(ctx context.Context, _ []desktop.MenuItem) error {
	select {
	case <-ctx.Done():
	case <-h.quit:
	}
	return nil
}

func (h *headlessUI) SetMenu([]desktop.MenuItem) {}

// OpenURL opens the URL and logs it without the fragment: the first-run
// token travels in the fragment and never reaches a log.
func (h *headlessUI) OpenURL(url string) error {
	h.log.Info("arenile desktop: panel", slog.String("url", stripFragment(url)))
	if !h.browser {
		return nil
	}
	return desktop.OpenURL(context.Background(), runtime.GOOS, url)
}

func (h *headlessUI) OpenFolder(path string) error {
	h.log.Info("arenile desktop: data folder", slog.String("path", path))
	if !h.browser {
		return nil
	}
	return desktop.OpenFolder(context.Background(), runtime.GOOS, path)
}

func (h *headlessUI) Notify(title, message string) {
	h.log.Info("arenile desktop: notification", slog.String("title", title), slog.String("message", message))
	if h.browser {
		_ = desktop.Notify(context.Background(), runtime.GOOS, title, message)
	}
}

func (h *headlessUI) Quit() { h.once.Do(func() { close(h.quit) }) }

func stripFragment(u string) string {
	if i := strings.IndexByte(u, '#'); i >= 0 {
		return u[:i]
	}
	return u
}
