package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/apikey"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
)

// API keys managed from the panel (M5A, spec 16, 17). Owner only; a request
// authenticated with an API key always gets 403. Responses that carry the
// plaintext key set Cache-Control: no-store; the key is never logged.
//
//	ListAPIKeys  -> APIKeys.List   -> api.ApiKeyList
//	CreateAPIKey -> APIKeys.Create -> 201 api.ApiKeyCreated
//	GetAPIKey    -> APIKeys.Get    -> api.ApiKey
//	UpdateAPIKey -> APIKeys.Update -> api.ApiKey
//	RevokeAPIKey -> APIKeys.Revoke -> 204

// ListAPIKeys implements GET /api/admin/v1/establishments/{establishmentId}/api-keys.
func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListAPIKeysParams) {
	if !h.apiKeysReady(w, r) {
		return
	}
	keys, err := h.APIKeys.List(r.Context(), principalOf(r), establishmentID, params.IncludeRevoked != nil && *params.IncludeRevoked)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ApiKeyList{Items: make([]api.ApiKey, len(keys))}
	for i := range keys {
		out.Items[i] = APIKeyToAPI(&keys[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateAPIKey implements POST /api/admin/v1/establishments/{establishmentId}/api-keys.
func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateAPIKeyParams) {
	if !h.apiKeysReady(w, r) {
		return
	}
	var req api.ApiKeyCreate
	if !decode(w, r, &req) {
		return
	}
	k, secret, err := h.APIKeys.Create(r.Context(), principalOf(r), establishmentID, apikey.CreateInput{
		Name: req.Name, Scopes: permissionsFromAPI(req.Scopes), ExpiresAt: req.ExpiresAt, IP: estClientIP(r),
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	// The plaintext key is a credential shown once: never cache it.
	w.Header().Set("Cache-Control", "no-store")
	middleware.NoReplay(w) // a secret shown once: never stored for Idempotency-Key replay
	writeJSON(w, http.StatusCreated, api.ApiKeyCreated{ApiKey: APIKeyToAPI(k), Secret: secret})
}

// GetAPIKey implements GET /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) GetAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID) {
	if !h.apiKeysReady(w, r) {
		return
	}
	k, err := h.APIKeys.Get(r.Context(), principalOf(r), establishmentID, apiKeyID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, APIKeyToAPI(k))
}

// UpdateAPIKey implements PATCH /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) UpdateAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID, _ api.UpdateAPIKeyParams) {
	if !h.apiKeysReady(w, r) {
		return
	}
	var req api.ApiKeyUpdate
	if !decode(w, r, &req) {
		return
	}
	in := apikey.UpdateInput{Name: req.Name, ExpiresAt: req.ExpiresAt, IP: estClientIP(r)}
	if req.Scopes != nil {
		in.Scopes = permissionsFromAPI(*req.Scopes)
		if in.Scopes == nil {
			in.Scopes = []auth.Permission{} // explicit empty list: rejected by validation
		}
	}
	if req.ClearExpiry != nil {
		in.ClearExpiry = *req.ClearExpiry
	}
	k, err := h.APIKeys.Update(r.Context(), principalOf(r), establishmentID, apiKeyID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, APIKeyToAPI(k))
}

// RevokeAPIKey implements DELETE /api/admin/v1/establishments/{establishmentId}/api-keys/{apiKeyId}.
func (h *Handlers) RevokeAPIKey(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, apiKeyID api.ApiKeyID, _ api.RevokeAPIKeyParams) {
	if !h.apiKeysReady(w, r) {
		return
	}
	if err := h.APIKeys.Revoke(r.Context(), principalOf(r), establishmentID, apiKeyID, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// APIKeyToAPI maps an API key (never the secret or its hash).
func APIKeyToAPI(k *apikey.Key) api.ApiKey {
	out := api.ApiKey{
		Id: k.ID, EstablishmentId: k.EstablishmentID, Name: k.Name, Prefix: k.DisplayPrefix,
		Scopes: make([]api.Permission, len(k.Scopes)), Status: api.ApiKeyStatus(k.Status),
		CreatedAt: k.CreatedAt.UTC(), LastUsedAt: utcPtr(k.LastUsedAt), ExpiresAt: utcPtr(k.ExpiresAt), RevokedAt: utcPtr(k.RevokedAt),
	}
	for i, s := range k.Scopes {
		out.Scopes[i] = api.Permission(s)
	}
	if k.CreatedByUserID != "" {
		id, name := k.CreatedByUserID, k.CreatedByName
		out.CreatedByUserId, out.CreatedByName = &id, &name
	}
	return out
}

func permissionsFromAPI(in []api.Permission) []auth.Permission {
	if len(in) == 0 {
		return nil
	}
	out := make([]auth.Permission, len(in))
	for i, p := range in {
		out[i] = auth.Permission(p)
	}
	return out
}

// apiKeysReady answers 501 when the API key service is not wired.
func (h *Handlers) apiKeysReady(w http.ResponseWriter, r *http.Request) bool {
	if h.APIKeys == nil {
		notImplemented(w, r)
		return false
	}
	return true
}
