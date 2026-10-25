package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Push notification settings of an establishment (ADR 0027). Permissions
// live in push.Service.

// GetPushSettings implements GET /push-settings.
func (h *Handlers) GetPushSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.Push == nil {
		notImplemented(w, r)
		return
	}
	s, err := h.Push.GetSettings(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, PushSettingsToAPI(s))
}

// UpdatePushSettings implements PUT /push-settings (If-Match mandatory).
func (h *Handlers) UpdatePushSettings(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdatePushSettingsParams) {
	if h.Push == nil {
		notImplemented(w, r)
		return
	}
	im, err := RequireIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var expected *int64
	if !im.Any {
		expected = &im.Version
	}
	var req api.PushSettingsInput
	if !decode(w, r, &req) {
		return
	}
	s, err := h.Push.UpdateSettings(r.Context(), principalOf(r), establishmentID, expected, push.SettingsInput{
		Enabled: req.Enabled,
		Events: push.Events{
			NewOnlineBooking: req.Events.BookingNewOnline, ApprovalRequested: req.Events.BookingApprovalRequested,
			PaymentFailed: req.Events.PaymentFailed,
		},
	}, customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, s.Version, PushSettingsToAPI(s))
}

// PushSettingsToAPI maps push.Settings to api.PushSettings.
func PushSettingsToAPI(s *push.Settings) api.PushSettings {
	return api.PushSettings{
		Enabled: s.Enabled, RelayConfigured: s.RelayConfigured, Version: s.Version,
		Events: api.PushEvents{
			BookingNewOnline: s.Events.NewOnlineBooking, BookingApprovalRequested: s.Events.ApprovalRequested,
			PaymentFailed: s.Events.PaymentFailed,
		},
	}
}
