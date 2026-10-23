package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Fiscal bridge status for the panel (M6, spec 29.1). Owner: fiscal bridge
// builder.
//
//	GetFiscalBridgeStatus -> FiscalBridge.Status -> api.FiscalBridgeStatus

// GetFiscalBridgeStatus implements GET /api/admin/v1/establishments/{establishmentId}/fiscal-bridge.
func (h *Handlers) GetFiscalBridgeStatus(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}
