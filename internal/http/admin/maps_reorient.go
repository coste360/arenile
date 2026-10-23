package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Sea side of a map (M5A, spec 11). Owner: map sea-side builder.
//
//	ReorientMap -> Mapping.ReorientMap -> api.MapReorientResult + ETag
//
// If-Match is required (parse it like SaveMap); the ETag of the response is
// the new version when applied, the current one with dry_run.

// ReorientMap implements POST /api/admin/v1/establishments/{establishmentId}/maps/{mapId}/reorient.
func (h *Handlers) ReorientMap(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, mapID api.MapID, params api.ReorientMapParams) {
	problem.Write(w, r, problem.NotImplemented)
}
