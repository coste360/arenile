package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of the inventory domain (internal/domain/inventory).
//
// M3 scaffolding: every operation answers 501 not_implemented until the
// owner of the domain package implements it (decode, call the service,
// encode; see maps.go and catalog.go for the patterns).

// CreateHold implements POST /api/admin/v1/establishments/{establishmentId}/holds.
func (h *Handlers) CreateHold(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateHoldParams) {
	notImplemented(w, r)
}

// GetHold implements GET /api/admin/v1/establishments/{establishmentId}/holds/{holdId}.
func (h *Handlers) GetHold(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.HoldID) {
	notImplemented(w, r)
}

// ReleaseHold implements DELETE /api/admin/v1/establishments/{establishmentId}/holds/{holdId}.
func (h *Handlers) ReleaseHold(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.HoldID, _ api.ReleaseHoldParams) {
	notImplemented(w, r)
}

// GetAvailability implements GET /api/admin/v1/establishments/{establishmentId}/availability.
func (h *Handlers) GetAvailability(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetAvailabilityParams) {
	notImplemented(w, r)
}

// GetOperationsDay implements GET /api/admin/v1/establishments/{establishmentId}/operations/day.
func (h *Handlers) GetOperationsDay(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetOperationsDayParams) {
	notImplemented(w, r)
}

// GetPlanning implements GET /api/admin/v1/establishments/{establishmentId}/planning.
func (h *Handlers) GetPlanning(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.GetPlanningParams) {
	notImplemented(w, r)
}

// ListSpotBlocks implements GET /api/admin/v1/establishments/{establishmentId}/spot-blocks.
func (h *Handlers) ListSpotBlocks(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListSpotBlocksParams) {
	notImplemented(w, r)
}

// CreateSpotBlock implements POST /api/admin/v1/establishments/{establishmentId}/spot-blocks.
func (h *Handlers) CreateSpotBlock(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateSpotBlockParams) {
	notImplemented(w, r)
}

// DeleteSpotBlock implements DELETE /api/admin/v1/establishments/{establishmentId}/spot-blocks/{blockId}.
func (h *Handlers) DeleteSpotBlock(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SpotBlockID, _ api.DeleteSpotBlockParams) {
	notImplemented(w, r)
}
