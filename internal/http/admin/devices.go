package admin

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/http/problem"
)

// Paired devices (M6/M7, spec 29.2). Owner: devices builder.
//
//	CreateDevicePairingCode -> Devices.CreatePairingCode -> api.DevicePairingCode (201)
//	ListDevices             -> Devices.List              -> api.DeviceList
//	RevokeDevice            -> Devices.Revoke            -> 204
//
// The pairing code, its QR text and QR image are returned only in the 201
// response (Cache-Control: no-store); never log them.

// CreateDevicePairingCode implements POST /api/admin/v1/establishments/{establishmentId}/devices/pairing-codes.
func (h *Handlers) CreateDevicePairingCode(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, _ api.CreateDevicePairingCodeParams) {
	if h.Devices == nil {
		notImplemented(w, r)
		return
	}
	var req api.DevicePairingCodeRequest
	if !decodeOptional(w, r, &req) {
		return
	}
	in := device.PairingCodeInput{UserID: estDeref(req.UserId)}
	if req.Kind != nil {
		in.Kind = string(*req.Kind)
	}
	if req.TtlSeconds != nil {
		in.TTL = time.Duration(*req.TtlSeconds) * time.Second
		if in.TTL == 0 {
			in.TTL = -1 // an explicit 0 is out of range, not "default"
		}
	}
	pc, err := h.Devices.CreatePairingCode(r.Context(), principalOf(r), establishmentID, in)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	middleware.NoReplay(w) // a secret shown once: never stored for Idempotency-Key replay
	writeJSON(w, http.StatusCreated, PairingCodeToAPI(pc))
}

// ListDevices implements GET /api/admin/v1/establishments/{establishmentId}/devices.
func (h *Handlers) ListDevices(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, params api.ListDevicesParams) {
	if h.Devices == nil {
		notImplemented(w, r)
		return
	}
	cursor, limit, err := pageParams(params.Cursor, params.Limit)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	f := device.ListFilter{Cursor: cursor, Limit: limit, IncludeRevoked: params.IncludeRevoked != nil && *params.IncludeRevoked}
	page, err := h.Devices.List(r.Context(), principalOf(r), establishmentID, f)
	if err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	out := api.DeviceList{Items: make([]api.Device, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for i := range page.Items {
		out.Items = append(out.Items, DeviceToAPI(&page.Items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// RevokeDevice implements DELETE /api/admin/v1/establishments/{establishmentId}/devices/{deviceId}.
func (h *Handlers) RevokeDevice(w http.ResponseWriter, r *http.Request, establishmentID api.EstablishmentID, deviceID api.DeviceID, _ api.RevokeDeviceParams) {
	if h.Devices == nil {
		notImplemented(w, r)
		return
	}
	if err := h.Devices.Revoke(r.Context(), principalOf(r), establishmentID, deviceID); err != nil {
		problem.Error(w, r, h.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PairingCodeToAPI maps a freshly created pairing code.
func PairingCodeToAPI(pc *device.PairingCode) api.DevicePairingCode {
	qr := api.DevicePairingQRPayload{V: api.DevicePairingQRPayloadV(pc.QR.V), Url: pc.QR.URL, Code: pc.QR.Code}
	if pc.QR.Fingerprint != "" {
		qr.Fingerprint = strPtr(pc.QR.Fingerprint)
	}
	if pc.QR.EstablishmentName != "" {
		qr.EstablishmentName = strPtr(pc.QR.EstablishmentName)
	}
	out := api.DevicePairingCode{
		Code: pc.Code, ExpiresAt: pc.ExpiresAt.UTC(), EstablishmentId: pc.EstablishmentID, UserId: pc.UserID,
		QrPayload: qr, QrText: pc.QRText,
	}
	if pc.QRSVG != "" {
		out.QrSvg = strPtr(pc.QRSVG)
	}
	return out
}

// DeviceToAPI maps a device view (shared with the device API handlers).
func DeviceToAPI(v *device.View) api.Device {
	out := api.Device{
		Id: v.ID, EstablishmentId: v.EstablishmentID, UserId: v.UserID, UserName: v.UserName,
		Role: api.Role(v.Role), Name: v.Name, Platform: api.DevicePlatform(v.Platform), Kind: api.DeviceKind(v.Kind),
		AppVersion: v.AppVersion, CreatedAt: v.CreatedAt.UTC(), LastUsedAt: utcPtr(v.LastUsedAt),
		RevokedAt: utcPtr(v.RevokedAt), Current: v.Current, FiscalBridge: v.FiscalBridge,
	}
	if v.TokenPrefix != "" {
		out.TokenPrefix = strPtr(device.DisplayPrefix(v.TokenPrefix))
	}
	return out
}

// decodeOptional is decode for an optional JSON body: an empty (or
// whitespace only) body leaves v at its zero value. The body is already
// bounded by middleware.LimitBody.
func decodeOptional(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		return true
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta troppo grande."))
		return false
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	return decode(w, r, v)
}
