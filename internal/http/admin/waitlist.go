package admin

import (
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/waitlist"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B waitlist (spec 10.5): owned by the builder of internal/domain/waitlist
// (Handlers.Waitlist). Responses carry customer names: no-store.

// ListWaitlistEntries implements GET /waitlist.
func (h *Handlers) ListWaitlistEntries(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListWaitlistEntriesParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := waitlist.Filter{ZoneID: customerStr(params.ZoneId), PackID: customerStr(params.PackId), From: params.From, To: params.To, Query: customerStr(params.Q)}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Waitlist.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.WaitlistEntryList{Items: make([]api.WaitlistEntry, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, waitlistEntryToAPI(&page.Items[i]))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// CreateWaitlistEntry implements POST /waitlist.
func (h *Handlers) CreateWaitlistEntry(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateWaitlistEntryParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	var req api.WaitlistEntryCreate
	if !decode(w, r, &req) {
		return
	}
	in := waitlist.Input{
		CustomerID: req.CustomerId, StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots),
		PackID: req.PackId, ZoneID: customerStr(req.ZoneId), SpotID: customerStr(req.SpotId), Note: customerStr(req.Note),
		IP: customerClientIP(r),
	}
	if req.Quantity != nil {
		in.Quantity = *req.Quantity
	}
	if req.Channel != nil {
		in.Channel = string(*req.Channel)
	}
	e, err := h.Waitlist.Create(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusCreated, e.Version, waitlistEntryToAPI(e))
}

// GetWaitlistEntry implements GET /waitlist/{entryId}.
func (h *Handlers) GetWaitlistEntry(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.WaitlistEntryID) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	e, offers, err := h.Waitlist.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.WaitlistEntryDetail{Entry: waitlistEntryToAPI(e), Offers: make([]api.WaitlistOffer, 0, len(offers))}
	for i := range offers {
		out.Offers = append(out.Offers, waitlistOfferToAPI(&offers[i]))
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, e.Version, out)
}

// UpdateWaitlistEntry implements PATCH /waitlist/{entryId}.
func (h *Handlers) UpdateWaitlistEntry(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.WaitlistEntryID, params api.UpdateWaitlistEntryParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.WaitlistEntryUpdate
	if !decode(w, r, &req) {
		return
	}
	var slots *calendar.Slots
	if req.Slots != nil {
		sl := calendar.Slots(*req.Slots)
		slots = &sl
	}
	in := waitlist.Patch{
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: slots, PackID: req.PackId, ZoneID: req.ZoneId,
		SpotID: req.SpotId, Quantity: req.Quantity, Priority: req.Priority, Note: req.Note, IP: customerClientIP(r),
	}
	e, err := h.Waitlist.Update(r.Context(), principalOf(r), establishmentID, id, ifMatch, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, e.Version, waitlistEntryToAPI(e))
}

// RemoveWaitlistEntry implements DELETE /waitlist/{entryId}.
func (h *Handlers) RemoveWaitlistEntry(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.WaitlistEntryID, params api.RemoveWaitlistEntryParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Waitlist.Remove(r.Context(), principalOf(r), establishmentID, id, ifMatch, customerClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderWaitlist implements POST /waitlist/reorder.
func (h *Handlers) ReorderWaitlist(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.ReorderWaitlistParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	var req api.WaitlistReorderRequest
	if !decode(w, r, &req) {
		return
	}
	if err := h.Waitlist.Reorder(r.Context(), principalOf(r), establishmentID, req.EntryIds, customerClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// OfferWaitlistEntry implements POST /waitlist/{entryId}/offer.
func (h *Handlers) OfferWaitlistEntry(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.WaitlistEntryID, _ api.OfferWaitlistEntryParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	var req api.WaitlistOfferCreate
	if !decodeOptional(w, r, &req) {
		return
	}
	var hold time.Duration
	if req.HoldMinutes != nil {
		hold = time.Duration(*req.HoldMinutes) * time.Minute
	}
	o, err := h.Waitlist.Offer(r.Context(), principalOf(r), establishmentID, id, customerStr(req.SpotId), hold, customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, waitlistOfferToAPI(o))
}

// RevokeWaitlistOffer implements POST /waitlist/{entryId}/offers/{offerId}/revoke.
func (h *Handlers) RevokeWaitlistOffer(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.WaitlistEntryID, offerID api.WaitlistOfferID, _ api.RevokeWaitlistOfferParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	o, err := h.Waitlist.RevokeOffer(r.Context(), principalOf(r), establishmentID, id, offerID, customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, waitlistOfferToAPI(o))
}

// GetWaitlistSettings implements GET /waitlist-settings.
func (h *Handlers) GetWaitlistSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	s, err := h.Waitlist.GetSettings(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, waitlistSettingsToAPI(s))
}

// UpdateWaitlistSettings implements PUT /waitlist-settings.
func (h *Handlers) UpdateWaitlistSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateWaitlistSettingsParams) {
	if h.Waitlist == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.WaitlistSettingsInput
	if !decode(w, r, &req) {
		return
	}
	s, err := h.Waitlist.UpdateSettings(r.Context(), principalOf(r), establishmentID, ifMatch, waitlist.Settings{
		Enabled: req.Enabled, PublicJoin: req.PublicJoin, AutoOffer: req.AutoOffer, OfferHoldMinutes: req.OfferHoldMinutes,
	}, customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, waitlistSettingsToAPI(s))
}

func waitlistEntryToAPI(e *waitlist.Entry) api.WaitlistEntry {
	out := api.WaitlistEntry{
		Id: e.ID, Status: api.WaitlistEntryStatus(e.Status), Channel: api.Channel(e.Channel), CustomerId: e.CustomerID,
		CustomerDisplayName: e.CustomerDisplayName, StartsOn: e.StartsOn, EndsOn: e.EndsOn, Slots: api.Slots(e.Slots),
		PackId: e.PackID, ZoneId: e.ZoneID, SpotId: e.SpotID, Quantity: e.Quantity, Priority: e.Priority,
		Position: e.Position, OfferCount: e.OfferCount, Version: e.Version, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
	if e.Note != "" {
		note := e.Note
		out.Note = &note
	}
	if e.ActiveOffer != nil {
		o := waitlistOfferToAPI(e.ActiveOffer)
		out.ActiveOffer = &o
	}
	return out
}

func waitlistOfferToAPI(o *waitlist.Offer) api.WaitlistOffer {
	out := api.WaitlistOffer{
		Id: o.ID, EntryId: o.EntryID, Status: api.WaitlistOfferStatus(o.Status), HoldId: o.HoldID, SpotId: o.SpotID,
		BookingId: o.BookingID, ExpiresAt: o.ExpiresAt.UTC(), RespondedAt: o.RespondedAt, CreatedAt: o.CreatedAt,
	}
	if o.Cause != "" {
		c := api.WaitlistOfferCause(o.Cause)
		out.Cause = &c
	}
	if o.SpotLabel != "" {
		l := o.SpotLabel
		out.SpotLabel = &l
	}
	return out
}

func waitlistSettingsToAPI(s *waitlist.Settings) api.WaitlistSettings {
	return api.WaitlistSettings{
		Enabled: s.Enabled, PublicJoin: s.PublicJoin, AutoOffer: s.AutoOffer, OfferHoldMinutes: s.OfferHoldMinutes, Version: s.Version,
	}
}
