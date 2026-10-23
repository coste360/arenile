package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// API keys managed from the panel (M5A, spec 16, 17). Owner: integrations
// backend builder. Responses that carry the plaintext key set
// Cache-Control: no-store; the key is never logged.
//
//	ListAPIKeys  -> APIKeys.List   -> api.ApiKeyList
//	CreateAPIKey -> APIKeys.Create -> 201 api.ApiKeyCreated
//	GetAPIKey    -> APIKeys.Get    -> api.ApiKey
//	UpdateAPIKey -> APIKeys.Update -> api.ApiKey
//	RevokeAPIKey -> APIKeys.Revoke -> 204

// ListAPIKeys implements GET /api/admin/v1/establishments/{establishmentId}/api-keys.
func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListAPIKeysParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateAPIKey implements POST /api/admin/v1/establishments/{establishmentId}/api-keys.
func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreateAPIKeyParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetAPIKey implements GET /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) GetAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdateAPIKey implements PATCH /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID, params api.UpdateAPIKeyParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RevokeAPIKey implements DELETE /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) RevokeAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID, params api.RevokeAPIKeyParams) {
	problem.Write(w, r, problem.NotImplemented)
}
