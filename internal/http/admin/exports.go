package admin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coste360/arenile/internal/domain/report"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/xlsx"
)

// Exports in CSV and XLSX (ADR 0019 F). Every XLSX export is the CSV export
// of the same data converted cell by cell (xlsx.FromCSV / xlsx.ParseCell):
// same columns and rows, numbers and dates typed, text never a formula, and
// no data that the CSV does not have.

// Export formats.
const (
	exportCSV  = "csv"
	exportXLSX = "xlsx"
)

// acceptedExport picks the export from the Accept header when no format
// parameter is given ("" = JSON).
func acceptedExport(r *http.Request) string {
	a := r.Header.Get("Accept")
	switch {
	case strings.Contains(a, xlsx.MediaType):
		return exportXLSX
	case strings.Contains(a, "text/csv"):
		return exportCSV
	}
	return ""
}

// writeExport answers a small export (reports, corrispettivi) written by
// writeCSV, as CSV or converted to XLSX. Both are built in memory first, so
// a failure still answers a problem; file is the name without extension.
func (h *Handlers) writeExport(w http.ResponseWriter, r *http.Request, format, file string, writeCSV func(io.Writer) error) {
	var csvBuf bytes.Buffer
	if err := writeCSV(&csvBuf); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	body, ctype, ext := csvBuf.Bytes(), "text/csv; charset=utf-8", ".csv"
	if format == exportXLSX {
		var out bytes.Buffer
		if err := xlsx.FromCSV(&out, &csvBuf, sheetOf(file)); err != nil {
			problem.Error(w, r, h.Log, err)
			return
		}
		body, ctype, ext = out.Bytes(), xlsx.MediaType, ".xlsx"
	}
	setDownloadHeaders(w, ctype, file+ext)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// sheetOf is the sheet name of an export file ("ricavi_2027-07-01_..." ->
// "ricavi").
func sheetOf(file string) string {
	name, _, _ := strings.Cut(file, "_")
	return name
}

func setDownloadHeaders(w http.ResponseWriter, ctype, file string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// listFileNames are the Italian file names of the list exports.
var listFileNames = map[string]string{
	report.ListCustomers: "clienti", report.ListBookings: "prenotazioni",
	report.ListSeasonPasses: "abbonamenti", report.ListPayments: "pagamenti",
}

// ExportList implements GET /exports/{list}: streamed, rows written as they
// are read. Authorisation and validation happen before the first byte; a
// failure after it can only truncate the download (logged).
func (h *Handlers) ExportList(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, list api.ExportListKind, params api.ExportListParams) {
	format := exportCSV
	if params.Format != nil && *params.Format == api.ExportFormatXLSX {
		format = exportXLSX
	}
	x, err := h.Reports.PrepareList(r.Context(), principalOf(r), establishmentID, string(list), params.From, params.To)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	file := listFileNames[x.Kind()]
	if params.From != nil {
		file += "_" + params.From.String()
	}
	if params.To != nil {
		file += "_" + params.To.String()
	}
	var sink interface {
		report.RowSink
		Flush() error
	}
	if format == exportXLSX {
		setDownloadHeaders(w, xlsx.MediaType, file+".xlsx")
		xw, err := xlsx.NewWriter(w, xlsx.Options{Sheet: listFileNames[x.Kind()], Header: true})
		if err != nil {
			h.logStreamError(r.Context(), err)
			return
		}
		sink = &xlsxSink{w: xw}
	} else {
		setDownloadHeaders(w, "text/csv; charset=utf-8", file+".csv")
		sink = report.NewCSVWriter(w)
	}
	err = x.Write(r.Context(), sink)
	if ferr := sink.Flush(); err == nil {
		err = ferr
	}
	if err != nil {
		h.logStreamError(r.Context(), err)
	}
}

// logStreamError logs an export interrupted after the headers were sent; a
// client that went away is not an error.
func (h *Handlers) logStreamError(ctx context.Context, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		h.Log.DebugContext(ctx, "export interrupted by the client")
		return
	}
	h.Log.ErrorContext(ctx, "export failed while streaming", slog.String("error", err.Error()))
}

// xlsxSink adapts xlsx.Writer to report.RowSink: the header row stays text,
// the others are typed with xlsx.ParseCell (the CSV field format).
type xlsxSink struct {
	w    *xlsx.Writer
	rows int
	err  error
}

func (s *xlsxSink) Row(fields ...string) {
	if s.err != nil {
		return
	}
	cells := make([]xlsx.Cell, len(fields))
	for i, f := range fields {
		if s.rows == 0 {
			cells[i] = xlsx.Text(f)
		} else {
			cells[i] = xlsx.ParseCell(f)
		}
	}
	s.rows++
	s.err = s.w.Row(cells...)
}

func (s *xlsxSink) Flush() error {
	if err := s.w.Close(); s.err == nil {
		s.err = err
	}
	return s.err
}
