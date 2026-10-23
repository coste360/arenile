package store

import (
	"time"

	"github.com/uptrace/bun"
)

// Models of the paired devices and of the fiscal bridge (migration 00006,
// spec 29.1, 29.2, 14.2). Models only: repository functions live in
// internal/store/device.go (owned by internal/domain/device) and
// internal/store/fiscalbridge.go (owned by internal/fiscalbridge).
//
// Tokens are never stored in clear: Device.TokenHash and
// DevicePairingCode.CodeHash hold the SHA-256 hex of the token / of the
// normalised pairing code (see device.HashToken and device.HashPairingCode).

// Device platforms (devices.platform, api.DevicePlatform).
const (
	DevicePlatformIOS     = "ios"
	DevicePlatformAndroid = "android"
	DevicePlatformWindows = "windows"
	DevicePlatformMacOS   = "macos"
	DevicePlatformLinux   = "linux"
	DevicePlatformOther   = "other"
)

// Device kinds (devices.kind, api.DeviceKind).
const (
	DeviceKindMobile  = "mobile"
	DeviceKindDesktop = "desktop"
)

// Device is a paired device (mobile app or desktop app in remote mode)
// bound to one user and one establishment. A device authenticates with
// Authorization: Bearer ar_dev_<prefix>_<secret>; RevokedAt non-nil means
// the token no longer authenticates.
type Device struct {
	bun.BaseModel `bun:"table:devices,alias:dev"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	UserID          string     `bun:"user_id,notnull"`
	Name            string     `bun:"name,notnull"`
	Platform        string     `bun:"platform,notnull"`
	Kind            string     `bun:"kind,notnull"`
	AppVersion      *string    `bun:"app_version"`
	TokenPrefix     string     `bun:"token_prefix,notnull"`
	TokenHash       string     `bun:"token_hash,notnull"`
	PairingCodeID   *string    `bun:"pairing_code_id"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	LastUsedAt      *time.Time `bun:"last_used_at"`
	RevokedAt       *time.Time `bun:"revoked_at"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// DevicePairingCode is a short-lived, single-use pairing code. It is
// consumed with a conditional UPDATE (used_at IS NULL AND expires_at > now)
// that also sets DeviceID.
type DevicePairingCode struct {
	bun.BaseModel `bun:"table:device_pairing_codes,alias:dpc"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	UserID          string     `bun:"user_id,notnull"`
	CreatedByUserID *string    `bun:"created_by_user_id"`
	Kind            *string    `bun:"kind"`
	CodeHash        string     `bun:"code_hash,notnull"`
	ExpiresAt       time.Time  `bun:"expires_at,notnull"`
	UsedAt          *time.Time `bun:"used_at"`
	DeviceID        *string    `bun:"device_id"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
}

// FiscalBridge registers the device that executes fiscal jobs on its local
// FiscalDriver. At most one active row per establishment (partial unique
// index). DeviceStatus is the last fiscalbridge.DeviceStatus reported.
type FiscalBridge struct {
	bun.BaseModel `bun:"table:fiscal_bridges,alias:fb"`

	ID              string     `bun:"id,pk"`
	EstablishmentID string     `bun:"establishment_id,notnull"`
	DeviceID        string     `bun:"device_id,notnull"`
	Driver          string     `bun:"driver,notnull"`
	Active          bool       `bun:"active,notnull"`
	DeviceStatus    JSON       `bun:"device_status,notnull"`
	LastSeenAt      *time.Time `bun:"last_seen_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

// Fiscal bridge job kinds and statuses (fiscal_bridge_jobs.kind/status).
const (
	FiscalBridgeJobIssue = "issue"
	FiscalBridgeJobVoid  = "void"

	FiscalBridgeJobQueued    = "queued"
	FiscalBridgeJobLeased    = "leased"
	FiscalBridgeJobSucceeded = "succeeded"
	FiscalBridgeJobFailed    = "failed"
)

// FiscalBridgeJob is a fiscal operation dispatched to the bridge device
// (at-least-once with leases). (EstablishmentID, Kind, IdempotencyKey) is
// unique, so the bridge driver enqueues idempotently. Payload is the
// fiscalbridge.Document (issue) or {"ref": ...} (void); Result the
// fiscalbridge.Result received from the device.
type FiscalBridgeJob struct {
	bun.BaseModel `bun:"table:fiscal_bridge_jobs,alias:fbj"`

	ID               string     `bun:"id,pk"`
	EstablishmentID  string     `bun:"establishment_id,notnull"`
	FiscalDocumentID *string    `bun:"fiscal_document_id"`
	Kind             string     `bun:"kind,notnull"`
	IdempotencyKey   string     `bun:"idempotency_key,notnull"`
	Status           string     `bun:"status,notnull"`
	Payload          JSON       `bun:"payload,notnull"`
	Result           JSON       `bun:"result"`
	Attempts         int        `bun:"attempts,notnull"`
	LeasedDeviceID   *string    `bun:"leased_device_id"`
	LeaseExpiresAt   *time.Time `bun:"lease_expires_at"`
	NextAttemptAt    time.Time  `bun:"next_attempt_at,notnull"`
	CompletedAt      *time.Time `bun:"completed_at"`
	CreatedAt        time.Time  `bun:"created_at,notnull"`
	UpdatedAt        time.Time  `bun:"updated_at,notnull"`
}
