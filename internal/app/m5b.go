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
//   - credit: customer.Service (payment/pass Credits port) delegates to it
//     (UseCreditLedger); Payments = a.Payments for the magic-link payments.
//
// Jobs and sinks are registered through renewalJobs / renewalSinks below
// (update wantJobs in app_test.go when adding one).
func (a *App) wireRenewals(aw *audit.Writer) {
	c := a.opts.clock
	// Credit ledger (M5B): customer.Service, the Credits port of the payment
	// and pass services, delegates GrantCredit / CreditBalance to it, so
	// every movement maintains customer_credit_balances (conditional
	// UPDATE, never negative) without rewiring those services.
	a.Credits = credit.NewService(credit.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Payments: a.Payments})
	a.Customers.UseCreditLedger(a.Credits)
	a.Rollover = rollover.NewService(rollover.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Copier: a.Pricing})
	a.Renewals = renewal.NewService(renewal.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Bookings: a.Bookings, Checkout: a.Checkout, Notify: a.Notify, Credits: a.Credits,
		CreditApplier: a.Credits,
	})
	a.Waitlist = waitlist.NewService(waitlist.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Notify: a.Notify, BaseURL: a.cfg.BaseURL,
		Holds: a.Inventory, Checkout: a.Checkout, Customers: a.Customers, Terms: a.Online,
	})
}

// renewalJobs are the background jobs of the M5B services (none until
// implemented: credit.JobExpire, renewal.JobSync, waitlist.JobExpire).
func (a *App) renewalJobs() []jobs.Job {
	var out []jobs.Job
	out = append(out, a.Renewals.Jobs()...)
	out = append(out, a.Credits.Jobs()...)
	out = append(out, a.Waitlist.Jobs()...)
	return out
}

// renewalSinks are the in-process event sinks of the M5B services: the
// waitlist offers freed inventory and forgets anonymised customers; the
// renewals convert a proposal as soon as its booking is confirmed.
func (a *App) renewalSinks() []ext.EventSink { return []ext.EventSink{a.Waitlist, a.Renewals} }
