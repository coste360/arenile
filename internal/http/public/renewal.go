package public

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/renewal"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// M5B renewal proposals from the magic link (spec 10.4, 12.4): handlers of
// internal/domain/renewal (Handlers.Renewals). The responses carry customer
// data: noStore. The token is never logged.

// GetManagedRenewal implements GET /manage/{token}/renewal.
func (h *Handlers) GetManagedRenewal(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	noStore(w)
	if h.Renewals == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, err := h.Renewals.GetByToken(r.Context(), slug, token)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, publicRenewalToAPI(p))
}

// AcceptManagedRenewal implements POST /manage/{token}/renewal/accept.
func (h *Handlers) AcceptManagedRenewal(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken, params api.AcceptManagedRenewalParams) {
	noStore(w)
	if h.Renewals == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.PublicRenewalAcceptRequest
	if !decode(w, r, &req) {
		return
	}
	in := renewal.PublicAcceptInput{
		Payment: string(req.Payment), UseCredit: req.UseCredit != nil && *req.UseCredit, AcceptTermsVersion: req.AcceptTermsVersion,
	}
	if params.IdempotencyKey != nil {
		in.IdempotencyKey = *params.IdempotencyKey
	}
	res, err := h.Renewals.AcceptByToken(r.Context(), slug, token, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.PublicRenewalAcceptResult{Proposal: publicRenewalToAPI(&res.Proposal), Pay: payToAPI(&res.Pay)}
	if res.CreditApplied > 0 {
		out.CreditApplied = ptr(int64(res.CreditApplied))
	}
	writeJSON(w, http.StatusOK, out)
}

// DeclineManagedRenewal implements POST /manage/{token}/renewal/decline.
func (h *Handlers) DeclineManagedRenewal(w http.ResponseWriter, r *http.Request, slug api.Slug, token api.ManageToken) {
	noStore(w)
	if h.Renewals == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.PublicRenewalDeclineRequest
	if !decodeOptional(w, r, &req) {
		return
	}
	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}
	p, err := h.Renewals.DeclineByToken(r.Context(), slug, token, reason)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, publicRenewalToAPI(p))
}

// publicRenewalToAPI maps the customer view of a proposal: no ids of the
// panel, only what the renewal page shows.
func publicRenewalToAPI(p *renewal.PublicProposal) api.PublicRenewalProposal {
	pr := &p.Proposal
	starts, _ := calendar.Parse(pr.StartsOn)
	ends, _ := calendar.Parse(pr.EndsOn)
	out := api.PublicRenewalProposal{
		Status: api.RenewalProposalStatus(pr.Status), SeasonYear: p.SeasonYear, SpotLabel: pr.SpotLabel,
		ZoneName: optStr(p.ZoneName), PackName: optStr(p.PackName), StartsOn: starts, EndsOn: ends, Slots: api.Slots(pr.Slots),
		PreviousAmount: int64(pr.PreviousAmount), TotalAmount: int64(pr.TotalAmount), DepositDue: int64(pr.DepositDue),
		PaidAmount: int64(pr.PaidAmount), DeadlineAt: pr.DeadlineAt, TermsVersion: optStr(p.TermsVersion),
	}
	if p.CreditAvailable > 0 {
		out.CreditAvailable = ptr(int64(p.CreditAvailable))
	}
	out.Allowed.Accept, out.Allowed.Decline = p.CanAccept, p.CanDecline
	out.Allowed.PayDeposit, out.Allowed.PayFull, out.Allowed.UseCredit = p.CanPayDeposit, p.CanPayFull, p.CanUseCredit
	return out
}

// attachRenewal adds the renewal proposal to the manage view of a booking
// that is one (M5B, ManageBooking.renewal); other bookings are unchanged.
func (h *Handlers) attachRenewal(r *http.Request, slug, token string, out *api.ManageBooking) {
	if h.Renewals == nil {
		return
	}
	p, err := h.Renewals.GetByToken(r.Context(), slug, token)
	if err != nil {
		return
	}
	v := publicRenewalToAPI(p)
	out.Renewal = &v
}
