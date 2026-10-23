package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/team"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
)

// Handlers of the team (spec 16): users with access to an establishment and
// their roles. Listing needs the view permission, every change is owner
// only. Owned by the team builder (internal/domain/team).

// ListTeamMembers implements GET /establishments/{establishmentId}/team.
func (h *Handlers) ListTeamMembers(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListTeamMembersParams) {
	if !h.teamReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Team.List(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.TeamMemberList{Items: make([]api.TeamMember, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, TeamMemberToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateTeamMember implements POST /establishments/{establishmentId}/team.
func (h *Handlers) CreateTeamMember(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateTeamMemberParams) {
	if !h.teamReady(w, r) {
		return
	}
	var req api.TeamMemberCreate
	if !decode(w, r, &req) {
		return
	}
	in := team.CreateInput{
		Email: string(req.Email), Name: req.Name, Locale: estDeref(req.Locale), Password: estDeref(req.Password),
		Role: auth.Role(req.Role), IP: estClientIP(r),
	}
	if req.Scope != nil {
		in.Scope = team.Scope(*req.Scope)
	}
	res, err := h.Team.Create(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	// The generated password is a credential shown once: never cache it.
	w.Header().Set("Cache-Control", "no-store")
	middleware.NoReplay(w) // a secret shown once: never stored for Idempotency-Key replay
	out := api.TeamMemberCreated{Member: TeamMemberToAPI(&res.Member), UserCreated: res.UserCreated}
	if res.InitialPassword != "" {
		pw := res.InitialPassword
		out.InitialPassword = &pw
	}
	writeJSON(w, http.StatusCreated, out)
}

// UpdateTeamMember implements PATCH /establishments/{establishmentId}/team/{userId}.
func (h *Handlers) UpdateTeamMember(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, userID api.TeamUserID, _ api.UpdateTeamMemberParams) {
	if !h.teamReady(w, r) {
		return
	}
	var req api.TeamMemberUpdate
	if !decode(w, r, &req) {
		return
	}
	in := team.UpdateInput{Active: req.Active, IP: estClientIP(r)}
	if req.Role != nil {
		role := auth.Role(*req.Role)
		in.Role = &role
	}
	if req.Scope != nil {
		sc := team.Scope(*req.Scope)
		in.Scope = &sc
	}
	m, err := h.Team.Update(r.Context(), principalOf(r), establishmentID, userID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, TeamMemberToAPI(m))
}

// RemoveTeamMember implements DELETE /establishments/{establishmentId}/team/{userId}.
func (h *Handlers) RemoveTeamMember(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, userID api.TeamUserID, _ api.RemoveTeamMemberParams) {
	if !h.teamReady(w, r) {
		return
	}
	if err := h.Team.Remove(r.Context(), principalOf(r), establishmentID, userID, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TeamMemberToAPI maps a team member.
func TeamMemberToAPI(m *team.Member) api.TeamMember {
	out := api.TeamMember{
		UserId: m.User.ID, Email: m.User.Email, Name: m.User.Name, Locale: m.User.Locale, Active: m.User.Active,
		TotpEnabled: m.User.TOTPEnabled(), LastLoginAt: utcPtr(m.User.LastLoginAt),
		Role: api.Role(m.Role), Scope: api.TeamScope(m.Scope), Memberships: make([]api.TeamMembership, len(m.Memberships)),
	}
	for i, ms := range m.Memberships {
		out.Memberships[i] = api.TeamMembership{Id: ms.ID, OrganizationId: ms.OrganizationID, EstablishmentId: ms.EstablishmentID, Role: api.Role(ms.Role)}
	}
	return out
}

// teamReady answers 501 when the team service is not wired.
func (h *Handlers) teamReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Team == nil {
		notImplemented(w, r)
		return false
	}
	return true
}
