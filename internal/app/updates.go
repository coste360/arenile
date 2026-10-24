package app

import (
	"context"
	"net/http"
	goruntime "runtime"

	"github.com/coste360/arenile/internal/config"
	"github.com/coste360/arenile/internal/domain/update"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/updates"
)

var _ interface{ Jobs() []jobs.Job } = (*update.Service)(nil)

// WithUpdateNotifier is called once per newer version found by the update
// check (ADR 0025): the desktop launcher shows a native notification.
func WithUpdateNotifier(fn func(latest string)) Option {
	return func(o *options) { o.updateNotify = fn }
}

// WithUpdateHTTPClient overrides the HTTP client of the update check (tests).
func WithUpdateHTTPClient(c *http.Client) Option {
	return func(o *options) { o.updateClient = c }
}

// wireUpdates builds the update notice service (ADR 0025). The check is
// ARENILE_UPDATE_CHECK on the server edition and the update_check setting
// of the desktop app on the desktop edition (read before every check).
func (a *App) wireUpdates() {
	edition, how := config.EditionServer, update.HowBinary
	enabled := func() bool { return a.cfg.UpdateCheck }
	switch {
	case a.opts.desktop != nil:
		edition, how = config.EditionDesktop, update.HowDesktop
		enabled = func() bool {
			s, err := a.opts.desktop.Settings(context.Background())
			return err == nil && s.UpdateCheck
		}
	case a.cfg.Install == config.InstallDocker:
		how = update.HowDocker
	}
	url := a.cfg.UpdateURL
	if url == "" {
		url = updates.DefaultURL
	}
	a.Updates = update.NewService(update.Deps{
		Fetcher: updates.Checker{
			URL: url, Current: a.opts.version, Edition: edition,
			GOOS: goruntime.GOOS, GOARCH: goruntime.GOARCH, Client: a.opts.updateClient,
		},
		Current: a.opts.version, HowTo: how, GOOS: goruntime.GOOS, GOARCH: goruntime.GOARCH,
		Clock: a.opts.clock, Enabled: enabled, OnNewer: a.opts.updateNotify, Log: a.log,
	})
}
