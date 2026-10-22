package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the policies builder (internal/domain/pricingconfig (PolicyService)).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListPolicies is not implemented yet.
func (h *Handlers) ListPolicies(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPoliciesParams) {
	notImplemented(w, r)
}

// CreatePolicy is not implemented yet.
func (h *Handlers) CreatePolicy(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePolicyParams) {
	notImplemented(w, r)
}

// DeletePolicy is not implemented yet.
func (h *Handlers) DeletePolicy(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PolicyID, _ api.DeletePolicyParams) {
	notImplemented(w, r)
}

// GetPolicy is not implemented yet.
func (h *Handlers) GetPolicy(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PolicyID) {
	notImplemented(w, r)
}

// UpdatePolicy is not implemented yet.
func (h *Handlers) UpdatePolicy(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PolicyID, _ api.UpdatePolicyParams) {
	notImplemented(w, r)
}
