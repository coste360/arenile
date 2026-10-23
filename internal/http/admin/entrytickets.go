package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Entry ticket handlers: ticket types and entry sales (spec 14.3, ADR 0020).
// Owner: internal/domain/ticketing. Contract scaffolding: every operation
// answers 501 not_implemented until wired.

// ListEntryTicketTypes implements GET .../entry-ticket-types.
func (h *Handlers) ListEntryTicketTypes(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateEntryTicketType implements POST .../entry-ticket-types.
func (h *Handlers) CreateEntryTicketType(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateEntryTicketTypeParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetEntryTicketType implements GET .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) GetEntryTicketType(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.EntryTicketTypeID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateEntryTicketType implements PATCH .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) UpdateEntryTicketType(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.EntryTicketTypeID, _ api.UpdateEntryTicketTypeParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeleteEntryTicketType implements DELETE .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) DeleteEntryTicketType(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.EntryTicketTypeID, _ api.DeleteEntryTicketTypeParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// QuoteEntrySale implements POST .../entry-sales/quote.
func (h *Handlers) QuoteEntrySale(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.QuoteEntrySaleParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateEntrySale implements POST .../entry-sales.
func (h *Handlers) CreateEntrySale(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateEntrySaleParams) {
	problem.Write(w, r, problem.NotImplemented)
}
