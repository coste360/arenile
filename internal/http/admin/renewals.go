package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B renewal campaigns and proposals (spec 10.4): handlers of
// internal/domain/renewal (Handlers.Renewals). Decode, call the service
// (which checks permissions and owns the transactions), encode.

// ListRenewalCampaigns implements GET /renewal-campaigns.
func (h *Handlers) ListRenewalCampaigns(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListRenewalCampaignsParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	status := ""
	if params.Status != nil {
		status = string(*params.Status)
	}
	page, err := h.Renewals.ListCampaigns(r.Context(), principalOf(r), establishmentID, customerStr(params.TargetSeasonId), status, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RenewalCampaignList{Items: make([]api.RenewalCampaign, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, renewalCampaignToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateRenewalCampaign implements POST /renewal-campaigns.
func (h *Handlers) CreateRenewalCampaign(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateRenewalCampaignParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	var req api.RenewalCampaignCreate
	if !decode(w, r, &req) {
		return
	}
	in := renewal.CreateInput{
		Name: customerStr(req.Name), SourceSeasonID: req.SourceSeasonId, TargetSeasonID: req.TargetSeasonId,
		DeadlineAt: req.DeadlineAt, Notes: customerStr(req.Notes), DryRun: req.DryRun != nil && *req.DryRun, IP: customerClientIP(r),
	}
	if req.PassIds != nil {
		in.PassIDs = *req.PassIds
	}
	res, err := h.Renewals.CreateCampaign(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RenewalCampaignResult{
		DryRun: res.DryRun, Proposals: make([]api.RenewalProposalPreview, 0, len(res.Proposals)),
		Skipped: make([]api.RenewalSkipped, 0, len(res.Skipped)),
	}
	if res.Campaign != nil && !res.DryRun {
		c := renewalCampaignToAPI(res.Campaign)
		out.Campaign = &c
	}
	for i := range res.Proposals {
		p := &res.Proposals[i]
		out.Proposals = append(out.Proposals, api.RenewalProposalPreview{
			SourcePassId: p.SourceBookingID, CustomerId: p.CustomerID, CustomerDisplayName: p.CustomerDisplayName,
			SpotId: p.SpotID, SpotLabel: p.SpotLabel, PackId: p.PackID, StartsOn: renewalDate(p.StartsOn), EndsOn: renewalDate(p.EndsOn),
			Slots: api.Slots(p.Slots), PreviousAmount: int64(p.PreviousAmount), TotalAmount: int64(p.TotalAmount), DepositDue: int64(p.DepositDue),
		})
	}
	for _, sk := range res.Skipped {
		out.Skipped = append(out.Skipped, api.RenewalSkipped{
			SourcePassId: sk.SourcePassID, Reason: api.RenewalSkipReason(sk.Reason), CustomerDisplayName: strPtr(sk.CustomerDisplayName),
			SpotLabel: strPtr(sk.SpotLabel), Detail: strPtr(sk.Detail),
		})
	}
	status := http.StatusCreated
	if res.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, out)
}

// GetRenewalCampaign implements GET /renewal-campaigns/{campaignId}.
func (h *Handlers) GetRenewalCampaign(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalCampaignID) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	c, err := h.Renewals.GetCampaign(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, renewalCampaignToAPI(c))
}

// UpdateRenewalCampaign implements PATCH /renewal-campaigns/{campaignId}.
func (h *Handlers) UpdateRenewalCampaign(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalCampaignID, params api.UpdateRenewalCampaignParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.RenewalCampaignUpdate
	if !decode(w, r, &req) {
		return
	}
	in := renewal.UpdateInput{Name: req.Name, Notes: req.Notes, DeadlineAt: req.DeadlineAt, IP: customerClientIP(r)}
	if req.Status != nil {
		switch *req.Status {
		case api.RenewalCampaignStatusClosed:
			in.Close = true
		case api.RenewalCampaignStatusOpen:
		default:
			problem.Write(w, r, problem.ValidationFailed, problem.Fields(api.FieldError{Field: "status", Message: "Usa open o closed."}))
			return
		}
	}
	c, err := h.Renewals.UpdateCampaign(r.Context(), principalOf(r), establishmentID, id, ifMatch, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, renewalCampaignToAPI(c))
}

// ListRenewalProposals implements GET /renewal-campaigns/{campaignId}/proposals.
func (h *Handlers) ListRenewalProposals(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalCampaignID, params api.ListRenewalProposalsParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := renewal.ProposalFilter{Sent: params.Sent, ZoneID: customerStr(params.ZoneId), Query: customerStr(params.Q)}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Renewals.ListProposals(r.Context(), principalOf(r), establishmentID, id, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RenewalProposalList{Items: make([]api.RenewalProposal, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, renewalProposalToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// BulkRenewalProposals implements POST /renewal-campaigns/{campaignId}/proposals/bulk.
func (h *Handlers) BulkRenewalProposals(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalCampaignID, _ api.BulkRenewalProposalsParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	var req api.RenewalBulkRequest
	if !decode(w, r, &req) {
		return
	}
	in := renewal.BulkInput{
		Action: string(req.Action), DeadlineAt: req.DeadlineAt, OnlyUnsent: req.OnlyUnsent == nil || *req.OnlyUnsent,
		Confirm: req.Confirm == nil || *req.Confirm, Force: req.Force != nil && *req.Force, IP: customerClientIP(r),
	}
	if req.ProposalIds != nil {
		in.ProposalIDs = *req.ProposalIds
	}
	if req.Status != nil {
		in.Status = string(*req.Status)
	}
	res, err := h.Renewals.Bulk(r.Context(), principalOf(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RenewalBulkResult{
		Action: req.Action, Matched: res.Matched, Succeeded: res.Succeeded, Skipped: res.Skipped,
		Failed: make([]api.RenewalBulkFailure, 0, len(res.Failed)),
	}
	for _, f := range res.Failed {
		p := problemOf(r, f.Err)
		out.Failed = append(out.Failed, api.RenewalBulkFailure{ProposalId: f.ProposalID, Type: p.Type, Detail: p.Detail})
	}
	writeJSON(w, http.StatusOK, out)
}

// problemOf maps a domain error to its problem (type and detail) exactly as
// problem.Error would answer it, for the per-item failures of a bulk action.
func problemOf(r *http.Request, err error) api.Problem {
	rec := httptest.NewRecorder()
	problem.Error(rec, r, nil, err)
	var p api.Problem
	if json.Unmarshal(rec.Body.Bytes(), &p) != nil || p.Type == "" {
		p.Type = problem.InternalError
	}
	if p.Type == problem.InternalError {
		p.Detail = nil // never leak internal errors
	}
	return p
}

// GetRenewalProposal implements GET /renewal-proposals/{proposalId}.
func (h *Handlers) GetRenewalProposal(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalProposalID) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	p, err := h.Renewals.GetProposal(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, renewalProposalToAPI(p))
}

// AcceptRenewalProposal implements POST /renewal-proposals/{proposalId}/accept.
func (h *Handlers) AcceptRenewalProposal(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalProposalID, params api.AcceptRenewalProposalParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.RenewalStaffAcceptRequest
	if !decode(w, r, &req) {
		return
	}
	in := renewal.AcceptInput{Confirm: req.Confirm == nil || *req.Confirm, Note: customerStr(req.Note), IP: customerClientIP(r)}
	p, err := h.Renewals.Accept(r.Context(), principalOf(r), establishmentID, id, ifMatch, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, renewalProposalToAPI(p))
}

// DeclineRenewalProposal implements POST /renewal-proposals/{proposalId}/decline.
func (h *Handlers) DeclineRenewalProposal(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalProposalID, params api.DeclineRenewalProposalParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.RenewalDeclineRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := h.Renewals.Decline(r.Context(), principalOf(r), establishmentID, id, ifMatch, customerStr(req.Reason), customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, renewalProposalToAPI(p))
}

// SendRenewalProposal implements POST /renewal-proposals/{proposalId}/send.
func (h *Handlers) SendRenewalProposal(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RenewalProposalID, _ api.SendRenewalProposalParams) {
	if h.Renewals == nil {
		notImplemented(w, r)
		return
	}
	p, err := h.Renewals.Send(r.Context(), principalOf(r), establishmentID, id, customerClientIP(r))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, p.Version, renewalProposalToAPI(p))
}

// ---- mapping ----------------------------------------------------------------------

func renewalCampaignToAPI(c *renewal.Campaign) api.RenewalCampaign {
	return api.RenewalCampaign{
		Id: c.ID, Name: c.Name, Status: api.RenewalCampaignStatus(c.Status), SourceSeasonId: c.SourceSeasonID,
		TargetSeasonId: c.TargetSeasonID, DeadlineAt: c.DeadlineAt, Notes: strPtr(c.Notes),
		Counts: api.RenewalStatusCounts{
			Proposed: c.Counts.Proposed, Accepted: c.Counts.Accepted, Declined: c.Counts.Declined,
			Expired: c.Counts.Expired, Converted: c.Counts.Converted, Sent: c.Counts.Sent,
		},
		TotalAmount: int64(c.TotalAmount), ConvertedAmount: int64(c.ConvertedAmount), Version: c.Version,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ClosedAt: c.ClosedAt,
	}
}

// renewalProposalToAPI maps a proposal.
func renewalProposalToAPI(p *renewal.Proposal) api.RenewalProposal {
	out := api.RenewalProposal{
		Id: p.ID, CampaignId: p.CampaignID, Status: api.RenewalProposalStatus(p.Status), SourcePassId: p.SourceBookingID,
		BookingId: p.BookingID, BookingCode: p.BookingCode, CustomerId: p.CustomerID, CustomerDisplayName: p.CustomerDisplayName,
		SpotId: p.SpotID, SpotLabel: p.SpotLabel, ZoneId: strPtr(p.ZoneID), PackId: p.PackID,
		StartsOn: renewalDate(p.StartsOn), EndsOn: renewalDate(p.EndsOn), Slots: api.Slots(p.Slots),
		PreviousAmount: int64(p.PreviousAmount), TotalAmount: int64(p.TotalAmount), DepositDue: int64(p.DepositDue),
		PaidAmount: int64(p.PaidAmount), DeadlineAt: p.DeadlineAt, SendCount: p.SendCount, LastSentAt: p.LastSentAt,
		RespondedAt: p.RespondedAt, DeclineReason: strPtr(p.DeclineReason), Version: p.Version,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	hasEmail := p.CustomerHasEmail
	out.CustomerHasEmail = &hasEmail
	if p.BookingStatus != "" {
		st := api.BookingStatus(p.BookingStatus)
		out.BookingStatus = &st
	}
	if p.ResponseChannel != nil {
		ch := api.RenewalProposalResponseChannel(*p.ResponseChannel)
		out.ResponseChannel = &ch
	}
	return out
}

// renewalDate parses a YYYY-MM-DD of a proposal view (zero when empty).
func renewalDate(s string) api.Date {
	d, _ := calendar.Parse(s)
	return d
}
