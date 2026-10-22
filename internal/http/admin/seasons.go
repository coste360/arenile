package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/season"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of seasons, rate periods and closure days (spec 6.1). Owned by the
// season builder (internal/domain/season). CopySeason lives in pricing.go
// and composes season.Service.CopySeason with the price list copy.

// ---- seasons -------------------------------------------------------------------

// ListSeasons implements GET /establishments/{establishmentId}/seasons.
func (h *Handlers) ListSeasons(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListSeasonsParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Seasons.ListSeasons(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.SeasonList{Items: make([]api.Season, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, SeasonToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateSeason implements POST /establishments/{establishmentId}/seasons.
func (h *Handlers) CreateSeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateSeasonParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	var req api.SeasonInput
	if !decode(w, r, &req) {
		return
	}
	sn, err := h.Seasons.CreateSeason(r.Context(), principalOf(r), establishmentID, seasonInputFromAPI(req, nil, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, sn.Version, SeasonToAPI(sn))
}

// GetSeason implements GET /establishments/{establishmentId}/seasons/{seasonId}.
func (h *Handlers) GetSeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonID) {
	if !h.seasonsReady(w, r) {
		return
	}
	sn, err := h.Seasons.GetSeason(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, sn.Version, SeasonToAPI(sn))
}

// UpdateSeason implements PUT /establishments/{establishmentId}/seasons/{seasonId}.
func (h *Handlers) UpdateSeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonID, params api.UpdateSeasonParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.SeasonInput
	if !decode(w, r, &req) {
		return
	}
	sn, err := h.Seasons.UpdateSeason(r.Context(), principalOf(r), establishmentID, id, seasonInputFromAPI(req, ifMatch, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, sn.Version, SeasonToAPI(sn))
}

// DeleteSeason implements DELETE /establishments/{establishmentId}/seasons/{seasonId}.
func (h *Handlers) DeleteSeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonID, params api.DeleteSeasonParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Seasons.DeleteSeason(r.Context(), principalOf(r), establishmentID, id, ifMatch, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func seasonInputFromAPI(req api.SeasonInput, ifMatch *int64, ip string) season.SeasonInput {
	in := season.SeasonInput{Year: req.Year, OpensOn: req.OpensOn, ClosesOn: req.ClosesOn, IfMatch: ifMatch, IP: ip}
	if req.Status != nil {
		in.Status = season.Status(*req.Status)
	}
	return in
}

// SeasonToAPI maps a season.
func SeasonToAPI(s *store.Season) api.Season {
	return api.Season{
		Id: s.ID, EstablishmentId: s.EstablishmentID, Year: s.Year, OpensOn: s.OpensOn, ClosesOn: s.ClosesOn,
		Status: api.SeasonStatus(s.Status), Version: s.Version, CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
}

// ---- rate periods --------------------------------------------------------------

// ListRatePeriods implements GET /establishments/{establishmentId}/rate-periods.
func (h *Handlers) ListRatePeriods(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListRatePeriodsParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	seasonID := ""
	if params.SeasonId != nil {
		seasonID = *params.SeasonId
	}
	page, err := h.Seasons.ListRatePeriods(r.Context(), principalOf(r), establishmentID, seasonID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.RatePeriodList{Items: make([]api.RatePeriod, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, RatePeriodToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateRatePeriod implements POST /establishments/{establishmentId}/rate-periods.
func (h *Handlers) CreateRatePeriod(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateRatePeriodParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	var req api.RatePeriodInput
	if !decode(w, r, &req) {
		return
	}
	rp, err := h.Seasons.CreateRatePeriod(r.Context(), principalOf(r), establishmentID, ratePeriodInputFromAPI(req, nil, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, rp.Version, RatePeriodToAPI(rp))
}

// GetRatePeriod implements GET /establishments/{establishmentId}/rate-periods/{ratePeriodId}.
func (h *Handlers) GetRatePeriod(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RatePeriodID) {
	if !h.seasonsReady(w, r) {
		return
	}
	rp, err := h.Seasons.GetRatePeriod(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, rp.Version, RatePeriodToAPI(rp))
}

// UpdateRatePeriod implements PUT /establishments/{establishmentId}/rate-periods/{ratePeriodId}.
func (h *Handlers) UpdateRatePeriod(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RatePeriodID, params api.UpdateRatePeriodParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.RatePeriodInput
	if !decode(w, r, &req) {
		return
	}
	rp, err := h.Seasons.UpdateRatePeriod(r.Context(), principalOf(r), establishmentID, id, ratePeriodInputFromAPI(req, ifMatch, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, rp.Version, RatePeriodToAPI(rp))
}

// DeleteRatePeriod implements DELETE /establishments/{establishmentId}/rate-periods/{ratePeriodId}.
func (h *Handlers) DeleteRatePeriod(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.RatePeriodID, params api.DeleteRatePeriodParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Seasons.DeleteRatePeriod(r.Context(), principalOf(r), establishmentID, id, ifMatch, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ratePeriodInputFromAPI(req api.RatePeriodInput, ifMatch *int64, ip string) season.RatePeriodInput {
	in := season.RatePeriodInput{
		SeasonID: req.SeasonId, Code: req.Code, Name: req.Name, Color: estDeref(req.Color),
		Ranges: make([]store.DateRange, len(req.Ranges)), SortOrder: req.SortOrder, IfMatch: ifMatch, IP: ip,
	}
	for i, rg := range req.Ranges {
		in.Ranges[i] = store.DateRange{From: rg.From, To: rg.To}
	}
	return in
}

// RatePeriodToAPI maps a rate period.
func RatePeriodToAPI(p *store.RatePeriod) api.RatePeriod {
	out := api.RatePeriod{
		Id: p.ID, SeasonId: p.SeasonID, Code: p.Code, Name: api.I18nText(p.Name), Color: p.Color,
		Ranges: make([]api.DateRange, len(p.Ranges)), SortOrder: p.SortOrder, Version: p.Version,
		CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(),
	}
	if out.Name == nil {
		out.Name = api.I18nText{}
	}
	for i, rg := range p.Ranges {
		out.Ranges[i] = api.DateRange{From: rg.From, To: rg.To}
	}
	return out
}

// ---- closure days --------------------------------------------------------------

// ListClosureDays implements GET /establishments/{establishmentId}/closure-days.
func (h *Handlers) ListClosureDays(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListClosureDaysParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var from, to calendar.Date
	ve := &configerr.ValidationError{}
	if params.From != nil {
		if from, err = calendar.Parse(*params.From); err != nil {
			ve.Add("from", "Data non valida (AAAA-MM-GG).")
		}
	}
	if params.To != nil {
		if to, err = calendar.Parse(*params.To); err != nil {
			ve.Add("to", "Data non valida (AAAA-MM-GG).")
		}
	}
	if err := ve.OrNil(); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Seasons.ListClosureDays(r.Context(), principalOf(r), establishmentID, from, to, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.ClosureDayList{Items: make([]api.ClosureDay, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, ClosureDayToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateClosureDay implements POST /establishments/{establishmentId}/closure-days.
func (h *Handlers) CreateClosureDay(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateClosureDayParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	var req api.ClosureDayInput
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Seasons.CreateClosureDay(r.Context(), principalOf(r), establishmentID, closureInputFromAPI(req, nil, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusCreated, c.Version, ClosureDayToAPI(c))
}

// GetClosureDay implements GET /establishments/{establishmentId}/closure-days/{closureDayId}.
func (h *Handlers) GetClosureDay(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ClosureDayID) {
	if !h.seasonsReady(w, r) {
		return
	}
	c, err := h.Seasons.GetClosureDay(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, ClosureDayToAPI(c))
}

// UpdateClosureDay implements PUT /establishments/{establishmentId}/closure-days/{closureDayId}.
func (h *Handlers) UpdateClosureDay(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ClosureDayID, params api.UpdateClosureDayParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.ClosureDayInput
	if !decode(w, r, &req) {
		return
	}
	c, err := h.Seasons.UpdateClosureDay(r.Context(), principalOf(r), establishmentID, id, closureInputFromAPI(req, ifMatch, estClientIP(r)))
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, c.Version, ClosureDayToAPI(c))
}

// DeleteClosureDay implements DELETE /establishments/{establishmentId}/closure-days/{closureDayId}.
func (h *Handlers) DeleteClosureDay(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ClosureDayID, params api.DeleteClosureDayParams) {
	if !h.seasonsReady(w, r) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	if err := h.Seasons.DeleteClosureDay(r.Context(), principalOf(r), establishmentID, id, ifMatch, estClientIP(r)); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func closureInputFromAPI(req api.ClosureDayInput, ifMatch *int64, ip string) season.ClosureDayInput {
	in := season.ClosureDayInput{Date: req.Date, Reason: estDeref(req.Reason), Slots: make([]calendar.Slot, len(req.Slots)), IfMatch: ifMatch, IP: ip}
	for i, s := range req.Slots {
		in.Slots[i] = calendar.Slot(s)
	}
	return in
}

// ClosureDayToAPI maps a closure day.
func ClosureDayToAPI(c *store.ClosureDay) api.ClosureDay {
	out := api.ClosureDay{
		Id: c.ID, Date: c.Date, Reason: c.Reason, Slots: make([]api.Slot, len(c.Slots)), Version: c.Version,
		CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
	}
	for i, s := range c.Slots {
		out.Slots[i] = api.Slot(s)
	}
	return out
}

// seasonsReady answers 501 when the season service is not wired.
func (h *Handlers) seasonsReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Seasons == nil {
		notImplemented(w, r)
		return false
	}
	return true
}
