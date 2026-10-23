package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Email settings and message log (M4, spec 19). Owners: online settings
// builder (GetEmailSettings, UpdateEmailSettings -> h.Online) and
// notifications builder (SendTestEmail, ListMessages -> h.Notify).
//
//	GetEmailSettings    -> Online.GetEmail     -> api.EmailSettings + ETag
//	UpdateEmailSettings -> Online.UpdateEmail  -> api.EmailSettings + ETag (If-Match)
//	SendTestEmail       -> Notify.SendTest     -> 202 api.MessageQueued
//	ListMessages        -> Notify.ListMessages -> api.MessageLogList (recipients masked)

// GetEmailSettings implements GET /api/admin/v1/establishments/{establishmentId}/email-settings.
func (h *Handlers) GetEmailSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateEmailSettings implements PUT /api/admin/v1/establishments/{establishmentId}/email-settings.
func (h *Handlers) UpdateEmailSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdateEmailSettingsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// SendTestEmail implements POST /api/admin/v1/establishments/{establishmentId}/email-settings/test.
func (h *Handlers) SendTestEmail(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.SendTestEmailParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListMessages implements GET /api/admin/v1/establishments/{establishmentId}/messages.
func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListMessagesParams) {
	problem.Write(w, r, problem.NotImplemented)
}
