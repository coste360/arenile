package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// F. CSV import of customers and season passes: contract stubs (501 not_implemented) until the builder of
// this area wires the domain service. See docs/adr/0019-funzioni-rinviate.md.

// ListImportJobs is not implemented yet.
func (h *Handlers) ListImportJobs(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ListImportJobsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// CreateImportJob is not implemented yet.
func (h *Handlers) CreateImportJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.CreateImportJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetImportTemplate is not implemented yet.
func (h *Handlers) GetImportTemplate(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ImportKind) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetImportJob is not implemented yet.
func (h *Handlers) GetImportJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ImportJobID) {
	problem.Write(w, r, problem.NotImplemented)
}

// DiscardImportJob is not implemented yet.
func (h *Handlers) DiscardImportJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ImportJobID, _ api.DiscardImportJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// PreviewImportJob is not implemented yet.
func (h *Handlers) PreviewImportJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ImportJobID, _ api.PreviewImportJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ApplyImportJob is not implemented yet.
func (h *Handlers) ApplyImportJob(w http.ResponseWriter, r *http.Request, _ api.EstablishmentID, _ api.ImportJobID, _ api.ApplyImportJobParams) {
	problem.Write(w, r, problem.NotImplemented)
}
