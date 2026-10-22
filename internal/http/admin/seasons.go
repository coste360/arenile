package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the season builder (internal/domain/season).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListClosureDays is not implemented yet.
func (h *Handlers) ListClosureDays(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListClosureDaysParams) {
	notImplemented(w, r)
}

// CreateClosureDay is not implemented yet.
func (h *Handlers) CreateClosureDay(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateClosureDayParams) {
	notImplemented(w, r)
}

// DeleteClosureDay is not implemented yet.
func (h *Handlers) DeleteClosureDay(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ClosureDayID, _ api.DeleteClosureDayParams) {
	notImplemented(w, r)
}

// GetClosureDay is not implemented yet.
func (h *Handlers) GetClosureDay(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ClosureDayID) {
	notImplemented(w, r)
}

// UpdateClosureDay is not implemented yet.
func (h *Handlers) UpdateClosureDay(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ClosureDayID, _ api.UpdateClosureDayParams) {
	notImplemented(w, r)
}

// ListRatePeriods is not implemented yet.
func (h *Handlers) ListRatePeriods(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListRatePeriodsParams) {
	notImplemented(w, r)
}

// CreateRatePeriod is not implemented yet.
func (h *Handlers) CreateRatePeriod(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateRatePeriodParams) {
	notImplemented(w, r)
}

// DeleteRatePeriod is not implemented yet.
func (h *Handlers) DeleteRatePeriod(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RatePeriodID, _ api.DeleteRatePeriodParams) {
	notImplemented(w, r)
}

// GetRatePeriod is not implemented yet.
func (h *Handlers) GetRatePeriod(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RatePeriodID) {
	notImplemented(w, r)
}

// UpdateRatePeriod is not implemented yet.
func (h *Handlers) UpdateRatePeriod(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RatePeriodID, _ api.UpdateRatePeriodParams) {
	notImplemented(w, r)
}

// ListSeasons is not implemented yet.
func (h *Handlers) ListSeasons(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListSeasonsParams) {
	notImplemented(w, r)
}

// CreateSeason is not implemented yet.
func (h *Handlers) CreateSeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateSeasonParams) {
	notImplemented(w, r)
}

// DeleteSeason is not implemented yet.
func (h *Handlers) DeleteSeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonID, _ api.DeleteSeasonParams) {
	notImplemented(w, r)
}

// GetSeason is not implemented yet.
func (h *Handlers) GetSeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonID) {
	notImplemented(w, r)
}

// UpdateSeason is not implemented yet.
func (h *Handlers) UpdateSeason(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonID, _ api.UpdateSeasonParams) {
	notImplemented(w, r)
}
