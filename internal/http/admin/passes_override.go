package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// E. Manual price override of season passes: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// SetSeasonPassPriceOverride is not implemented yet.
func (h *Handlers) SetSeasonPassPriceOverride(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID, _ api.SetSeasonPassPriceOverrideParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ClearSeasonPassPriceOverride is not implemented yet.
func (h *Handlers) ClearSeasonPassPriceOverride(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.SeasonPassID, _ api.ClearSeasonPassPriceOverrideParams) {
	problem.Write(w, r, problem.NotImplemented)
}
