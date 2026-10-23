package admin

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/fiscal"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// Handlers of the fiscal module (internal/domain/fiscal, spec 14.2): the
// corrispettivi export (JSON or CSV) and the fiscal documents list.
// Permissions are checked by the service (reports.read).

// GetCorrispettivi implements GET /api/admin/v1/establishments/{establishmentId}/fiscal/corrispettivi.
// The format is ?format=csv|json; without it, Accept: text/csv selects CSV.
func (h *Handlers) GetCorrispettivi(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.GetCorrispettiviParams) {
	q := fiscal.CorrispettiviQuery{From: params.From, To: params.To}
	if params.GroupBy != nil {
		q.GroupBy = fiscal.GroupBy(*params.GroupBy)
	}
	rep, err := h.Fiscal.Corrispettivi(r.Context(), principalOf(r), establishmentID, q)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	csv := strings.Contains(r.Header.Get("Accept"), "text/csv")
	if params.Format != nil {
		csv = *params.Format == api.Csv
	}
	if !csv {
		writeJSON(w, http.StatusOK, CorrispettiviToAPI(rep))
		return
	}
	var buf bytes.Buffer
	if err := fiscal.WriteCSV(&buf, rep); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	name := "corrispettivi_" + rep.From.String() + "_" + rep.To.String() + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
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
