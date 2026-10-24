package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/coste360/arenile/internal/domain/importer"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// F. CSV import of customers and season passes (internal/domain/importer,
// ADR 0019): decode, call the service (which checks establishment.configure),
// encode. Responses never contain the file content, only the report (row,
// column, code); they are still marked no-store.

// ListImportJobs implements GET /establishments/{establishmentId}/imports.
func (h *Handlers) ListImportJobs(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListImportJobsParams) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	page, err := h.Imports.List(r.Context(), principalOf(r), establishmentID, cursor, limit)
	if err != nil {
		h.importError(w, r, err)
		return
	}
	out := api.ImportJobList{Items: make([]api.ImportJob, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, ImportJobToAPI(&page.Items[i]))
	}
	customerNoStore(w)
	writeJSON(w, http.StatusOK, out)
}

// CreateImportJob implements POST /establishments/{establishmentId}/imports
// (raw text/csv body, at most importer.MaxFileBytes).
func (h *Handlers) CreateImportJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreateImportJobParams) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	if r.Body == nil {
		problem.Write(w, r, problem.ImportInvalid, problem.Detail("Il file è vuoto."))
		return
	}
	content, err := io.ReadAll(io.LimitReader(r.Body, importer.MaxFileBytes+1))
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) || len(content) > importer.MaxFileBytes {
		problem.Write(w, r, problem.ImportInvalid, problem.Detail("Il file supera i 5 MB."))
		return
	}
	if err != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("Impossibile leggere il file."))
		return
	}
	filename := ""
	if params.Filename != nil {
		filename = *params.Filename
	}
	j, err := h.Imports.Create(r.Context(), principalOf(r), establishmentID, string(params.Kind), filename, content)
	if err != nil {
		h.importError(w, r, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusCreated, j.Version, ImportJobToAPI(j))
}

// GetImportTemplate implements GET /establishments/{establishmentId}/imports/templates/{kind}.
func (h *Handlers) GetImportTemplate(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, kind api.ImportKind) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	b, err := h.Imports.Template(r.Context(), principalOf(r), establishmentID, string(kind))
	if err != nil {
		h.importError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="arenile-`+string(kind)+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b) //nolint:gosec // G705: server-built CSV template served as a text/csv attachment
}

// GetImportJob implements GET /establishments/{establishmentId}/imports/{importId}.
func (h *Handlers) GetImportJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ImportJobID) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	j, err := h.Imports.Get(r.Context(), principalOf(r), establishmentID, id)
	if err != nil {
		h.importError(w, r, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, j.Version, ImportJobToAPI(j))
}

// DiscardImportJob implements DELETE /establishments/{establishmentId}/imports/{importId}.
func (h *Handlers) DiscardImportJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ImportJobID, _ api.DiscardImportJobParams) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	if err := h.Imports.Discard(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id); err != nil {
		h.importError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PreviewImportJob implements POST /establishments/{establishmentId}/imports/{importId}/preview.
func (h *Handlers) PreviewImportJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ImportJobID, params api.PreviewImportJobParams) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	var req api.ImportOptions
	if !decode(w, r, &req) {
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	var opts importer.Options
	if b, err := json.Marshal(req); err == nil {
		_ = json.Unmarshal(b, &opts)
	}
	j, err := h.Imports.Preview(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch, opts)
	if err != nil {
		h.importError(w, r, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, j.Version, ImportJobToAPI(j))
}

// ApplyImportJob implements POST /establishments/{establishmentId}/imports/{importId}/apply.
func (h *Handlers) ApplyImportJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, id api.ImportJobID, params api.ApplyImportJobParams) {
	if h.Imports == nil {
		notImplemented(w, r)
		return
	}
	ifMatch, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	j, err := h.Imports.Apply(r.Context(), principalOf(r), customerClientIP(r), establishmentID, id, ifMatch)
	if err != nil {
		h.importError(w, r, err)
		return
	}
	customerNoStore(w)
	writeVersioned(w, http.StatusOK, j.Version, ImportJobToAPI(j))
}

// importError maps the importer errors with their own problem types and
// delegates the others to problem.Error.
func (h *Handlers) importError(w http.ResponseWriter, r *http.Request, err error) {
	var inv *importer.InvalidError
	switch {
	case errors.As(err, &inv):
		problem.Write(w, r, problem.ImportInvalid, problem.Detail(inv.Detail))
	case errors.Is(err, importer.ErrAlreadyApplied):
		problem.Write(w, r, problem.ImportAlreadyApplied)
	default:
		problem.Error(w, r, h.Log, err)
	}
}

// ImportJobToAPI maps an import job (never its content) to api.ImportJob.
func ImportJobToAPI(j *store.ImportJob) api.ImportJob {
	out := api.ImportJob{
		Id: j.ID, Kind: api.ImportKind(j.Kind), Status: api.ImportJobStatus(j.Status), Filename: j.Filename,
		SizeBytes: j.SizeBytes, Sha256: j.SHA256, CreatedByUserId: j.CreatedByUserID, AppliedAt: j.AppliedAt,
		Version: j.Version, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
	}
	remarshal(j.Options, &out.Options)
	if len(j.Report) > 0 {
		var rep api.ImportReport
		remarshal(j.Report, &rep)
		if rep.Errors == nil {
			rep.Errors = []api.ImportIssue{}
		}
		if rep.Warnings == nil {
			rep.Warnings = []api.ImportIssue{}
		}
		out.Report = &rep
	}
	return out
}

// remarshal converts a stored JSON map into an API type.
func remarshal(src map[string]any, dst any) {
	if src == nil {
		return
	}
	if b, err := json.Marshal(src); err == nil {
		_ = json.Unmarshal(b, dst)
	}
}
