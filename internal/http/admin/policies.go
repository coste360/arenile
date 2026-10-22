package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricingconfig"
	"github.com/coste360/arenile/internal/http/api"
)

// Handlers of this file are owned by the policies builder
// (internal/domain/pricingconfig PolicyService): booking policies of a
// season. The establishment default policy is part of the establishment
// settings (spec 9).

func pcPolicyInputFromAPI(in api.BookingPolicyInput) (pricingconfig.PolicyInput, error) {
	out := pricingconfig.PolicyInput{
		SeasonID: in.SeasonId, Name: in.Name, Priority: pcIntOr(in.Priority, 0), Active: pcBoolOr(in.Active, true),
	}
	if err := ConvertJSON(in.Scope, &out.Scope); err != nil {
		return out, configerr.Invalid("scope", "Ambito non valido.")
	}
	if err := ConvertJSON(in.Rules, &out.Rules); err != nil {
		return out, configerr.Invalid("rules", "Regole non valide.")
	}
	return out, nil
}

func pcPolicyScopeToAPI(s policy.Scope) api.PolicyScope {
	var out api.PolicyScope
	_ = ConvertJSON(s, &out)
	return out
}

// PolicyToAPI maps a booking policy.
func PolicyToAPI(p pricingconfig.Policy) api.BookingPolicy {
	return api.BookingPolicy{
		Id: p.ID, SeasonId: p.SeasonID, Name: p.Name, Scope: pcPolicyScopeToAPI(p.Scope), Rules: PolicyRulesToAPI(p.Rules),
		Priority: p.Priority, Active: p.Active, Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// ListPolicies implements GET /establishments/{id}/policies.
func (h *Handlers) ListPolicies(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPoliciesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	items, next, err := h.Policies.List(r.Context(), principalOf(r), establishmentID, pcStrOr(params.SeasonId), cursor, limit)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	out := api.BookingPolicyList{Items: make([]api.BookingPolicy, 0, len(items)), NextCursor: nextCursor(next)}
	for _, it := range items {
		out.Items = append(out.Items, PolicyToAPI(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePolicy implements POST /establishments/{id}/policies.
func (h *Handlers) CreatePolicy(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePolicyParams) {
	var req api.BookingPolicyInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcPolicyInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	p, err := h.Policies.Create(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, p.Version, PolicyToAPI(*p))
}

// GetPolicy implements GET /establishments/{id}/policies/{policyId}.
func (h *Handlers) GetPolicy(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PolicyID) {
	p, err := h.Policies.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, PolicyToAPI(*p))
}

// UpdatePolicy implements PUT /establishments/{id}/policies/{policyId}.
func (h *Handlers) UpdatePolicy(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PolicyID, params api.UpdatePolicyParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	var req api.BookingPolicyInput
	if !decode(w, r, &req) {
		return
	}
	in, err := pcPolicyInputFromAPI(req)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	p, err := h.Policies.Update(r.Context(), principalOf(r), establishmentID, id, expected, in)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, PolicyToAPI(*p))
}

// DeletePolicy implements DELETE /establishments/{id}/policies/{policyId}.
func (h *Handlers) DeletePolicy(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.PolicyID, params api.DeletePolicyParams) {
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.pricingFail(w, r, err)
		return
	}
	if err := h.Policies.Delete(r.Context(), principalOf(r), establishmentID, id, expected); err != nil {
		h.pricingFail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
