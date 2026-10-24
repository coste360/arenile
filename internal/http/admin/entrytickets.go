package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"

	"github.com/coste360/arenile/internal/domain/dailysale"
	"github.com/coste360/arenile/internal/domain/ticketing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Entry ticket handlers: ticket types (internal/domain/ticketing) and entry
// sales (internal/domain/dailysale), spec 14.3, ADR 0020.

// ListEntryTicketTypes implements GET .../entry-ticket-types.
func (h *Handlers) ListEntryTicketTypes(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	ts, err := h.Tickets.ListEntryTypes(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.EntryTicketTypeList{Items: make([]api.EntryTicketType, 0, len(ts))}
	for i := range ts {
		out.Items = append(out.Items, EntryTicketTypeToAPI(&ts[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateEntryTicketType implements POST .../entry-ticket-types.
func (h *Handlers) CreateEntryTicketType(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateEntryTicketTypeParams) {
	var req api.EntryTicketTypeInput
	if !decode(w, r, &req) {
		return
	}
	in := ticketing.EntryTypeInput{
		Code: req.Code, Name: store.I18nText(req.Name), Description: catalogI18nFromAPI(req.Description),
		PackID: req.PackId, ZoneID: req.ZoneId, Active: pcBoolOr(req.Active, true),
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	t, err := h.Tickets.CreateEntryType(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, t.Version, EntryTicketTypeToAPI(t))
}

// GetEntryTicketType implements GET .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) GetEntryTicketType(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.EntryTicketTypeID) {
	t, err := h.Tickets.GetEntryType(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, t.Version, EntryTicketTypeToAPI(t))
}

// UpdateEntryTicketType implements PATCH .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) UpdateEntryTicketType(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.EntryTicketTypeID, params api.UpdateEntryTicketTypeParams) {
	im, err := RequireIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.EntryTicketTypeUpdate
	if !decode(w, r, &req) {
		return
	}
	in := ticketing.EntryTypePatch{PackID: req.PackId, ZoneID: req.ZoneId, SortOrder: req.SortOrder, Active: req.Active}
	if !im.Any {
		v := im.Version
		in.ExpectedVersion = &v
	}
	if req.Name != nil {
		n := store.I18nText(*req.Name)
		in.Name = &n
	}
	if req.Description != nil {
		d := store.I18nText(*req.Description)
		in.Description = &d
	}
	t, err := h.Tickets.UpdateEntryType(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, t.Version, EntryTicketTypeToAPI(t))
}

// DeleteEntryTicketType implements DELETE .../entry-ticket-types/{entryTicketTypeId}.
func (h *Handlers) DeleteEntryTicketType(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.EntryTicketTypeID, _ api.DeleteEntryTicketTypeParams) {
	if err := h.Tickets.DeleteEntryType(r.Context(), principalOf(r), establishmentID, id); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// QuoteEntrySale implements POST .../entry-sales/quote.
func (h *Handlers) QuoteEntrySale(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.QuoteEntrySaleParams) {
	var req api.EntrySaleQuoteRequest
	if !decode(w, r, &req) {
		return
	}
	q, err := h.DailySale.QuoteEntry(r.Context(), principalOf(r), establishmentID, dailysale.EntryInput{
		Date: req.Date, Slots: calendar.Slots(req.Slots), Items: entryItems(req.Items),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.EntrySaleQuote{
		Breakdown: BreakdownToAPI(q.Result.Breakdown), Total: int64(q.Result.Breakdown.Total), Available: q.Available,
		Remaining: q.Remaining, Conflicts: bkConflicts(q.Conflicts),
	})
}

// CreateEntrySale implements POST .../entry-sales.
func (h *Handlers) CreateEntrySale(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateEntrySaleParams) {
	var req api.EntrySaleCreate
	if !decode(w, r, &req) {
		return
	}
	in := dailysale.EntryInput{
		Date: req.Date, Slots: calendar.Slots(req.Slots), Items: entryItems(req.Items), Payment: salePayment(req.Payment),
		ExpectedTotal: centsPtr(req.ExpectedTotal),
	}
	if req.Holder != nil {
		in.FirstName, in.LastName = paymentDeref(req.Holder.FirstName), paymentDeref(req.Holder.LastName)
	}
	res, err := h.DailySale.CreateEntry(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	h.writeSaleResult(w, r, establishmentID, res)
}

func entryItems(xs []api.EntrySaleItem) []dailysale.EntryItem {
	out := make([]dailysale.EntryItem, 0, len(xs))
	for _, x := range xs {
		out = append(out, dailysale.EntryItem{TypeID: x.EntryTicketTypeId, Quantity: x.Quantity})
	}
	return out
}
