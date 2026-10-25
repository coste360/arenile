package device

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Push registration of the paired device (owner: push, ADR 0027). The
// native token is accepted here and never written back, logged or audited.

// GetDevicePushRegistration implements GET /api/device/v1/push.
func (h *Handlers) GetDevicePushRegistration(w http.ResponseWriter, r *http.Request) {
	if h.Push == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	st, err := h.Push.Status(r.Context(), p)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDeviceJSON(w, http.StatusOK, pushStatusToAPI(st))
}

// RegisterDevicePush implements PUT /api/device/v1/push.
func (h *Handlers) RegisterDevicePush(w http.ResponseWriter, r *http.Request) {
	if h.Push == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.PushRegistrationInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
			return
		}
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return
	}
	in := push.RegisterInput{Token: req.Token, Locale: string(req.Locale)}
	if req.Environment != nil {
		in.Environment = string(*req.Environment)
	}
	for _, t := range req.Topics {
		in.Topics = append(in.Topics, string(t))
	}
	p, _ := auth.PrincipalFrom(r.Context())
	st, err := h.Push.Register(r.Context(), p, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDeviceJSON(w, http.StatusOK, pushStatusToAPI(st))
}

// UnregisterDevicePush implements DELETE /api/device/v1/push.
func (h *Handlers) UnregisterDevicePush(w http.ResponseWriter, r *http.Request) {
	if h.Push == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.Push.Unregister(r.Context(), p); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pushStatusToAPI(st *push.Status) api.PushRegistrationStatus {
	out := api.PushRegistrationStatus{
		Available: st.Available, RelayConfigured: st.RelayConfigured, EstablishmentEnabled: st.EstablishmentEnabled,
		Registered: st.Registered, UpdatedAt: st.UpdatedAt,
		Topics: make([]api.PushTopic, 0, len(st.Topics)), AllowedTopics: make([]api.PushTopic, 0, len(st.AllowedTopics)),
	}
	if st.Environment != "" {
		e := api.PushEnvironment(st.Environment)
		out.Environment = &e
	}
	if st.Locale != "" {
		l := api.PushLocale(st.Locale)
		out.Locale = &l
	}
	for _, t := range st.Topics {
		out.Topics = append(out.Topics, api.PushTopic(t))
	}
	for _, t := range st.AllowedTopics {
		out.AllowedTopics = append(out.AllowedTopics, api.PushTopic(t))
	}
	return out
}
