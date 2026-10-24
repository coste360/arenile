package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/money"
)

// E. Manual price override of season passes (ADR 0019 E). If-Match is
// required (428 without it, "*" saves over the current version); the
// service checks establishment.configure.

// requiredVersion parses a mandatory If-Match: nil version for "*".
func requiredVersion(h string) (*int64, error) {
	m, err := RequireIfMatch(&h)
	if err != nil || m.Any {
		return nil, err
	}
	return &m.Version, nil
}

// SetSeasonPassPriceOverride implements PUT /season-passes/{passId}/price-override.
func (h *Handlers) SetSeasonPassPriceOverride(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID, params api.SetSeasonPassPriceOverrideParams) {
	ifMatch, err := requiredVersion(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.SeasonPassPriceOverrideInput
	if !decode(w, r, &req) {
		return
	}
	d, err := h.Passes.SetPriceOverride(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch, money.Cents(req.Amount), req.Reason)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Pass.Booking.Version, seasonPassDetailToAPI(d))
}

// ClearSeasonPassPriceOverride implements DELETE /season-passes/{passId}/price-override.
func (h *Handlers) ClearSeasonPassPriceOverride(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.SeasonPassID, params api.ClearSeasonPassPriceOverrideParams) {
	ifMatch, err := requiredVersion(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	d, err := h.Passes.ClearPriceOverride(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, d.Pass.Booking.Version, seasonPassDetailToAPI(d))
}
