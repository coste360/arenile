package device

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// Domain errors (mapped by internal/http/problem).
var (
	// ErrPairingCodeInvalid: unknown, expired or already used code
	// (400 pairing_code_invalid, deliberately indistinguishable).
	ErrPairingCodeInvalid = errors.New("device: pairing code invalid")
	// ErrLANModeRequired: desktop edition in local mode, no address other
	// devices can reach (409 lan_mode_required).
	ErrLANModeRequired = errors.New("device: lan mode required")
	// ErrDemoPairingDisabled: the instance has no demo pairing (404).
	ErrDemoPairingDisabled = errors.New("device: demo pairing disabled")
	// ErrNotImplemented: scaffolding, the operation is not built yet (501).
	ErrNotImplemented = configerr.ErrNotImplemented
)

// Defaults of pairing codes and token bookkeeping.
const (
	DefaultPairingTTL = 5 * time.Minute
	MinPairingTTL     = time.Minute
	MaxPairingTTL     = 15 * time.Minute
	// TouchInterval bounds the writes of devices.last_used_at.
	TouchInterval = time.Minute
)

// Principals builds the principal of a device token (auth.Service).
type Principals interface {
	PrincipalForDevice(ctx context.Context, d auth.DeviceIdentity) (*auth.Principal, error)
}

// PairingTarget is the address put in the QR. Fingerprint is the SHA-256
// of the self-signed certificate (desktop LAN mode), empty otherwise.
type PairingTarget struct {
	URL         string
	Fingerprint string
}

// Instance tells where other devices reach this instance. The server
// edition answers with ARENILE_BASE_URL; the desktop edition with the LAN
// URL and certificate fingerprint, or ErrLANModeRequired in local mode.
type Instance interface {
	PairingTarget(ctx context.Context) (PairingTarget, error)
}

// BridgeRevoker deactivates the fiscal bridge registration of a device in
// the revocation transaction (fiscalbridge.Service).
type BridgeRevoker interface {
	DeactivateDevice(ctx context.Context, tx bun.Tx, es store.EstablishmentScope, deviceID string) error
}

// Deps are the dependencies of Service.
type Deps struct {
	DB         *store.DB
	Clock      clock.Clock
	Audit      *audit.Writer
	Log        *slog.Logger
	Principals Principals
	Instance   Instance
	// Bridge is optional (nil: no fiscal bridge wired).
	Bridge BridgeRevoker
	// Demo enables CreateDemoPairingCode (nil: disabled, the usual case).
	Demo *DemoPairing
}

// DemoPairing names who a demo pairing code is for (ARENILE_DEMO_PAIRING_USER
// and ARENILE_DEMO_PAIRING_ESTABLISHMENT): an active member of that
// establishment, whose role the paired devices get.
type DemoPairing struct {
	UserEmail         string
	EstablishmentSlug string
}

// Service implements the device use cases. Construct with NewService.
type Service struct{ d Deps }

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// PairingCodeInput is the request of CreatePairingCode. Zero values mean
// defaults: UserID = the caller, TTL = DefaultPairingTTL.
type PairingCodeInput struct {
	UserID string
	Kind   string // store.DeviceKind*, optional hint
	TTL    time.Duration
}

// PairingCode is a freshly created code (returned once, stored as hash).
type PairingCode struct {
	Code            string
	ExpiresAt       time.Time
	EstablishmentID string
	UserID          string
	QR              QRPayload
	QRText          string
	// QRSVG is QRText rendered as SVG (empty if rendering failed: the
	// panel then shows the code only).
	QRSVG string
}

// PairInput is the request of Pair (unauthenticated).
type PairInput struct {
	Code       string
	Name       string
	Platform   string // store.DevicePlatform*
	Kind       string // store.DeviceKind*
	AppVersion string
	IP         string
}

// PairResult is the outcome of Pair: Token is returned once.
type PairResult struct {
	Token  string
	Device View
}

// View is a device as shown by the API (api.Device).
type View struct {
	store.Device
	UserName     string
	Role         auth.Role
	Current      bool
	FiscalBridge bool
}

// ListFilter filters List.
type ListFilter struct {
	IncludeRevoked bool
	Cursor         string
	Limit          int
}
