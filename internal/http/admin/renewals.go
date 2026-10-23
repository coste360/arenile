package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
)

// M5B renewal campaigns and proposals (spec 10.4): owned by the builder of
// internal/domain/renewal (Handlers.Renewals). 501 until implemented.

// ListRenewalCampaigns implements GET /renewal-campaigns.
func (h *Handlers) ListRenewalCampaigns(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListRenewalCampaignsParams) {
	notImplemented(w, r)
}

// CreateRenewalCampaign implements POST /renewal-campaigns.
func (h *Handlers) CreateRenewalCampaign(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateRenewalCampaignParams) {
	notImplemented(w, r)
}

// GetRenewalCampaign implements GET /renewal-campaigns/{campaignId}.
func (h *Handlers) GetRenewalCampaign(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalCampaignID) {
	notImplemented(w, r)
}

// UpdateRenewalCampaign implements PATCH /renewal-campaigns/{campaignId}.
func (h *Handlers) UpdateRenewalCampaign(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalCampaignID, _ api.UpdateRenewalCampaignParams) {
	notImplemented(w, r)
}

// ListRenewalProposals implements GET /renewal-campaigns/{campaignId}/proposals.
func (h *Handlers) ListRenewalProposals(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalCampaignID, _ api.ListRenewalProposalsParams) {
	notImplemented(w, r)
}

// BulkRenewalProposals implements POST /renewal-campaigns/{campaignId}/proposals/bulk.
func (h *Handlers) BulkRenewalProposals(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalCampaignID, _ api.BulkRenewalProposalsParams) {
	notImplemented(w, r)
}

// GetRenewalProposal implements GET /renewal-proposals/{proposalId}.
func (h *Handlers) GetRenewalProposal(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalProposalID) {
	notImplemented(w, r)
}

// AcceptRenewalProposal implements POST /renewal-proposals/{proposalId}/accept.
func (h *Handlers) AcceptRenewalProposal(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalProposalID, _ api.AcceptRenewalProposalParams) {
	notImplemented(w, r)
}

// DeclineRenewalProposal implements POST /renewal-proposals/{proposalId}/decline.
func (h *Handlers) DeclineRenewalProposal(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalProposalID, _ api.DeclineRenewalProposalParams) {
	notImplemented(w, r)
}

// SendRenewalProposal implements POST /renewal-proposals/{proposalId}/send.
func (h *Handlers) SendRenewalProposal(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.RenewalProposalID, _ api.SendRenewalProposalParams) {
	notImplemented(w, r)
}
