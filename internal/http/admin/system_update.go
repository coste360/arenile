package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/update"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Update notice (ADR 0025). Owner: internal/domain/update.
//
//	GetSystemUpdate   -> Updates.Status   -> api.SystemUpdate
//	CheckSystemUpdate -> Updates.CheckNow -> api.SystemUpdate

// GetSystemUpdate implements GET /api/admin/v1/system/update.
func (h *Handlers) GetSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if h.Updates == nil {
		notImplemented(w, r)
		return
	}
	st, err := h.Updates.Status(r.Context(), principalOf(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, SystemUpdateToAPI(st))
}

// CheckSystemUpdate implements POST /api/admin/v1/system/update/check
// ("Verifica aggiornamenti"): the service debounces the network check.
func (h *Handlers) CheckSystemUpdate(w http.ResponseWriter, r *http.Request, _ api.CheckSystemUpdateParams) {
	if h.Updates == nil {
		notImplemented(w, r)
		return
	}
	st, err := h.Updates.CheckNow(r.Context(), principalOf(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, SystemUpdateToAPI(st))
}

// SystemUpdateToAPI maps the domain status to the API representation.
func SystemUpdateToAPI(st update.Status) api.SystemUpdate {
	out := api.SystemUpdate{
		Current: st.Current, Status: api.SystemUpdateStatus(st.State), UpdateAvailable: st.UpdateAvailable, CheckEnabled: st.CheckEnabled,
		HowToUpdate: api.SystemUpdateHowToUpdate(st.HowTo), Pending: st.Pending,
	}
	if !st.LastAttemptAt.IsZero() {
		t := st.LastAttemptAt.UTC()
		out.LastAttemptAt = &t
	}
	if st.GOOS != "" {
		out.Os = &st.GOOS
	}
	if st.GOARCH != "" {
		out.Arch = &st.GOARCH
	}
	if st.Latest != "" {
		out.Latest = &st.Latest
	}
	if !st.PublishedAt.IsZero() {
		t := st.PublishedAt.UTC()
		out.PublishedAt = &t
	}
	if !st.CheckedAt.IsZero() {
		t := st.CheckedAt.UTC()
		out.CheckedAt = &t
	}
	if st.ReleaseNotesURL != "" {
		out.ReleaseNotesUrl = &st.ReleaseNotesURL
	}
	if st.DownloadURL != "" {
		out.DownloadUrl = &st.DownloadURL
	}
	return out
}
