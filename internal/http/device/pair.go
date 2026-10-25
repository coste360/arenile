package device

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/http/admin"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// Pairing and the paired device itself (owner: devices builder). The
// token is returned once in the 201 of PairDevice with Cache-Control:
// no-store; it is never logged.

// PairDevice implements POST /api/device/v1/pair.
func (h *Handlers) PairDevice(w http.ResponseWriter, r *http.Request) {
	if h.Devices == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	var req api.DevicePairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
			return
		}
		problem.Write(w, r, problem.BadRequest, problem.Detail("JSON non valido o campi con formato errato."))
		return
	}
	in := device.PairInput{
		Code: req.Code, Name: req.Name, Platform: string(req.Platform), Kind: string(req.Kind),
		IP: reqctx.From(r.Context()).ClientIP,
	}
	if req.AppVersion != nil {
		in.AppVersion = *req.AppVersion
	}
	res, err := h.Devices.Pair(r.Context(), in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeDeviceJSON(w, http.StatusCreated, api.DevicePairResult{
		Token: res.Token, Device: admin.DeviceToAPI(&res.Device), EstablishmentId: res.Device.EstablishmentID,
	})
}

// CreateDemoPairingCode implements POST /api/device/v1/demo-pairing (404
// unless the instance is a demo, limited per IP by the server).
func (h *Handlers) CreateDemoPairingCode(w http.ResponseWriter, r *http.Request) {
	if h.Devices == nil {
		problem.Write(w, r, problem.NotFound)
		return
	}
	pc, err := h.Devices.CreateDemoPairingCode(r.Context(), reqctx.From(r.Context()).ClientIP)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeDeviceJSON(w, http.StatusCreated, admin.PairingCodeToAPI(pc))
}

// GetCurrentDevice implements GET /api/device/v1/self.
func (h *Handlers) GetCurrentDevice(w http.ResponseWriter, r *http.Request) {
	if h.Devices == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	v, err := h.Devices.Current(r.Context(), p)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	writeDeviceJSON(w, http.StatusOK, admin.DeviceToAPI(v))
}

// UnpairCurrentDevice implements DELETE /api/device/v1/self.
func (h *Handlers) UnpairCurrentDevice(w http.ResponseWriter, r *http.Request) {
	if h.Devices == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	if err := h.Devices.UnpairCurrent(r.Context(), p); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeDeviceJSON writes v as JSON with status.
func writeDeviceJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
