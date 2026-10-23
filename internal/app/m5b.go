package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/credit"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/pkg/ext"
)

// wireRenewals builds the M5 part B services (season rollover, renewals,
// waitlist, customer credits) on top of the M0-M5A services. This file is
// the only wiring file the M5B builders edit:
//   - rollover: Copier = a.Pricing once pricingconfig has CopySeasonInTx;
//   - renewal: Bookings = a.Bookings (+ booking ExtendHold), Checkout =
//     a.Checkout (+ ResolveManage, PayManagedAmount), Notify = a.Notify;
//   - waitlist: Holds = a.Inventory (+ CreateHoldInTx, ReleaseHoldInTx),
//     Checkout = a.Checkout (+ CreateFromHold), Notify = a.Notify, BaseURL;
//   - credit: replace customer.Service as payment/pass Credits port
//     (a.Payments / a.Passes deps) once GrantCredit maintains the balance.
//
// Jobs and sinks are registered through renewalJobs / renewalSinks below
// (update wantJobs in app_test.go when adding one).
func (a *App) wireRenewals(aw *audit.Writer) {
	c := a.opts.clock
	a.Credits = credit.NewService(credit.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log})
	a.Rollover = rollover.NewService(rollover.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log})
	a.Renewals = renewal.NewService(renewal.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Notify: a.Notify})
	a.Waitlist = waitlist.NewService(waitlist.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Notify: a.Notify, BaseURL: a.cfg.BaseURL,
	})
}

// renewalJobs are the background jobs of the M5B services (none until
// implemented: credit.JobExpire, renewal.JobSync, waitlist.JobExpire).
func (a *App) renewalJobs() []jobs.Job { return nil }

// renewalSinks are the in-process event sinks of the M5B services (the
// waitlist service once its Handle is implemented).
func (a *App) renewalSinks() []ext.EventSink { return nil }
