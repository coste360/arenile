package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// M5B season rollover (spec 8.7, 10.4, 28): owned by the builder of
// internal/domain/rollover (Handlers.Rollover). 501 until implemented.

// RolloverSeason implements POST /seasons/{seasonId}/rollover.
func (h *Handlers) RolloverSeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonID, _ api.RolloverSeasonParams) {
	notImplemented(w, r)
}

// ListSeasonRollovers implements GET /season-rollovers.
func (h *Handlers) ListSeasonRollovers(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListSeasonRolloversParams) {
	notImplemented(w, r)
}
