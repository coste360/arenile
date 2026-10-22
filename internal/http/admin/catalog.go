package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the catalog builder (internal/domain/catalog).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListComponents is not implemented yet.
func (h *Handlers) ListComponents(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListComponentsParams) {
	notImplemented(w, r)
}

// CreateComponent is not implemented yet.
func (h *Handlers) CreateComponent(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateComponentParams) {
	notImplemented(w, r)
}

// DeleteComponent is not implemented yet.
func (h *Handlers) DeleteComponent(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ComponentID, _ api.DeleteComponentParams) {
	notImplemented(w, r)
}

// GetComponent is not implemented yet.
func (h *Handlers) GetComponent(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ComponentID) {
	notImplemented(w, r)
}

// UpdateComponent is not implemented yet.
func (h *Handlers) UpdateComponent(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ComponentID, _ api.UpdateComponentParams) {
	notImplemented(w, r)
}

// ListExtras is not implemented yet.
func (h *Handlers) ListExtras(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListExtrasParams) {
	notImplemented(w, r)
}

// CreateExtra is not implemented yet.
func (h *Handlers) CreateExtra(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateExtraParams) {
	notImplemented(w, r)
}

// DeleteExtra is not implemented yet.
func (h *Handlers) DeleteExtra(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ExtraID, _ api.DeleteExtraParams) {
	notImplemented(w, r)
}

// GetExtra is not implemented yet.
func (h *Handlers) GetExtra(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ExtraID) {
	notImplemented(w, r)
}

// UpdateExtra is not implemented yet.
func (h *Handlers) UpdateExtra(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ExtraID, _ api.UpdateExtraParams) {
	notImplemented(w, r)
}

// ListPacks is not implemented yet.
func (h *Handlers) ListPacks(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPacksParams) {
	notImplemented(w, r)
}

// CreatePack is not implemented yet.
func (h *Handlers) CreatePack(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePackParams) {
	notImplemented(w, r)
}

// DeletePack is not implemented yet.
func (h *Handlers) DeletePack(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PackID, _ api.DeletePackParams) {
	notImplemented(w, r)
}

// GetPack is not implemented yet.
func (h *Handlers) GetPack(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PackID) {
	notImplemented(w, r)
}

// UpdatePack is not implemented yet.
func (h *Handlers) UpdatePack(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PackID, _ api.UpdatePackParams) {
	notImplemented(w, r)
}
