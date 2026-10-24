package httpserver

import (
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/ticketing"
)

// newDailySale builds the daily sale and ticketing services (ADR 0020) from
// the services already in Deps: they need no wiring of their own in
// internal/app. Without the operations services (routing tests) the reads
// and the input validation still work; the sales need them all.
func newDailySale(d Deps) (*dailysale.Service, *ticketing.Service) {
	aw := audit.NewWriter(d.Clock)
	td := ticketing.Deps{DB: d.DB, Clock: d.Clock, Audit: aw, Log: d.Log}
	sd := dailysale.Deps{DB: d.DB, Clock: d.Clock, Audit: aw, Log: d.Log}
	if d.Bookings != nil {
		td.Bookings = d.Bookings
		td.BookingsIn = func(db bun.IDB) ticketing.Bookings { return d.Bookings.WithDB(db) }
		sd.Bookings = d.Bookings
	}
	if d.Payments != nil {
		sd.Payments = dailysale.PaymentRecorder{Svc: d.Payments}
	}
	if d.Customers != nil {
		sd.Customers = d.Customers
	}
	if d.Inventory != nil {
		sd.AvailabilityIn = func(db bun.IDB) dailysale.Availability { return d.Inventory.WithDB(db) }
	}
	tickets := ticketing.NewService(td)
	sd.Tickets = tickets
	return dailysale.NewService(sd), tickets
}
