package httpserver

import "github.com/coste360/arenile/internal/domain/ticketassign"

// newTicketAssign builds the ticket assignment service (daily sale,
// ADR 0020) from the booking service already in Deps: it needs no wiring of
// its own in internal/app. Without a booking service (routing tests) the
// reads still work.
func newTicketAssign(d Deps) *ticketassign.Service {
	var b ticketassign.Bookings
	if d.Bookings != nil {
		b = d.Bookings
	}
	return ticketassign.NewService(ticketassign.Deps{DB: d.DB, Clock: d.Clock, Bookings: b})
}
