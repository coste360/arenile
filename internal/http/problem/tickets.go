package problem

import (
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
)

// Daily sale and ticket type slugs (spec 14.3, ADR 0020).
const (
	ZoneFull             = api.ProblemType("zone_full")
	AssignmentConflict   = api.ProblemType("assignment_conflict")
	TicketNumberConflict = api.ProblemType("ticket_number_conflict")
	TicketVoided         = api.ProblemType("ticket_voided")
	TicketAlreadyEntered = api.ProblemType("ticket_already_entered")
	PriceChanged         = api.ProblemType("price_changed")
)

// Registered in their own file so the main catalog stays untouched.
func init() {
	catalog[ZoneFull] = meta{http.StatusConflict, "Nessun posto libero nella zona per la fascia scelta"}
	catalog[AssignmentConflict] = meta{http.StatusConflict, "La postazione è stata appena occupata: scegline un'altra"}
	catalog[TicketNumberConflict] = meta{http.StatusConflict, "Numero del biglietto già usato, riprova"}
	catalog[TicketVoided] = meta{http.StatusConflict, "Il biglietto è annullato"}
	catalog[TicketAlreadyEntered] = meta{http.StatusConflict, "Il biglietto è già stato usato per entrare"}
	catalog[PriceChanged] = meta{http.StatusConflict, "Il prezzo è cambiato: controlla il nuovo totale"}
}

// writeTicketing writes the problem of a ticketing error and reports
// whether err was one.
func writeTicketing(w http.ResponseWriter, r *http.Request, err error) bool {
	var typ api.ProblemType
	switch {
	case errors.Is(err, ticketing.ErrZoneFull):
		typ = ZoneFull
	case errors.Is(err, ticketing.ErrAssignmentConflict):
		typ = AssignmentConflict
	case errors.Is(err, ticketing.ErrNumberConflict):
		typ = TicketNumberConflict
	case errors.Is(err, ticketing.ErrVoided):
		typ = TicketVoided
	case errors.Is(err, ticketing.ErrAlreadyEntered):
		typ = TicketAlreadyEntered
	case errors.Is(err, ticketing.ErrPriceChanged):
		typ = PriceChanged
	default:
		return false
	}
	Write(w, r, typ)
	return true
}
