package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Operational map additions (spec 30.4, 30.5, 30.10, ADR 0028). Owner: the
// operational map builder.
//
//	GetSpotCalendar  -> Inventory.SpotCalendar  -> api.SpotCalendar
//	GetLifeguardList -> Inventory.LifeguardList -> api.LifeguardList (json | html)

// GetSpotCalendar implements GET /spots/{spotId}/calendar.
func (h *Handlers) GetSpotCalendar(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SpotID, _ api.GetSpotCalendarParams) {
	notImplemented(w, r)
}

// GetLifeguardList implements GET /operations/lifeguard-list.
func (h *Handlers) GetLifeguardList(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetLifeguardListParams) {
	notImplemented(w, r)
}
