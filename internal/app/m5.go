package app

import (
	"time"

	"github.com/coste360/arenile/internal/domain/apikey"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/domain/webhook"
	"github.com/coste360/arenile/internal/jobs"
)

// wireSeason builds the M5 part A services (reports, API keys, outgoing
// webhook management) on top of the M0-M4 services. They have no
// dependencies on each other. deliverer is the single event.Deliverer of
// the instance (also run by the core job "webhooks.deliver"): the webhook
// service uses it for the synchronous test attempt.
func (a *App) wireSeason(aw *audit.Writer, deliverer *event.Deliverer) {
	c := a.opts.clock
	a.Reports = report.NewService(report.Deps{DB: a.db, Clock: c, Log: a.log})
	a.APIKeys = apikey.NewService(apikey.Deps{DB: a.db, Clock: c, Audit: aw, Log: a.log})
	a.Webhooks = webhook.NewService(webhook.Deps{
		DB: a.db, Clock: c, Audit: aw, Keys: a.keys, Deliverer: deliverer, Log: a.log,
		Egress:           event.EgressPolicy{AllowPrivate: a.cfg.WebhooksAllowPrivate},
		AttemptRetention: time.Duration(a.cfg.WebhookAttemptRetentionDays) * 24 * time.Hour,
	})
	// ADR 0019 F: an auto-disabled endpoint notifies the owners (panel + email).
	if a.Notify != nil {
		deliverer.OnEndpointDisabled(a.Notify.WebhookEndpointDisabled)
	}
}

// seasonJobs are the background jobs of the M5A services (webhook delivery
// log retention).
func (a *App) seasonJobs() []jobs.Job { return a.Webhooks.Jobs() }
