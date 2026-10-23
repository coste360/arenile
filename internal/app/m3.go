package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/customer"
	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/domain/payment"
	"github.com/coste360/arenile/internal/jobs"
)

// Compile-time checks of the M3 ports (ports & adapters): each consumer
// package declares the interface it needs, the provider implements it, and
// this composition root is the only place that knows both.
var (
	_ mapping.AllocationChecker = (*inventory.Service)(nil)
	_ booking.Inventory         = (*inventory.Service)(nil)
	_ booking.Customers         = (*customer.Service)(nil)
	_ booking.Payments          = (*payment.Service)(nil)
	_ booking.Releases          = (*pass.Resale)(nil)
	_ payment.BookingLedger     = (*booking.Ledger)(nil)
	_ payment.CashDesk          = (*cash.Service)(nil)
	_ payment.Fiscal            = (*fiscal.Service)(nil)
	_ payment.Credits           = (*customer.Service)(nil)
	_ payment.Providers         = (*Registry)(nil)
	_ fiscal.Drivers            = (*Registry)(nil)
	_ pass.Bookings             = (*booking.Service)(nil)
	_ pass.Inventory            = (*inventory.Service)(nil)
	_ pass.Credits              = (*customer.Service)(nil)
)

// wireOperations builds the M3 services (bookings from the panel and cash
// desk). Construction order avoids cycles: the stateless adapters
// booking.Ledger and pass.Resale break the payment <-> booking and
// booking <-> pass dependencies. a.Inventory is built by wireConfiguration
// (mapping uses it as AllocationChecker) and a.registry must exist.
func (a *App) wireOperations(aw *audit.Writer) {
	c := a.opts.clock
	a.Customers = customer.NewService(customer.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log})
	a.Cash = cash.NewService(cash.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log})
	a.Fiscal = fiscal.NewService(fiscal.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log, Drivers: a.registry})
	a.Payments = payment.NewService(payment.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Bookings: booking.NewLedger(booking.LedgerDeps{Clock: c}), Cash: a.Cash, Fiscal: a.Fiscal,
		Credits: a.Customers, Providers: a.registry,
	})
	a.Bookings = booking.NewService(booking.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Inventory: a.Inventory, Pricer: a.Pricing, Customers: a.Customers, Payments: a.Payments,
		Releases: pass.NewResale(pass.ResaleDeps{Clock: c, Credits: a.Customers, Log: a.log}),
		HoldTTL:  a.cfg.HoldTTL,
	})
	a.Passes = pass.NewService(pass.Deps{
		DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log,
		Bookings: a.Bookings, Inventory: a.Inventory, Credits: a.Customers,
	})
}

// operationJobs are the background jobs contributed by the M3 services.
func (a *App) operationJobs() []jobs.Job {
	var out []jobs.Job
	out = append(out, a.Inventory.Jobs()...)
	out = append(out, a.Bookings.Jobs()...)
	return out
}
