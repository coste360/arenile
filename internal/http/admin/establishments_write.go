package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the establishment builder (internal/domain/establishment (Service.Create, Service.Update)).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// CreateEstablishment is not implemented yet.
func (h *Handlers) CreateEstablishment(w http.ResponseWriter, r *http.Request, _ api.CreateEstablishmentParams) {
	notImplemented(w, r)
}

// UpdateEstablishment is not implemented yet.
func (h *Handlers) UpdateEstablishment(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.UpdateEstablishmentParams) {
	notImplemented(w, r)
}
