package app

import (
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/findspot"
	"github.com/coste360/arenile/internal/domain/partner"
)

// wireM8 builds the M8 services (spec 30, ADR 0028) on top of the M3
// operations: partners, customer categories and the partner account
// (internal/domain/partner), and "Trova posto" (internal/domain/findspot),
// which books through the booking service. The other M8 use cases are
// methods of existing services (booking: swap, group move, undo, duplicate,
// requirement verification; inventory: spot calendar, lifeguard list;
// report: partner report, statement, agreements register).
//
// The find-spot builder adds the Occupancy (inventory) and Quoter (pricing)
// adapters here; until then Search answers 501.
func (a *App) wireM8(aw *audit.Writer) {
	c := a.opts.clock
	a.Partners = partner.NewService(partner.Deps{DB: a.db, Clock: c, Audit: aw, Outbox: a.Outbox, Log: a.log})
	a.SpotFinder = findspot.NewService(findspot.Deps{Clock: c, Booker: a.Bookings, Log: a.log})
}
