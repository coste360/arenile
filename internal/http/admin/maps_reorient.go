package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/mapping"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// Sea side of a map (M5A, spec 11). Owner: map sea-side builder.
//
//	ReorientMap -> Mapping.ReorientMap -> api.MapReorientResult + ETag
//
// If-Match is required (parsed like SaveMap, "*" = current version); the
// ETag of the response is the new version when applied, the current one
// with dry_run or when the sea is already on that side.

// ReorientMap implements POST /api/admin/v1/establishments/{establishmentId}/maps/{mapId}/reorient.
func (h *Handlers) ReorientMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID, params api.ReorientMapParams) {
	ifMatch := params.IfMatch
	m, err := RequireIfMatch(&ifMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var req api.MapReorientRequest
	if !decode(w, r, &req) {
		return
	}
	expected, err := h.mapVersionFor(r, establishmentID, mapID, m)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	res, err := h.Mapping.ReorientMap(r.Context(), principalOf(r), establishmentID, mapID, expected, mapping.ReorientInput{
		SeaSide:                      string(req.SeaSide),
		DryRun:                       req.DryRun != nil && *req.DryRun,
		AcknowledgeFutureAllocations: req.AcknowledgeFutureAllocations != nil && *req.AcknowledgeFutureAllocations,
		IP:                           reqctx.From(r.Context()).ClientIP,
	})
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeVersioned(w, http.StatusOK, res.Document.Map.Version, api.MapReorientResult{
		Applied:                    res.Applied,
		Document:                   documentToAPI(res.Document),
		FromSeaSide:                api.SeaSide(res.From),
		ToSeaSide:                  api.SeaSide(res.To),
		Transform:                  api.MapTransform(res.Transform),
		FutureAllocations:          res.FutureAllocations,
		SpotsWithFutureAllocations: res.SpotsWithFutureAllocations,
	})
}
