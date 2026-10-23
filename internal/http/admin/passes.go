package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the pass domain (internal/domain/pass).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// ListSeasonPasses implements GET /api/admin/v1/establishments/{establishmentId}/season-passes.
func (h *Handlers) ListSeasonPasses(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListSeasonPassesParams) {
	notImplemented(w, r)
}

// CreateSeasonPass implements POST /api/admin/v1/establishments/{establishmentId}/season-passes.
func (h *Handlers) CreateSeasonPass(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateSeasonPassParams) {
	notImplemented(w, r)
}

// GetSeasonPass implements GET /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}.
func (h *Handlers) GetSeasonPass(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID) {
	notImplemented(w, r)
}

// UpdateSeasonPass implements PATCH /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}.
func (h *Handlers) UpdateSeasonPass(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID, _ api.UpdateSeasonPassParams) {
	notImplemented(w, r)
}

// ListSeasonPassReleases implements GET /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases.
func (h *Handlers) ListSeasonPassReleases(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID) {
	notImplemented(w, r)
}

// CreateSeasonPassReleases implements POST /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases.
func (h *Handlers) CreateSeasonPassReleases(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID, _ api.CreateSeasonPassReleasesParams) {
	notImplemented(w, r)
}

// RevokeSeasonPassRelease implements DELETE /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases/{releaseId}.
func (h *Handlers) RevokeSeasonPassRelease(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID, _ api.ReleaseID, _ api.RevokeSeasonPassReleaseParams) {
	notImplemented(w, r)
}
