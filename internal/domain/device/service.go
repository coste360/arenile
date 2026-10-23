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
	"github.com/coste360/arenile/internal/jobs"
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

// CreatePairingCode creates a single-use code for in.UserID (default the
// caller) on establishmentID. Audit "device.pairing_code_created".
func (s *Service) CreatePairingCode(ctx context.Context, p *auth.Principal, establishmentID string, in PairingCodeInput) (*PairingCode, error) {
	return nil, ErrNotImplemented
}

// Pair exchanges a code for a device token (conditional UPDATE on the
// code, insert of the device, audit "device.paired" in one transaction).
func (s *Service) Pair(ctx context.Context, in PairInput) (*PairResult, error) {
	return nil, ErrNotImplemented
}

// List returns the devices of establishmentID visible to p.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f ListFilter) (store.Page[View], error) {
	return store.Page[View]{}, ErrNotImplemented
}

// Revoke revokes deviceID (idempotent) and deactivates its fiscal bridge
// registration in the same transaction. Audit "device.revoked".
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, establishmentID, deviceID string) error {
	return ErrNotImplemented
}

// Current returns the device of a device principal (p.DeviceID).
func (s *Service) Current(ctx context.Context, p *auth.Principal) (*View, error) {
	return nil, ErrNotImplemented
}

// UnpairCurrent revokes the device of a device principal.
func (s *Service) UnpairCurrent(ctx context.Context, p *auth.Principal) error {
	return ErrNotImplemented
}

// AuthenticateDevice resolves a device token to its principal: shape check
// (ParseToken), lookup by token_hash, revoked_at IS NULL, then
// Principals.PrincipalForDevice; touches last_used_at at most every
// TouchInterval. Any failure is auth.ErrUnauthenticated.
func (s *Service) AuthenticateDevice(ctx context.Context, token string) (*auth.Principal, error) {
	if _, ok := ParseToken(token); !ok {
		return nil, auth.ErrUnauthenticated
	}
	// Scaffolding: no device can authenticate until the builder implements
	// the lookup (fails closed).
	return nil, auth.ErrUnauthenticated
}

// CleanupExpiredCodes deletes pairing codes expired for more than a day.
func (s *Service) CleanupExpiredCodes(ctx context.Context) error {
	return nil
}

// Jobs are the background jobs of the device service.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "device.cleanup_pairing_codes", Interval: time.Hour, Run: s.CleanupExpiredCodes}}
}
