package admin

import (
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/domain/printing"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/store"
)

// ESC/POS network printers, document routing and print jobs (ADR 0021):
// thin handlers over internal/domain/printing. The printing errors are
// mapped here (printingError) so that problem.go stays untouched.

// printingError maps the printing domain errors, then the shared ones.
func (h *Handlers) printingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, printing.ErrUnavailable):
		problem.Write(w, r, problem.PrintingUnavailable)
	case errors.Is(err, printing.ErrAddressNotAllowed):
		problem.Write(w, r, problem.PrinterAddressNotAllowed, problem.Fields(api.FieldError{
			Field: "host", Message: "Usa un indirizzo della rete locale (es. 192.168.1.50).",
		}))
	case errors.Is(err, printing.ErrRouteMissing):
		problem.Write(w, r, problem.PrintRouteMissing)
	case errors.Is(err, printing.ErrPrinterInactive):
		problem.Write(w, r, problem.Conflict, problem.Detail("La stampante è disattivata."))
	case errors.Is(err, printing.ErrInvalidTransition):
		problem.Write(w, r, problem.InvalidTransition, problem.Detail("Si può riprovare solo un lavoro non riuscito."))
	case errors.Is(err, printing.ErrIdempotencyKeyReused):
		problem.Write(w, r, problem.IdempotencyKeyReused)
	case errors.Is(err, printing.ErrNotFound):
		problem.Write(w, r, problem.NotFound)
	default:
		problem.Error(w, r, h.Log, err)
	}
}

func (h *Handlers) printingReady(w http.ResponseWriter, r *http.Request) bool {
	if h.Printing == nil {
		notImplemented(w, r)
		return false
	}
	return true
}

// ListPrinters implements GET /printers.
func (h *Handlers) ListPrinters(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPrintersParams) {
	if !h.printingReady(w, r) {
		return
	}
	ps, err := h.Printing.ListPrinters(r.Context(), principalOf(r), establishmentID, params.IncludeInactive != nil && *params.IncludeInactive)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	tickets := h.Printing.TicketsAvailable()
	out := api.PrinterList{Items: make([]api.Printer, len(ps)), EscposAvailable: h.Printing.Available(), TicketsAvailable: &tickets}
	for i := range ps {
		out.Items[i] = PrinterToAPI(&ps[i])
	}
	if !out.EscposAvailable {
		reason := api.PrinterUnavailableReasonServerEdition
		out.UnavailableReason = &reason
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePrinter implements POST /printers.
func (h *Handlers) CreatePrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreatePrinterParams) {
	if !h.printingReady(w, r) {
		return
	}
	var req api.PrinterInput
	if !decode(w, r, &req) {
		return
	}
	pr, err := h.Printing.CreatePrinter(r.Context(), principalOf(r), establishmentID, printerInputFromAPI(req))
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeVersioned(w, http.StatusCreated, pr.Version, PrinterToAPI(pr))
}

// GetPrinter implements GET /printers/{printerId}.
func (h *Handlers) GetPrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printerID api.PrinterID) {
	if !h.printingReady(w, r) {
		return
	}
	pr, err := h.Printing.GetPrinter(r.Context(), principalOf(r), establishmentID, printerID)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, pr.Version, PrinterToAPI(pr))
}

// UpdatePrinter implements PUT /printers/{printerId}.
func (h *Handlers) UpdatePrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printerID api.PrinterID, params api.UpdatePrinterParams) {
	if !h.printingReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	var req api.PrinterInput
	if !decode(w, r, &req) {
		return
	}
	pr, err := h.Printing.UpdatePrinter(r.Context(), principalOf(r), establishmentID, printerID, expected, printerInputFromAPI(req))
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, pr.Version, PrinterToAPI(pr))
}

// DeletePrinter implements DELETE /printers/{printerId}.
func (h *Handlers) DeletePrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printerID api.PrinterID, params api.DeletePrinterParams) {
	if !h.printingReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	if err := h.Printing.DeletePrinter(r.Context(), principalOf(r), establishmentID, printerID, expected); err != nil {
		h.printingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CheckPrinter implements POST /printers/{printerId}/check.
func (h *Handlers) CheckPrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printerID api.PrinterID, _ api.CheckPrinterParams) {
	if !h.printingReady(w, r) {
		return
	}
	res, err := h.Printing.CheckPrinter(r.Context(), principalOf(r), establishmentID, printerID)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	out := api.PrinterCheckResult{
		Status: api.PrinterStatus(res.Status), CheckedAt: res.CheckedAt.UTC(),
		PaperLow: res.PaperLow, CoverOpen: res.CoverOpen, Error: strPtr(res.Error),
	}
	if res.Status == store.PrinterStatusReachable {
		ms := int(res.Latency.Milliseconds())
		out.LatencyMs = &ms
	}
	writeJSON(w, http.StatusOK, out)
}

// TestPrinter implements POST /printers/{printerId}/test-print.
func (h *Handlers) TestPrinter(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printerID api.PrinterID, params api.TestPrinterParams) {
	if !h.printingReady(w, r) {
		return
	}
	j, _, err := h.Printing.TestPrint(r.Context(), principalOf(r), establishmentID, printerID, deref(params.IdempotencyKey))
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.printJobToAPI(r, establishmentID, j))
}

// GetPrintRoutes implements GET /print-routes.
func (h *Handlers) GetPrintRoutes(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if !h.printingReady(w, r) {
		return
	}
	rs, err := h.Printing.GetRoutes(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, rs.Version, routeSetToAPI(rs))
}

// UpdatePrintRoutes implements PUT /print-routes.
func (h *Handlers) UpdatePrintRoutes(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.UpdatePrintRoutesParams) {
	if !h.printingReady(w, r) {
		return
	}
	expected, err := ParseIfMatch(params.IfMatch)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	var req api.PrintRouteSet
	if !decode(w, r, &req) {
		return
	}
	routes := make([]printing.Route, len(req.Items))
	for i, it := range req.Items {
		routes[i] = printing.Route{
			DocumentKind: string(it.DocumentKind), RegisterID: it.RegisterId, PrinterID: it.PrinterId,
			Copies: deref(it.Copies), KickDrawer: deref(it.KickDrawer),
		}
	}
	rs, err := h.Printing.UpdateRoutes(r.Context(), principalOf(r), establishmentID, expected, routes)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeVersioned(w, http.StatusOK, rs.Version, routeSetToAPI(rs))
}

// ResolvePrintRoute implements GET /print-routes/resolve.
func (h *Handlers) ResolvePrintRoute(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ResolvePrintRouteParams) {
	if !h.printingReady(w, r) {
		return
	}
	res, err := h.Printing.ResolveRoute(r.Context(), principalOf(r), establishmentID, string(params.DocumentKind), params.RegisterId)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	out := api.PrintRouteResolution{
		DocumentKind: api.PrintDocumentKind(res.DocumentKind), Ready: res.Ready, Copies: res.Copies, KickDrawer: res.KickDrawer,
	}
	if res.Reason != "" {
		reason := api.PrintRouteUnreadyReason(res.Reason)
		out.Reason = &reason
	}
	if res.Printer != nil {
		p := PrinterToAPI(res.Printer)
		out.Printer = &p
	}
	writeJSON(w, http.StatusOK, out)
}

// ListPrintJobs implements GET /print-jobs.
func (h *Handlers) ListPrintJobs(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListPrintJobsParams) {
	if !h.printingReady(w, r) {
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	f := store.PrintJobFilter{PrinterID: deref(params.PrinterId)}
	if params.Status != nil {
		f.Status = string(*params.Status)
	}
	page, err := h.Printing.ListJobs(r.Context(), principalOf(r), establishmentID, f, cursor, limit)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	out := api.PrintJobList{Items: make([]api.PrintJob, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		j := &page.Items[i]
		name := ""
		if j.PrinterID != nil {
			name = page.PrinterNames[*j.PrinterID]
		}
		out.Items[i] = PrintJobToAPI(j, name)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreatePrintJob implements POST /print-jobs: 202 for a new job, 200 when
// a job with the same idempotency key already exists.
func (h *Handlers) CreatePrintJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreatePrintJobParams) {
	if !h.printingReady(w, r) {
		return
	}
	var req api.PrintJobInput
	if !decode(w, r, &req) {
		return
	}
	in := printing.JobInput{
		DocumentKind: string(req.DocumentKind), DocumentID: deref(req.DocumentId), RegisterID: req.RegisterId,
		PrinterID: req.PrinterId, Copies: req.Copies, KickDrawer: req.KickDrawer,
	}
	j, created, err := h.Printing.Enqueue(r.Context(), principalOf(r), establishmentID, in, deref(params.IdempotencyKey))
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, h.printJobToAPI(r, establishmentID, j))
}

// GetPrintJob implements GET /print-jobs/{printJobId}.
func (h *Handlers) GetPrintJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printJobID api.PrintJobID) {
	if !h.printingReady(w, r) {
		return
	}
	j, err := h.Printing.GetJob(r.Context(), principalOf(r), establishmentID, printJobID)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h.printJobToAPI(r, establishmentID, j))
}

// RetryPrintJob implements POST /print-jobs/{printJobId}/retry.
func (h *Handlers) RetryPrintJob(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, printJobID api.PrintJobID, _ api.RetryPrintJobParams) {
	if !h.printingReady(w, r) {
		return
	}
	j, err := h.Printing.RetryJob(r.Context(), principalOf(r), establishmentID, printJobID)
	if err != nil {
		h.printingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.printJobToAPI(r, establishmentID, j))
}

func (h *Handlers) printJobToAPI(r *http.Request, establishmentID string, j *store.PrintJob) api.PrintJob {
	return PrintJobToAPI(j, h.Printing.JobPrinterName(r.Context(), principalOf(r), establishmentID, j))
}

// PrinterToAPI converts a printer (the logo raster stays server-side).
func PrinterToAPI(p *store.Printer) api.Printer {
	out := api.Printer{
		Id: p.ID, Name: p.Name, Driver: api.PrinterDriver(p.Driver), Host: p.Host, Port: p.Port,
		PaperWidthMm: api.PrinterPaperWidth(p.PaperWidthMM), CharsPerLine: p.CharsPerLine,
		Codepage: api.PrinterCodepage(p.Codepage), CodepageTable: p.CodepageTable, CutMode: api.PrinterCutMode(p.CutMode),
		FeedLines: p.FeedLines, PrintLogo: p.PrintLogo, LogoCached: len(p.LogoRaster) > 0, Active: p.Active,
		Status: api.PrinterStatus(p.Status), LastCheckedAt: utcPtr(p.LastCheckedAt), LastSuccessAt: utcPtr(p.LastSuccessAt),
		LastError: p.LastError, Version: p.Version, CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(),
	}
	if p.DrawerPin != nil {
		pin := api.PrinterDrawerPin(*p.DrawerPin)
		out.DrawerPin = &pin
	}
	return out
}

// PrintJobToAPI converts a print job (never the payload).
func PrintJobToAPI(j *store.PrintJob, printerName string) api.PrintJob {
	return api.PrintJob{
		Id: j.ID, PrinterId: j.PrinterID, PrinterName: strPtr(printerName), DocumentKind: api.PrintDocumentKind(j.DocumentKind),
		DocumentId: j.DocumentID, RegisterId: j.RegisterID, Status: api.PrintJobStatus(j.Status),
		Attempts: j.Attempts, MaxAttempts: j.MaxAttempts, NextAttemptAt: utcPtr(j.NextAttemptAt), LastError: j.LastError,
		Copies: j.Copies, CreatedAt: j.CreatedAt.UTC(), UpdatedAt: j.UpdatedAt.UTC(), PrintedAt: utcPtr(j.PrintedAt),
	}
}

func routeSetToAPI(rs printing.RouteSet) api.PrintRouteSet {
	out := api.PrintRouteSet{Items: make([]api.PrintRoute, len(rs.Routes))}
	for i, r := range rs.Routes {
		copies, kick := r.Copies, r.KickDrawer
		out.Items[i] = api.PrintRoute{
			DocumentKind: api.PrintDocumentKind(r.DocumentKind), RegisterId: r.RegisterID, PrinterId: r.PrinterID,
			Copies: &copies, KickDrawer: &kick,
		}
	}
	return out
}

func printerInputFromAPI(req api.PrinterInput) printing.PrinterInput {
	in := printing.PrinterInput{
		Name: req.Name, Host: req.Host, Port: deref(req.Port), PaperWidthMM: int(req.PaperWidthMm),
		CharsPerLine: deref(req.CharsPerLine), CodepageTable: req.CodepageTable, FeedLines: req.FeedLines,
		PrintLogo: deref(req.PrintLogo), Active: req.Active,
	}
	if req.Codepage != nil {
		in.Codepage = string(*req.Codepage)
	}
	if req.CutMode != nil {
		in.CutMode = string(*req.CutMode)
	}
	if req.DrawerPin != nil {
		pin := int(*req.DrawerPin)
		in.DrawerPin = &pin
	}
	return in
}

// deref returns *p or the zero value.
func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
