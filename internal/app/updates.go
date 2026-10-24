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
// ARENILE_UPDATE_CHECK on the server edition; on the desktop edition it is
// the update_check setting of the app, and ARENILE_UPDATE_CHECK=false in
// the launcher's environment (acceptance runs) turns it off as well.
func (a *App) wireUpdates() {
	edition, how := config.EditionServer, update.HowBinary
	enabled := func(context.Context) bool { return a.cfg.UpdateCheck }
	switch {
	case a.opts.desktop != nil:
		edition, how = config.EditionDesktop, update.HowDesktop
		enabled = func(ctx context.Context) bool {
			if !a.cfg.UpdateCheck {
				return false
			}
			s, err := a.opts.desktop.Settings(ctx)
			return err == nil && s.UpdateCheck
		}
	case a.cfg.Install == config.InstallDocker:
		how = update.HowDocker
	}
	a.Updates = update.NewService(update.Deps{
		Fetcher: updates.Checker{
			URL: a.cfg.UpdateURL, Current: a.opts.version, Edition: edition,
			GOOS: goruntime.GOOS, GOARCH: goruntime.GOARCH, Client: a.opts.updateClient,
		},
		Current: a.opts.version, HowTo: how, GOOS: goruntime.GOOS, GOARCH: goruntime.GOARCH,
		Clock: a.opts.clock, Enabled: enabled, OnNewer: a.opts.updateNotify, Log: a.log,
	})
}
