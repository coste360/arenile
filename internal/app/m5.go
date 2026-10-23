package app

import (
	"github.com/coste360/arenile/internal/domain/apikey"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/domain/webhook"
)

// wireSeason builds the M5 part A services (reports, API keys, outgoing
// webhook management) on top of the M0-M4 services. They have no
// dependencies on each other.
//
// Webhooks.Deliverer is left nil here: the integrations builder adds
// event.Deliverer.AttemptNow, then builds the Deliverer before this call
// (it is created in app.go after wireOnline today) and passes it, with the
// compile-time check
//
//	var _ webhook.Deliverer = (*event.Deliverer)(nil)
func (a *App) wireSeason(aw *audit.Writer) {
	c := a.opts.clock
	a.Reports = report.NewService(report.Deps{DB: a.db, Clock: c, Log: a.log})
	a.APIKeys = apikey.NewService(apikey.Deps{DB: a.db, Clock: c, Audit: aw, Log: a.log})
	a.Webhooks = webhook.NewService(webhook.Deps{DB: a.db, Clock: c, Audit: aw, Keys: a.keys, Log: a.log})
}
