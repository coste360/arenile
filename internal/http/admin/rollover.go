package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/rollover"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// M5B season rollover (spec 8.7, 10.4, 28): handlers of
// internal/domain/rollover (Handlers.Rollover). Decode, call the service
// (permissions, idempotency and the transaction live there), encode.

// RolloverSeason implements POST /seasons/{seasonId}/rollover.
func (h *Handlers) RolloverSeason(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, seasonID api.SeasonID, _ api.RolloverSeasonParams) {
	if h.Rollover == nil {
		notImplemented(w, r)
		return
	}
	var req api.SeasonRolloverRequest
	if !decode(w, r, &req) {
		return
	}
	def := func(b *bool, v bool) bool {
		if b == nil {
			return v
		}
		return *b
	}
	in := rollover.Input{
		TargetYear: req.TargetYear, OpensOn: req.OpensOn, ClosesOn: req.ClosesOn,
		IncludeRatePeriods: def(req.IncludeRatePeriods, true), IncludeClosureDays: def(req.IncludeClosureDays, true),
		IncludePriceRules: def(req.IncludePriceRules, true), IncludeFixedPrices: def(req.IncludeFixedPrices, true),
		IncludePolicies: def(req.IncludePolicies, true), DryRun: def(req.DryRun, true), IP: customerClientIP(r),
	}
	res, created, err := h.Rollover.Rollover(r.Context(), principalOf(r), establishmentID, seasonID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rolloverResultToAPI(res))
}

// ListSeasonRollovers implements GET /season-rollovers.
func (h *Handlers) ListSeasonRollovers(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListSeasonRolloversParams) {
	if h.Rollover == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Rollover.List(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.SeasonRolloverList{Items: make([]api.SeasonRollover, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, rolloverToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func rolloverCountsToAPI(c rollover.Counts) api.SeasonRolloverCounts {
	return api.SeasonRolloverCounts{
		RatePeriods: c.RatePeriods, ClosureDays: c.ClosureDays, PriceRules: c.PriceRules, FixedPrices: c.FixedPrices, Policies: c.Policies,
	}
}

func rolloverResultToAPI(res *rollover.Result) api.SeasonRolloverResult {
	passes := res.SeasonPasses
	out := api.SeasonRolloverResult{
		DryRun: res.DryRun, AlreadyApplied: res.AlreadyApplied, SourceSeasonId: res.SourceSeasonID, TargetYear: res.TargetYear,
		OpensOn: res.OpensOn, ClosesOn: res.ClosesOn, Counts: rolloverCountsToAPI(res.Counts), SeasonPasses: &passes,
		Warnings: res.Warnings,
	}
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	if res.RolloverID != "" {
		id := res.RolloverID
		out.RolloverId = &id
	}
	if res.Season != nil {
		s := SeasonToAPI(res.Season)
		out.Season = &s
	}
	return out
}

func rolloverToAPI(r *store.SeasonRollover) api.SeasonRollover {
	return api.SeasonRollover{
		Id: r.ID, SourceSeasonId: r.SourceSeasonID, TargetSeasonId: r.TargetSeasonID, TargetYear: r.TargetYear,
		Counts: rolloverCountsToAPI(rollover.CountsOf(r)), CreatedByUserId: r.CreatedByUserID, CreatedAt: r.CreatedAt,
	}
}
