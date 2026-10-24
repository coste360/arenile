package admin

import (
	"context"
	"net/http"

	"github.com/coste360/arenile/internal/domain/auth"

	"github.com/coste360/arenile/internal/domain/inventory"
	"github.com/coste360/arenile/internal/domain/ticketassign"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Ticket assignment handlers (zone-first daily sales, spec 14.3, ADR 0020):
// the "Da assegnare" list, the automatic fill, assign and move. Owner:
// internal/domain/ticketassign (Handlers.TicketAssign).

// ListUnassignedTickets implements GET .../tickets/unassigned.
func (h *Handlers) ListUnassignedTickets(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListUnassignedTicketsParams) {
	zoneID := ""
	if params.ZoneId != nil {
		zoneID = *params.ZoneId
	}
	vs, err := h.TicketAssign.ListUnassigned(r.Context(), principalOf(r), establishmentID, params.Date, zoneID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.UnassignedTicketList{Items: make([]api.Ticket, 0, len(vs))}
	for i := range vs {
		out.Items = append(out.Items, TicketToAPI(&vs[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// AutoAssignTickets implements POST .../tickets/auto-assign.
func (h *Handlers) AutoAssignTickets(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.AutoAssignTicketsParams) {
	var req api.TicketAutoAssignRequest
	if !decode(w, r, &req) {
		return
	}
	in := ticketassign.AutoInput{Date: req.Date, DryRun: pcBoolOr(req.DryRun, false)}
	if req.ZoneId != nil {
		in.ZoneID = *req.ZoneId
	}
	res, err := h.TicketAssign.AutoAssign(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.TicketAutoAssignResult{
		Applied:    res.Applied,
		Assigned:   make([]api.TicketAutoAssignment, 0, len(res.Assigned)),
		Unassigned: make([]api.TicketAutoUnassigned, 0, len(res.Unassigned)),
	}
	for _, a := range res.Assigned {
		out.Assigned = append(out.Assigned, api.TicketAutoAssignment{TicketId: a.TicketID, TicketNumber: a.TicketNumber, SpotId: a.SpotID, SpotLabel: a.SpotLabel})
	}
	for _, u := range res.Unassigned {
		out.Unassigned = append(out.Unassigned, api.TicketAutoUnassigned{TicketId: u.TicketID, TicketNumber: u.TicketNumber, Reason: api.TicketAutoUnassignedReason(u.Reason)})
	}
	writeJSON(w, http.StatusOK, out)
}

// AssignTicket implements POST .../tickets/{ticketId}/assign.
func (h *Handlers) AssignTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, ticketID api.TicketID, _ api.AssignTicketParams) {
	h.placeTicket(w, r, establishmentID, ticketID, h.TicketAssign.Assign)
}

// MoveTicket implements POST .../tickets/{ticketId}/move.
func (h *Handlers) MoveTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, ticketID api.TicketID, _ api.MoveTicketParams) {
	h.placeTicket(w, r, establishmentID, ticketID, h.TicketAssign.Move)
}

type placeFunc func(ctx context.Context, p *auth.Principal, establishmentID, ticketID string, in ticketassign.Input) (*ticketing.View, error)

func (h *Handlers) placeTicket(w http.ResponseWriter, r *http.Request, establishmentID, ticketID string, place placeFunc) {
	var req api.TicketAssignRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := place(r.Context(), principalOf(r), establishmentID, ticketID, ticketassign.Input{SpotID: req.SpotId, ExpectedVersion: req.Version})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Ticket.Version, TicketToAPI(v))
}

// opTicketToAPI maps the ticket of a booking on the day map.
func opTicketToAPI(t *inventory.OperationalTicket) *api.OperationalTicket {
	if t == nil {
		return nil
	}
	return &api.OperationalTicket{Number: t.Number, Label: t.Label, NeedsAssignment: t.NeedsAssignment}
}
