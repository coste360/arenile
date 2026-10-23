package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Ticket handlers: daily list, print data, void, assignment and the entry
// gate (spec 14.3, ADR 0020). Owner: internal/domain/ticketing. Contract
// scaffolding: every operation answers 501 not_implemented until wired.

// ListTickets implements GET .../tickets (JSON or CSV).
func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListTicketsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListUnassignedTickets implements GET .../tickets/unassigned.
func (h *Handlers) ListUnassignedTickets(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListUnassignedTicketsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// AutoAssignTickets implements POST .../tickets/auto-assign.
func (h *Handlers) AutoAssignTickets(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.AutoAssignTicketsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetTicketByCode implements GET .../tickets/by-code/{bookingCode}.
func (h *Handlers) GetTicketByCode(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.BookingCode) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetTicket implements GET .../tickets/{ticketId}.
func (h *Handlers) GetTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetTicketPrint implements GET .../tickets/{ticketId}/print.
func (h *Handlers) GetTicketPrint(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID, _ api.GetTicketPrintParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ReprintTicket implements POST .../tickets/{ticketId}/reprint.
func (h *Handlers) ReprintTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID, _ api.ReprintTicketParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// VoidTicket implements POST .../tickets/{ticketId}/void.
func (h *Handlers) VoidTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID, _ api.VoidTicketParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// AssignTicket implements POST .../tickets/{ticketId}/assign.
func (h *Handlers) AssignTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID, _ api.AssignTicketParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// MoveTicket implements POST .../tickets/{ticketId}/move.
func (h *Handlers) MoveTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TicketID, _ api.MoveTicketParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CheckInEntryTicket implements POST .../entry-gate/check-in.
func (h *Handlers) CheckInEntryTicket(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CheckInEntryTicketParams) {
	problem.Write(w, r, problem.NotImplemented)
}
