package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the team builder (internal/domain/team).
// They are 501 stubs generated with the M1/M2 contract: replace each body with
// decode -> service call -> encode (see config_helpers.go for ETag, If-Match
// and pagination helpers). Keep the method set: api.ServerInterface needs it.

// ListTeamMembers is not implemented yet.
func (h *Handlers) ListTeamMembers(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListTeamMembersParams) {
	notImplemented(w, r)
}

// CreateTeamMember is not implemented yet.
func (h *Handlers) CreateTeamMember(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateTeamMemberParams) {
	notImplemented(w, r)
}

// RemoveTeamMember is not implemented yet.
func (h *Handlers) RemoveTeamMember(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TeamUserID, _ api.RemoveTeamMemberParams) {
	notImplemented(w, r)
}

// UpdateTeamMember is not implemented yet.
func (h *Handlers) UpdateTeamMember(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.TeamUserID, _ api.UpdateTeamMemberParams) {
	notImplemented(w, r)
}
