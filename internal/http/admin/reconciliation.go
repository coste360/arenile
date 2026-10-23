package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/payment/gateway"
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
	if h.Gateway == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	q := gateway.ReportQuery{From: params.From, To: params.To}
	if params.IssuesOnly != nil {
		q.IssuesOnly = *params.IssuesOnly
	}
	rep, err := h.Gateway.Report(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, ReconciliationReportToAPI(rep))
}

// RunReconciliation implements POST /api/admin/v1/establishments/{establishmentId}/payments/reconciliation.
func (h *Handlers) RunReconciliation(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.RunReconciliationParams) {
	if h.Gateway == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	st, err := h.Gateway.ReconcileEstablishment(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, api.ReconciliationRunResult{
		Checked: st.Checked, Updated: st.Updated, Failed: st.Failed,
		StartedAt: st.StartedAt.UTC(), FinishedAt: st.FinishedAt.UTC(),
	})
}

// ReconciliationReportToAPI maps the report.
func ReconciliationReportToAPI(rep *gateway.Report) api.ReconciliationReport {
	out := api.ReconciliationReport{
		GeneratedAt: rep.GeneratedAt.UTC(), From: rep.From, To: rep.To, Checked: rep.Checked, Issues: rep.Issues,
		Items: make([]api.ReconciliationItem, 0, len(rep.Items)),
	}
	for i := range rep.Items {
		it := &rep.Items[i]
		pi := &it.Intent
		created := pi.CreatedAt.UTC()
		item := api.ReconciliationItem{
			PaymentIntentId: pi.ID, Provider: pi.Provider, ExternalId: pi.ExternalID, Amount: int64(pi.Amount),
			Kind: api.ReconciliationItemKind(pi.Kind), LocalStatus: api.ReconciliationItemLocalStatus(pi.Status),
			Issue: api.ReconciliationIssue(it.Issue), BookingId: pi.BookingID, CreatedAt: &created,
		}
		if it.BookingCode != "" {
			item.BookingCode = strPtr(it.BookingCode)
		}
		if it.ProviderStatus != "" {
			item.ProviderStatus = strPtr(it.ProviderStatus)
		}
		if pi.LastCheckedAt != nil {
			t := pi.LastCheckedAt.UTC()
			item.LastCheckedAt = &t
		}
		out.Items = append(out.Items, item)
	}
	return out
}
