package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the mapping builder (internal/domain/mapping).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListMaps is not implemented yet.
func (h *Handlers) ListMaps(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListMapsParams) {
	notImplemented(w, r)
}

// CreateMap is not implemented yet.
func (h *Handlers) CreateMap(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateMapParams) {
	notImplemented(w, r)
}

// DeleteMap is not implemented yet.
func (h *Handlers) DeleteMap(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MapID, _ api.DeleteMapParams) {
	notImplemented(w, r)
}

// GetMap is not implemented yet.
func (h *Handlers) GetMap(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MapID) {
	notImplemented(w, r)
}

// SaveMap is not implemented yet.
func (h *Handlers) SaveMap(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MapID, _ api.SaveMapParams) {
	notImplemented(w, r)
}

// GenerateMapRows is not implemented yet.
func (h *Handlers) GenerateMapRows(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.MapID, _ api.GenerateMapRowsParams) {
	notImplemented(w, r)
}

// ListPools is not implemented yet.
func (h *Handlers) ListPools(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPoolsParams) {
	notImplemented(w, r)
}

// CreatePool is not implemented yet.
func (h *Handlers) CreatePool(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePoolParams) {
	notImplemented(w, r)
}

// DeletePool is not implemented yet.
func (h *Handlers) DeletePool(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PoolID, _ api.DeletePoolParams) {
	notImplemented(w, r)
}

// GetPool is not implemented yet.
func (h *Handlers) GetPool(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PoolID) {
	notImplemented(w, r)
}

// UpdatePool is not implemented yet.
func (h *Handlers) UpdatePool(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PoolID, _ api.UpdatePoolParams) {
	notImplemented(w, r)
}

// ListZones is not implemented yet.
func (h *Handlers) ListZones(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListZonesParams) {
	notImplemented(w, r)
}

// CreateZone is not implemented yet.
func (h *Handlers) CreateZone(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateZoneParams) {
	notImplemented(w, r)
}

// DeleteZone is not implemented yet.
func (h *Handlers) DeleteZone(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ZoneID, _ api.DeleteZoneParams) {
	notImplemented(w, r)
}

// GetZone is not implemented yet.
func (h *Handlers) GetZone(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ZoneID) {
	notImplemented(w, r)
}

// UpdateZone is not implemented yet.
func (h *Handlers) UpdateZone(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ZoneID, _ api.UpdateZoneParams) {
	notImplemented(w, r)
}
