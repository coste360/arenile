package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// ESC/POS network printers, document routing and print jobs: contract stubs
// (501 not_implemented) until the builders wire internal/domain/printing.
// See docs/adr/0021-stampa-escpos-di-rete.md.

// ListPrinters is not implemented yet.
func (h *Handlers) ListPrinters(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPrintersParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreatePrinter is not implemented yet.
func (h *Handlers) CreatePrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePrinterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetPrinter is not implemented yet.
func (h *Handlers) GetPrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrinterID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdatePrinter is not implemented yet.
func (h *Handlers) UpdatePrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrinterID, _ api.UpdatePrinterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// DeletePrinter is not implemented yet.
func (h *Handlers) DeletePrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrinterID, _ api.DeletePrinterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CheckPrinter is not implemented yet.
func (h *Handlers) CheckPrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrinterID, _ api.CheckPrinterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// TestPrinter is not implemented yet.
func (h *Handlers) TestPrinter(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrinterID, _ api.TestPrinterParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetPrintRoutes is not implemented yet.
func (h *Handlers) GetPrintRoutes(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID) {
	problem.Write(w, r, problem.NotImplemented)
}

// UpdatePrintRoutes is not implemented yet.
func (h *Handlers) UpdatePrintRoutes(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.UpdatePrintRoutesParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ResolvePrintRoute is not implemented yet.
func (h *Handlers) ResolvePrintRoute(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ResolvePrintRouteParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListPrintJobs is not implemented yet.
func (h *Handlers) ListPrintJobs(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListPrintJobsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreatePrintJob is not implemented yet.
func (h *Handlers) CreatePrintJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreatePrintJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetPrintJob is not implemented yet.
func (h *Handlers) GetPrintJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrintJobID) {
	problem.Write(w, r, problem.NotImplemented)
}

// RetryPrintJob is not implemented yet.
func (h *Handlers) RetryPrintJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.PrintJobID, _ api.RetryPrintJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}
