package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the setup builder (internal/domain/setup).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ApplySetupTemplate is not implemented yet.
func (h *Handlers) ApplySetupTemplate(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ApplySetupTemplateParams) {
	notImplemented(w, r)
}

// CreateQuickPricelist is not implemented yet.
func (h *Handlers) CreateQuickPricelist(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateQuickPricelistParams) {
	notImplemented(w, r)
}

// GetSetupStatus is not implemented yet.
func (h *Handlers) GetSetupStatus(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	notImplemented(w, r)
}

// ListSetupTemplates is not implemented yet.
func (h *Handlers) ListSetupTemplates(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	notImplemented(w, r)
}
