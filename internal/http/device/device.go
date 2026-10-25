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

	"github.com/coste360/arenile/internal/domain/device"
	"github.com/coste360/arenile/internal/domain/push"
	"github.com/coste360/arenile/internal/fiscalbridge"
)

// Handlers implements the device operations of api.ServerInterface.
//
//	PairDevice                  -> Devices.Pair            -> api.DevicePairResult (201)   owner: devices builder (pair.go)
//	GetCurrentDevice            -> Devices.Current         -> api.Device                   owner: devices builder
//	UnpairCurrentDevice         -> Devices.UnpairCurrent   -> 204                          owner: devices builder
//	RegisterFiscalBridge        -> FiscalBridge.Register   -> api.FiscalBridgeStatus       owner: fiscal bridge builder (fiscalbridge.go)
//	UnregisterFiscalBridge      -> FiscalBridge.Unregister -> 204                          owner: fiscal bridge builder
//	PollFiscalBridgeJobs        -> FiscalBridge.Poll       -> api.FiscalBridgeJobBatch     owner: fiscal bridge builder
//	SubmitFiscalBridgeJobResult -> FiscalBridge.Submit     -> 204                          owner: fiscal bridge builder
//	GetDevicePushRegistration   -> Push.Status             -> api.PushRegistrationStatus   owner: push (push.go)
//	RegisterDevicePush          -> Push.Register           -> api.PushRegistrationStatus   owner: push
//	UnregisterDevicePush        -> Push.Unregister         -> 204                          owner: push
type Handlers struct {
	Devices      *device.Service
	FiscalBridge *fiscalbridge.Service
	Push         *push.Service
	Log          *slog.Logger
}
