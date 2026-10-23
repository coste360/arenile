package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Reconciliation of online payments (M4, spec 13.3). Owner: payment
// gateway builder.
//
//	GetReconciliationReport -> Gateway.Report                 -> api.ReconciliationReport
//	RunReconciliation       -> Gateway.ReconcileEstablishment -> api.ReconciliationRunResult

// GetReconciliationReport implements GET /api/admin/v1/establishments/{establishmentId}/payments/reconciliation.
func (h *Handlers) GetReconciliationReport(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetReconciliationReportParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RunReconciliation implements POST /api/admin/v1/establishments/{establishmentId}/payments/reconciliation.
func (h *Handlers) RunReconciliation(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.RunReconciliationParams) {
	problem.Write(w, r, problem.NotImplemented)
}
