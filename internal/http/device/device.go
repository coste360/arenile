// Package device implements the thin HTTP handlers of the device API
// (/api/device/v1, spec 29.2): pairing (unauthenticated, rate limited) and
// the operations of a paired device authenticated with its device token
// (Authorization: Bearer ar_dev_..., no cookies, no CSRF), including the
// fiscal bridge long-poll of the desktop app in remote mode (spec 29.1).
//
// Routing and middlewares are set in internal/http/server.go: the
// /api/device/ prefix authenticates with device.DeviceOnly (device tokens
// only), POST /api/device/v1/pair is the only public path and has its own
// per-IP limit, and Idempotency-Key is not honoured on this prefix (the
// pairing response carries the token, fiscal results are idempotent by job
// id).
package device

import (
	"log/slog"
	"net/http"

	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Handlers implements the device operations of api.ServerInterface.
//
//	PairDevice                  -> Devices.Pair            -> api.DevicePairResult (201)   owner: devices builder
//	GetCurrentDevice            -> Devices.Current         -> api.Device                   owner: devices builder
//	UnpairCurrentDevice         -> Devices.UnpairCurrent   -> 204                          owner: devices builder
//	RegisterFiscalBridge        -> FiscalBridge.Register   -> api.FiscalBridgeStatus       owner: fiscal bridge builder
//	UnregisterFiscalBridge      -> FiscalBridge.Unregister -> 204                          owner: fiscal bridge builder
//	PollFiscalBridgeJobs        -> FiscalBridge.Poll       -> api.FiscalBridgeJobBatch     owner: fiscal bridge builder
//	SubmitFiscalBridgeJobResult -> FiscalBridge.Submit     -> 204                          owner: fiscal bridge builder
type Handlers struct {
	Devices      *device.Service
	FiscalBridge *fiscalbridge.Service
	Log          *slog.Logger
}

// PairDevice implements POST /api/device/v1/pair.
func (h *Handlers) PairDevice(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// GetCurrentDevice implements GET /api/device/v1/self.
func (h *Handlers) GetCurrentDevice(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// UnpairCurrentDevice implements DELETE /api/device/v1/self.
func (h *Handlers) UnpairCurrentDevice(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// RegisterFiscalBridge implements PUT /api/device/v1/fiscal-bridge.
func (h *Handlers) RegisterFiscalBridge(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// UnregisterFiscalBridge implements DELETE /api/device/v1/fiscal-bridge.
func (h *Handlers) UnregisterFiscalBridge(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// PollFiscalBridgeJobs implements GET /api/device/v1/fiscal-bridge/jobs.
func (h *Handlers) PollFiscalBridgeJobs(w http.ResponseWriter, r *http.Request, params api.PollFiscalBridgeJobsParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// SubmitFiscalBridgeJobResult implements POST /api/device/v1/fiscal-bridge/jobs/{jobId}/result.
func (h *Handlers) SubmitFiscalBridgeJobResult(w http.ResponseWriter, r *http.Request, jobID api.FiscalBridgeJobID) {
	problem.Write(w, r, problem.NotImplemented)
}
