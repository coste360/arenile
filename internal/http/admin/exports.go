package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// F. List exports (CSV / XLSX): contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ExportList is not implemented yet.
func (h *Handlers) ExportList(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ExportListKind, _ api.ExportListParams) {
	problem.Write(w, r, problem.NotImplemented)
}
