package admin

import (
	"io"
	"net/http"

	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the fiscal module (internal/domain/fiscal, spec 14.2): the
// corrispettivi export (JSON or CSV) and the fiscal documents list.
// Permissions are checked by the service (reports.read).

// GetCorrispettivi implements GET /api/admin/v1/establishments/{establishmentId}/fiscal/corrispettivi.
// The format is ?format=csv|json|xlsx; without it, Accept selects CSV or XLSX.
func (h *Handlers) GetCorrispettivi(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetCorrispettiviParams) {
	q := fiscal.CorrispettiviQuery{From: params.From, To: params.To}
	if params.RegisterId != nil {
		q.RegisterID = *params.RegisterId
	}
	if params.GroupBy != nil {
		q.GroupBy = fiscal.GroupBy(*params.GroupBy)
	}
	rep, err := h.Fiscal.Corrispettivi(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	format := acceptedExport(r)
	if params.Format != nil {
		switch *params.Format {
		case api.Csv:
			format = exportCSV
		case api.Xlsx:
			format = exportXLSX
		default:
			format = ""
		}
	}
	if format == "" {
		writeJSON(w, http.StatusOK, CorrispettiviToAPI(rep))
		return
	}
	h.writeExport(w, r, format, "corrispettivi_"+rep.From.String()+"_"+rep.To.String(), func(dst io.Writer) error { return fiscal.WriteCSV(dst, rep) })
}

// ListFiscalDocuments implements GET /api/admin/v1/establishments/{establishmentId}/fiscal/documents.
func (h *Handlers) ListFiscalDocuments(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListFiscalDocumentsParams) {
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := store.FiscalDocumentFilter{From: params.From, To: params.To}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Fiscal.ListDocuments(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.FiscalDocumentList{Items: make([]api.FiscalDocument, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, FiscalDocumentToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// CorrispettiviToAPI maps the export report.
func CorrispettiviToAPI(rep *fiscal.Report) api.CorrispettiviReport {
	out := api.CorrispettiviReport{
		From: rep.From, To: rep.To, GroupBy: api.CorrispettiviReportGroupBy(rep.GroupBy), Currency: rep.Currency,
		GrandTotal: int64(rep.GrandTotal),
		Rows:       make([]api.CorrispettiviRow, 0, len(rep.Rows)),
		Totals:     make([]api.CorrispettiviTotal, 0, len(rep.Totals)),
	}
	for _, row := range rep.Rows {
		out.Rows = append(out.Rows, api.CorrispettiviRow{
			Period: row.Period, VatRateBp: row.RateBP, Method: api.PaymentMethod(row.Method), Count: row.Count,
			Gross: int64(row.Gross), Taxable: int64(row.Taxable), Tax: int64(row.Tax),
		})
	}
	for _, t := range rep.Totals {
		out.Totals = append(out.Totals, api.CorrispettiviTotal{
			VatRateBp: t.RateBP, Gross: int64(t.Gross), Taxable: int64(t.Taxable), Tax: int64(t.Tax),
		})
	}
	return out
}

// FiscalDocumentToAPI maps a fiscal document.
func FiscalDocumentToAPI(d *store.FiscalDocument) api.FiscalDocument {
	return api.FiscalDocument{
		Id: d.ID, PaymentId: d.PaymentID, Driver: d.Driver, Status: api.FiscalDocumentStatus(d.Status),
		DocumentNumber: d.DocumentNumber, BusinessDate: d.BusinessDate, Error: d.Error, CreatedAt: d.CreatedAt.UTC(),
	}
}
