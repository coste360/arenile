package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/pass"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the pass domain (internal/domain/pass): season passes are
// bookings with is_season_pass (same id); releases free half-days for
// resale. Decode, call the service (which checks permissions), encode.

// ListSeasonPasses implements GET /api/admin/v1/establishments/{establishmentId}/season-passes.
func (h *Handlers) ListSeasonPasses(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListSeasonPassesParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := pass.ListFilter{SeasonID: customerStr(params.SeasonId), CustomerID: customerStr(params.CustomerId), SpotID: customerStr(params.SpotId)}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Passes.List(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.SeasonPassList{Items: make([]api.SeasonPass, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, SeasonPassToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateSeasonPass implements POST /api/admin/v1/establishments/{establishmentId}/season-passes.
func (h *Handlers) CreateSeasonPass(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateSeasonPassParams) {
	var req api.SeasonPassCreate
	if !decode(w, r, &req) {
		return
	}
	in := pass.CreateInput{
		CustomerID: req.CustomerId, SeasonID: req.SeasonId, SpotID: req.SpotId, PackID: req.PackId,
		StartsOn: req.StartsOn, EndsOn: req.EndsOn, NotesInternal: customerStr(req.NotesInternal),
		RenewalOfBookingID: customerStr(req.RenewalOfBookingId),
	}
	if req.Slots != nil {
		in.Slots = calendar.Slots(*req.Slots)
	}
	if req.Extras != nil {
		for _, x := range *req.Extras {
			in.Extras = append(in.Extras, booking.ExtraInput{ExtraID: x.ExtraId, Quantity: x.Quantity})
		}
	}
	d, err := h.Passes.Create(r.Context(), principalOf(r), customerClientIP(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, d.Pass.Booking.Version, seasonPassDetailToAPI(d))
}

// GetSeasonPass implements GET /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}.
func (h *Handlers) GetSeasonPass(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID) {
	d, err := h.Passes.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Pass.Booking.Version, seasonPassDetailToAPI(d))
}

// UpdateSeasonPass implements PATCH /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}.
func (h *Handlers) UpdateSeasonPass(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID, params api.UpdateSeasonPassParams) {
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.SeasonPassUpdate
	if !decode(w, r, &req) {
		return
	}
	d, err := h.Passes.Update(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch,
		pass.UpdateInput{NotesCustomer: req.NotesCustomer, NotesInternal: req.NotesInternal})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Pass.Booking.Version, seasonPassDetailToAPI(d))
}

// ListSeasonPassReleases implements GET /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases.
func (h *Handlers) ListSeasonPassReleases(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID) {
	rl, err := h.Passes.ListReleases(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.SeasonReleaseList{Items: seasonReleasesToAPI(rl.Releases, rl.ResoldBookingIDs)})
}

// CreateSeasonPassReleases implements POST /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases.
func (h *Handlers) CreateSeasonPassReleases(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID, _ api.CreateSeasonPassReleasesParams) {
	var req api.SeasonReleaseCreate
	if !decode(w, r, &req) {
		return
	}
	in := pass.ReleaseInput{Dates: req.Dates, Note: customerStr(req.Note)}
	if req.Slots != nil {
		in.Slots = calendar.Slots(*req.Slots)
	}
	in.Range = releaseRangeFromAPI(req.From, req.To, req.Weekdays)
	rows, err := h.Passes.Release(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, api.SeasonReleaseList{Items: seasonReleasesToAPI(rows, nil)})
}

// RevokeSeasonPassRelease implements DELETE /api/admin/v1/establishments/{establishmentId}/season-passes/{passId}/releases/{releaseId}.
func (h *Handlers) RevokeSeasonPassRelease(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID, releaseID api.ReleaseID, _ api.RevokeSeasonPassReleaseParams) {
	if err := h.Passes.Revoke(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, releaseID); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- mapping ----------------------------------------------------------------

// SeasonPassToAPI maps a season pass view (also used by the customer page).
func SeasonPassToAPI(v *store.SeasonPassView) api.SeasonPass {
	b := &v.Booking
	out := api.SeasonPass{
		Id: b.ID, Code: b.Code, Status: api.BookingStatus(b.Status), CustomerId: b.CustomerID,
		CustomerDisplayName: v.CustomerDisplayName, SpotId: v.SpotID, SpotLabel: v.SpotLabel, PackId: v.PackID,
		StartsOn: b.StartsOn, EndsOn: b.EndsOn, Slots: api.Slots(b.Slots),
		TotalAmount: int64(b.TotalAmount), PaidAmount: int64(b.PaidAmount), BalanceDue: int64(b.BalanceDue()),
		ReleasedDays: v.ReleasedDays, CreditAmount: int64(v.CreditAmount), CreatedAt: b.CreatedAt, Version: b.Version,
	}
	if b.SeasonID != nil {
		out.SeasonId = *b.SeasonID
	}
	out.PriceOverride = seasonPassOverrideToAPI(b)
	out.Test = testFlag(b.Test)
	return out
}

// seasonPassOverrideToAPI maps the manual price of a pass (nil when none).
func seasonPassOverrideToAPI(b *store.Booking) *api.SeasonPassPriceOverride {
	if b.PriceOverrideAmount == nil || b.PriceOverrideAt == nil {
		return nil
	}
	o := &api.SeasonPassPriceOverride{Amount: int64(*b.PriceOverrideAmount), SetAt: *b.PriceOverrideAt, SetByUserId: b.PriceOverrideByUserID}
	if b.PriceOverrideEngineAmount != nil {
		o.EngineAmount = int64(*b.PriceOverrideEngineAmount)
	}
	if b.PriceOverrideReason != nil {
		o.Reason = *b.PriceOverrideReason
	}
	return o
}

func seasonPassDetailToAPI(d *pass.Detail) api.SeasonPassDetail {
	return api.SeasonPassDetail{Pass: SeasonPassToAPI(&d.Pass), Releases: seasonReleasesToAPI(d.Releases, d.ResoldBookingIDs)}
}

// seasonReleasesToAPI maps releases; resold maps resale line ids to booking ids.
func seasonReleasesToAPI(rows []store.SeasonRelease, resold map[string]string) []api.SeasonRelease {
	out := make([]api.SeasonRelease, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		x := api.SeasonRelease{
			Id: r.ID, PassId: r.BookingID, Date: r.Date, Slot: api.Slot(r.Slot), CreditAmount: int64(r.CreditAmount),
			RevokedAt: r.RevokedAt, CreatedAt: r.CreatedAt, Status: api.SeasonReleaseStatusReleased,
		}
		if r.Note != "" {
			note := r.Note
			x.Note = &note
		}
		switch {
		case r.RevokedAt != nil:
			x.Status = api.SeasonReleaseStatusRevoked
		case r.ResoldBookingLineID != nil:
			x.Status = api.SeasonReleaseStatusResold
		}
		if r.ResoldBookingLineID != nil {
			if id, ok := resold[*r.ResoldBookingLineID]; ok {
				x.ResoldBookingId = &id
			}
		}
		out = append(out, x)
	}
	return out
}
