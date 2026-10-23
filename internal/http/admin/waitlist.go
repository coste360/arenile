package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// M5B waitlist (spec 10.5): owned by the builder of internal/domain/waitlist
// (Handlers.Waitlist). 501 until implemented.

// ListWaitlistEntries implements GET /waitlist.
func (h *Handlers) ListWaitlistEntries(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListWaitlistEntriesParams) {
	notImplemented(w, r)
}

// CreateWaitlistEntry implements POST /waitlist.
func (h *Handlers) CreateWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateWaitlistEntryParams) {
	notImplemented(w, r)
}

// GetWaitlistEntry implements GET /waitlist/{entryId}.
func (h *Handlers) GetWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WaitlistEntryID) {
	notImplemented(w, r)
}

// UpdateWaitlistEntry implements PATCH /waitlist/{entryId}.
func (h *Handlers) UpdateWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WaitlistEntryID, _ api.UpdateWaitlistEntryParams) {
	notImplemented(w, r)
}

// RemoveWaitlistEntry implements DELETE /waitlist/{entryId}.
func (h *Handlers) RemoveWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WaitlistEntryID, _ api.RemoveWaitlistEntryParams) {
	notImplemented(w, r)
}

// ReorderWaitlist implements POST /waitlist/reorder.
func (h *Handlers) ReorderWaitlist(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ReorderWaitlistParams) {
	notImplemented(w, r)
}

// OfferWaitlistEntry implements POST /waitlist/{entryId}/offer.
func (h *Handlers) OfferWaitlistEntry(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WaitlistEntryID, _ api.OfferWaitlistEntryParams) {
	notImplemented(w, r)
}

// RevokeWaitlistOffer implements POST /waitlist/{entryId}/offers/{offerId}/revoke.
func (h *Handlers) RevokeWaitlistOffer(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.WaitlistEntryID, _ api.WaitlistOfferID, _ api.RevokeWaitlistOfferParams) {
	notImplemented(w, r)
}

// GetWaitlistSettings implements GET /waitlist-settings.
func (h *Handlers) GetWaitlistSettings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	notImplemented(w, r)
}

// UpdateWaitlistSettings implements PUT /waitlist-settings.
func (h *Handlers) UpdateWaitlistSettings(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.UpdateWaitlistSettingsParams) {
	notImplemented(w, r)
}
