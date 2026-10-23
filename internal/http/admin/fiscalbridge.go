package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/fiscalbridge"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Fiscal bridge status for the panel (M6, spec 29.1). Owner: fiscal bridge
// builder.
//
//	GetFiscalBridgeStatus -> FiscalBridge.Status -> api.FiscalBridgeStatus

// GetFiscalBridgeStatus implements GET /api/admin/v1/establishments/{establishmentId}/fiscal-bridge.
func (h *Handlers) GetFiscalBridgeStatus(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID) {
	if h.FiscalBridge == nil {
		problem.Write(w, r, problem.NotImplemented)
		return
	}
	st, err := h.FiscalBridge.Status(r.Context(), principalOf(r), establishmentID)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, FiscalBridgeStatusToAPI(st))
}

// FiscalBridgeStatusToAPI maps the bridge state (also used by the device API).
func FiscalBridgeStatusToAPI(s fiscalbridge.Status) api.FiscalBridgeStatus {
	out := api.FiscalBridgeStatus{
		Registered: s.Registered, Online: s.Online, PendingJobs: s.PendingJobs, FailedJobs: s.FailedJobs,
		LastSeenAt: s.LastSeenAt,
	}
	if s.DeviceID != "" {
		id := s.DeviceID
		out.DeviceId = &id
	}
	if s.DeviceName != "" {
		n := s.DeviceName
		out.DeviceName = &n
	}
	if s.Driver != "" {
		d := s.Driver
		out.Driver = &d
	}
	if ds := s.DeviceStatus; ds != nil {
		st := api.FiscalDeviceStatus{Online: ds.Online}
		if ds.Message != "" {
			m := ds.Message
			st.Message = &m
		}
		if !ds.CheckedAt.IsZero() {
			c := ds.CheckedAt.UTC()
			st.CheckedAt = &c
		}
		out.DeviceStatus = &st
	}
	return out
}
