package device

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Fiscal bridge operations of the desktop app in remote mode (owner:
// fiscal bridge builder, spec 29.1 "Ponte verso le periferiche locali").
// The device token authenticates every call (no cookies, no CSRF); the
// service checks that the device user may operate the cash desk.

// maxBridgeBody bounds the JSON bodies of the bridge (raw driver response
// up to 16 KiB plus the envelope).
const maxBridgeBody = 64 << 10

// RegisterFiscalBridge implements PUT /api/device/v1/fiscal-bridge.
func (h *Handlers) RegisterFiscalBridge(w http.ResponseWriter, r *http.Request) {
	if h.FiscalBridge == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.FiscalBridgeRegistration
	if !decodeBridgeJSON(w, r, &req) {
		return
	}
	reg := fiscalbridge.Registration{Driver: req.Driver}
	if req.DeviceStatus != nil {
		ds := DeviceStatusFromAPI(*req.DeviceStatus)
		reg.DeviceStatus = &ds
	}
	p, _ := auth.PrincipalFrom(r.Context())
	st, err := h.FiscalBridge.Register(r.Context(), p, reg)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDeviceJSON(w, http.StatusOK, admin.FiscalBridgeStatusToAPI(st))
}

// UnregisterFiscalBridge implements DELETE /api/device/v1/fiscal-bridge.
func (h *Handlers) UnregisterFiscalBridge(w http.ResponseWriter, r *http.Request) {
	if h.FiscalBridge == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.FiscalBridge.Unregister(r.Context(), p); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PollFiscalBridgeJobs implements GET /api/device/v1/fiscal-bridge/jobs
// (long-poll: wait 0..25 s, max 1..50 jobs).
func (h *Handlers) PollFiscalBridgeJobs(w http.ResponseWriter, r *http.Request, params api.PollFiscalBridgeJobsParams) {
	if h.FiscalBridge == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	wait, maxJobs := int(fiscalbridge.MaxWait/time.Second), fiscalbridge.DefaultMaxJobs
	if params.Wait != nil {
		wait = *params.Wait
	}
	if params.Max != nil {
		maxJobs = *params.Max
	}
	if wait < 0 || wait > int(fiscalbridge.MaxWait/time.Second) {
		problem.Write(w, r, problem.ValidationFailed, problem.Fields(api.FieldError{Field: "wait", Message: "deve essere tra 0 e 25"}))
		return
	}
	if maxJobs < 1 || maxJobs > fiscalbridge.MaxJobsLimit {
		problem.Write(w, r, problem.ValidationFailed, problem.Fields(api.FieldError{Field: "max", Message: "deve essere tra 1 e 50"}))
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	jobs, err := h.FiscalBridge.Poll(r.Context(), p, time.Duration(wait)*time.Second, maxJobs)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.FiscalBridgeJobBatch{Jobs: make([]api.FiscalBridgeJob, 0, len(jobs))}
	for i := range jobs {
		out.Jobs = append(out.Jobs, JobToAPI(jobs[i]))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeDeviceJSON(w, http.StatusOK, out)
}

// SubmitFiscalBridgeJobResult implements POST /api/device/v1/fiscal-bridge/jobs/{jobId}/result.
func (h *Handlers) SubmitFiscalBridgeJobResult(w http.ResponseWriter, r *http.Request, jobID api.FiscalBridgeJobID) {
	if h.FiscalBridge == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.FiscalBridgeJobResult
	if !decodeBridgeJSON(w, r, &req) {
		return
	}
	res, err := ResultFromAPI(req)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.FiscalBridge.Submit(r.Context(), p, jobID, res); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeBridgeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBridgeBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
			return false
		}
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return false
	}
	return true
}

// ---- mapping (shared with the agent tests) -------------------------------------

// DeviceStatusFromAPI maps api.FiscalDeviceStatus.
func DeviceStatusFromAPI(s api.FiscalDeviceStatus) fiscalbridge.DeviceStatus {
	ds := fiscalbridge.DeviceStatus{Online: s.Online}
	if s.Message != nil {
		ds.Message = *s.Message
	}
	if s.CheckedAt != nil {
		ds.CheckedAt = s.CheckedAt.UTC()
	}
	return ds
}

// JobToAPI maps a leased job.
func JobToAPI(j fiscalbridge.Job) api.FiscalBridgeJob {
	out := api.FiscalBridgeJob{Id: j.ID, Kind: api.FiscalBridgeJobKind(j.Kind), Attempt: j.Attempt, LeaseExpiresAt: j.LeaseExpiresAt}
	if j.Ref != "" {
		ref := j.Ref
		out.Ref = &ref
	}
	if d := j.Document; d != nil {
		doc := api.FiscalBridgeDocument{
			EstablishmentId: d.EstablishmentID, Currency: d.Currency, Total: d.Total, IssuedAt: d.IssuedAt.UTC(),
			IdempotencyKey: d.IdempotencyKey,
			Lines:          make([]api.FiscalBridgeLine, len(d.Lines)), Payments: make([]api.FiscalBridgePayment, len(d.Payments)),
		}
		if d.BookingID != "" {
			b := d.BookingID
			doc.BookingId = &b
		}
		if d.PaymentID != "" {
			pid := d.PaymentID
			doc.PaymentId = &pid
		}
		for i, l := range d.Lines {
			doc.Lines[i] = api.FiscalBridgeLine{Description: l.Description, Quantity: l.Quantity, UnitGross: l.UnitGross, Gross: l.Gross, VatRateBp: l.VATRateBP}
		}
		for i, p := range d.Payments {
			doc.Payments[i] = api.FiscalBridgePayment{Method: p.Method, Amount: p.Amount}
		}
		out.Document = &doc
	}
	return out
}

// ResultFromAPI maps a submitted result.
func ResultFromAPI(req api.FiscalBridgeJobResult) (fiscalbridge.Result, error) {
	res := fiscalbridge.Result{Outcome: fiscalbridge.Outcome(req.Outcome), IssuedAt: req.IssuedAt}
	if req.Ref != nil {
		res.Ref = *req.Ref
	}
	if req.Number != nil {
		res.Number = *req.Number
	}
	if req.Error != nil {
		res.Error = *req.Error
	}
	if req.Retryable != nil {
		res.Retryable = *req.Retryable
	}
	if req.Raw != nil {
		raw, err := json.Marshal(*req.Raw)
		if err != nil {
			return res, err
		}
		res.Raw = raw
	}
	return res, nil
}
