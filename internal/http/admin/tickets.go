package admin

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Ticket handlers: daily list, print data, void and the entry gate (spec
// 14.3, ADR 0020). Owner: internal/domain/ticketing. Assignment (unassigned
// list, auto-assign, assign, move) lives in tickets_assign.go.

// ListTickets implements GET .../tickets (JSON or CSV).
func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListTicketsParams) {
	csvOut := strings.Contains(r.Header.Get("Accept"), "text/csv")
	if params.Format != nil {
		csvOut = *params.Format == api.ReportFormatCSV
	}
	in := ticketing.ListInput{Date: params.Date}
	if params.State != nil {
		in.State = ticketing.State(*params.State)
	}
	if params.Kind != nil {
		in.Kind = string(*params.Kind)
	}
	if params.ZoneId != nil {
		in.ZoneID = *params.ZoneId
	}
	if csvOut {
		in.Limit = -1
	} else {
		cursor, limit, err := pageParams(params.Cursor, params.Limit)
		if err != nil {
			problem.Error(w, r, h.Log, err)
			return
		}
		in.Cursor, in.Limit = cursor, limit
	}
	l, err := h.Tickets.List(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if !csvOut {
		writeJSON(w, http.StatusOK, TicketListToAPI(l))
		return
	}
	var buf bytes.Buffer
	if err := WriteTicketsCSV(&buf, l.Items, "it"); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="biglietti_`+params.Date.String()+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// GetTicketByCode implements GET .../tickets/by-code/{bookingCode}.
func (h *Handlers) GetTicketByCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, code api.BookingCode) {
	v, err := h.Tickets.GetByCode(r.Context(), principalOf(r), establishmentID, code)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, TicketToAPI(v))
}

// GetTicket implements GET .../tickets/{ticketId}.
func (h *Handlers) GetTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.TicketID) {
	v, err := h.Tickets.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, TicketToAPI(v))
}

// GetTicketPrint implements GET .../tickets/{ticketId}/print.
func (h *Handlers) GetTicketPrint(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.TicketID, params api.GetTicketPrintParams) {
	format := ""
	if params.Format != nil {
		format = string(*params.Format)
	}
	pd, err := h.Tickets.Print(r.Context(), principalOf(r), establishmentID, id, format)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, TicketPrintToAPI(pd))
}

// ReprintTicket implements POST .../tickets/{ticketId}/reprint.
func (h *Handlers) ReprintTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.TicketID, _ api.ReprintTicketParams) {
	var req api.TicketReprintRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	format := ""
	if req.Format != nil {
		format = string(*req.Format)
	}
	pd, err := h.Tickets.Reprint(r.Context(), principalOf(r), establishmentID, id, format)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, TicketPrintToAPI(pd))
}

// VoidTicket implements POST .../tickets/{ticketId}/void.
// If-Match with the ticket version is mandatory (428 / 412, INV-80), like
// the booking cancellation from the map.
func (h *Handlers) VoidTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.TicketID, params api.VoidTicketParams) {
	expected, err := bkRequireIfMatch(&params.IfMatch, false)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.TicketVoidRequest
	if !decode(w, r, &req) {
		return
	}
	in := ticketing.VoidInput{Reason: req.Reason, ExpectedVersion: expected}
	if req.Refund != nil {
		in.Refund = string(*req.Refund)
	}
	v, err := h.Tickets.Void(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, v.Ticket.Version, TicketToAPI(v))
}

// CheckInEntryTicket implements POST .../entry-gate/check-in.
func (h *Handlers) CheckInEntryTicket(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CheckInEntryTicketParams) {
	var req api.GateCheckInRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.Tickets.CheckIn(r.Context(), principalOf(r), establishmentID, req.Code, pcBoolOr(req.Manual, false))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.GateCheckInResult{Outcome: api.GateCheckInResultOutcome(res.Outcome)}
	if res.View != nil {
		t := TicketToAPI(res.View)
		out.Ticket = &t
	}
	writeJSON(w, http.StatusOK, out)
}
