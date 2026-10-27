package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/findspot"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
)

// Find spot (spec 30.6, ADR 0028).
//
//	FindSpot             -> SpotFinder.Search -> api.FindSpotResult
//	BookFindSpotProposal -> SpotFinder.Book   -> api.BookingDetail (201)

// FindSpot implements POST /find-spot.
func (h *Handlers) FindSpot(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.FindSpotParams) {
	var req api.FindSpotRequest
	if !decode(w, r, &req) {
		return
	}
	q := findspot.Query{
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, Slots: calendar.Slots(req.Slots),
		Count: pcIntOr(req.Count, 1), ZoneIDs: pcSliceOf(req.ZoneIds), PackID: req.PackId, Attributes: pcSliceOf(req.Attributes),
		MaxSpotChanges: pcIntOr(req.MaxSpotChanges, 0), AgreementID: req.AgreementId, CustomerID: req.CustomerId,
		Limit: pcIntOr(req.Limit, findspot.DefaultLimit),
	}
	if req.Formula != nil {
		f := string(*req.Formula)
		q.Formula = &f
	}
	if req.SpotKinds != nil {
		for _, k := range *req.SpotKinds {
			q.SpotKinds = append(q.SpotKinds, string(k))
		}
	}
	res, err := h.SpotFinder.Search(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, findSpotResultToAPI(res))
}

// BookFindSpotProposal implements POST /find-spot/book.
func (h *Handlers) BookFindSpotProposal(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.BookFindSpotProposalParams) {
	var req api.FindSpotBookRequest
	if !decode(w, r, &req) {
		return
	}
	var expected *money.Cents
	if req.ExpectedTotal != nil {
		t := money.Cents(*req.ExpectedTotal)
		expected = &t
	}
	in := bkCreateFromAPI(req.Booking)
	if err := bkCreateM8(&in, &req.Booking); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	d, err := h.SpotFinder.Book(r.Context(), principalOf(r), establishmentID, in, expected)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, d.Booking.Version, BookingDetailToAPI(d))
}

func findSpotResultToAPI(res *findspot.Result) api.FindSpotResult {
	out := api.FindSpotResult{Proposals: []api.FindSpotProposal{}}
	if res.Truncated {
		t := true
		out.Truncated = &t
	}
	for _, p := range res.Proposals {
		ap := api.FindSpotProposal{Id: p.ID, Rank: p.Rank, SpotChanges: p.Proposal.SpotChanges, Lines: []api.FindSpotLine{}, SpotIds: []string{}}
		if p.Proposal.Adjacent {
			a := true
			ap.Adjacent = &a
		}
		for _, l := range p.Proposal.Lines {
			al := api.FindSpotLine{Segments: []api.FindSpotSegment{}}
			for _, sg := range l.Segments {
				id := sg.SpotID.String()
				info := res.Spots[id]
				al.Segments = append(al.Segments, api.FindSpotSegment{
					SpotId: id, SpotLabel: info.Label, ZoneId: info.ZoneID, PriceCategory: strPtr(info.PriceCategory),
					StartsOn: sg.Interval.From, EndsOn: sg.Interval.To,
				})
			}
			ap.Lines = append(ap.Lines, al)
		}
		for _, id := range p.Proposal.SpotIDs() {
			ap.SpotIds = append(ap.SpotIds, id.String())
		}
		if p.Total != nil {
			t := int64(*p.Total)
			ap.Total = &t
			applied := p.AgreementApplied
			ap.AgreementApplied = &applied
		}
		if p.Price != nil {
			bd := BreakdownToAPI(p.Price.Breakdown)
			ap.Price = &bd
			if len(p.Price.Violations) > 0 {
				vs := bkViolations(p.Price.Violations)
				ap.Violations = &vs
			}
		}
		out.Proposals = append(out.Proposals, ap)
	}
	return out
}
