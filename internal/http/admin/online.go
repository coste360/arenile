package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Online settings of the public page (M4, spec 12). Owner: online settings
// builder (GetOnlineSettings, UpdateOnlineSettings, ListTermsVersions,
// PublishTermsVersion -> h.Online) and notifications builder
// (ResendBookingConfirmation, RegenerateMagicLink -> h.Notify).
//
//	GetOnlineSettings         -> Online.Get            -> api.OnlineSettings + ETag
//	UpdateOnlineSettings      -> Online.Update         -> api.OnlineSettings + ETag (If-Match, 412 version_conflict)
//	ListTermsVersions         -> Online.ListTerms      -> api.TermsVersionList
//	PublishTermsVersion       -> Online.PublishTerms   -> 201 api.TermsVersion
//	ResendBookingConfirmation -> Notify.ResendConfirmation -> 202 api.MessageQueued
//	RegenerateMagicLink       -> Notify.RegenerateLink -> 201 api.MagicLinkResult (Cache-Control no-store)

// GetOnlineSettings implements GET /api/admin/v1/establishments/{establishmentId}/online-settings.
func (h *Handlers) GetOnlineSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateOnlineSettings implements PUT /api/admin/v1/establishments/{establishmentId}/online-settings.
func (h *Handlers) UpdateOnlineSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateOnlineSettingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListTermsVersions implements GET /api/admin/v1/establishments/{establishmentId}/terms-versions.
func (h *Handlers) ListTermsVersions(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// PublishTermsVersion implements POST /api/admin/v1/establishments/{establishmentId}/terms-versions.
func (h *Handlers) PublishTermsVersion(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.PublishTermsVersionParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ResendBookingConfirmation implements POST .../bookings/{bookingId}/resend-confirmation.
func (h *Handlers) ResendBookingConfirmation(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.ResendBookingConfirmationParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RegenerateMagicLink implements POST .../bookings/{bookingId}/magic-link.
func (h *Handlers) RegenerateMagicLink(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, bookingID api.BookingID, params api.RegenerateMagicLinkParams) {
	problem.Write(w, r, problem.NotImplemented)
}
