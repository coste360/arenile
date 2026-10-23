package admin

import (
	"net/http"

	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Paired devices (M6/M7, spec 29.2). Owner: devices builder.
//
//	CreateDevicePairingCode -> Devices.CreatePairingCode -> api.DevicePairingCode (201)
//	ListDevices             -> Devices.List              -> api.DeviceList
//	RevokeDevice            -> Devices.Revoke            -> 204
//
// The pairing code and its QR text are returned only in the 201 response;
// never log them.

// CreateDevicePairingCode implements POST /api/admin/v1/establishments/{establishmentId}/devices/pairing-codes.
func (h *Handlers) CreateDevicePairingCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.CreateDevicePairingCodeParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// ListDevices implements GET /api/admin/v1/establishments/{establishmentId}/devices.
func (h *Handlers) ListDevices(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListDevicesParams) {
	problem.Write(w, r, problem.NotImplemented)
}

// RevokeDevice implements DELETE /api/admin/v1/establishments/{establishmentId}/devices/{deviceId}.
func (h *Handlers) RevokeDevice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, deviceID api.DeviceID, params api.RevokeDeviceParams) {
	problem.Write(w, r, problem.NotImplemented)
}
