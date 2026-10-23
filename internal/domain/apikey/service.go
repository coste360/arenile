package apikey

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
)

// Errors of the apikey package (mapped through the configerr kinds).
var (
	// ErrRevoked: the key is revoked and cannot be changed (409).
	ErrRevoked = fmt.Errorf("%w: chiave API revocata", configerr.ErrConflict)
)

// Status is the derived state of a key (API enum ApiKeyStatus).
type Status string

// Key states.
const (
	StatusActive  Status = "active"
	StatusExpired Status = "expired"
	StatusRevoked Status = "revoked"
)

// Key is the management view of an API key (never the secret or its hash).
type Key struct {
	ID              string
	EstablishmentID string
	Name            string
	// DisplayPrefix is "ar_live_<prefix>".
	DisplayPrefix   string
	Scopes          []auth.Permission
	Status          Status
	CreatedByUserID string
	CreatedByName   string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
	ExpiresAt       *time.Time
	RevokedAt       *time.Time
}

// CreateInput describes a new key.
type CreateInput struct {
	Name      string
	Scopes    []auth.Permission
	ExpiresAt *time.Time
	IP        string
}

// UpdateInput changes a key; nil fields are unchanged. ClearExpiry removes
// the expiry and is incompatible with ExpiresAt.
type UpdateInput struct {
	Name        *string
	Scopes      []auth.Permission // nil = unchanged
	ExpiresAt   *time.Time
	ClearExpiry bool
	IP          string
}

// Deps are the dependencies of the service. Every field is required except
// Log.
type Deps struct {
	DB    bun.IDB
	Clock clock.Clock
	Audit *audit.Writer
	Log   *slog.Logger
}

// Service implements the API key management use cases.
//
// Handler mapping (internal/http/admin/apikeys.go):
//
//	ListAPIKeys  -> List   -> api.ApiKeyList
//	CreateAPIKey -> Create -> 201 api.ApiKeyCreated (Cache-Control: no-store)
//	GetAPIKey    -> Get    -> api.ApiKey
//	UpdateAPIKey -> Update -> api.ApiKey
//	RevokeAPIKey -> Revoke -> 204
type Service struct {
	d Deps
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

// List returns the keys of the establishment, newest first; revoked keys
// only when includeRevoked.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, includeRevoked bool) ([]Key, error) {
	return nil, configerr.ErrNotImplemented
}

// Get returns one key.
func (s *Service) Get(ctx context.Context, p *auth.Principal, establishmentID, id string) (*Key, error) {
	return nil, configerr.ErrNotImplemented
}

// Create creates a key and returns it with the plaintext secret (shown once).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*Key, string, error) {
	return nil, "", configerr.ErrNotImplemented
}

// Update changes name, scopes or expiry of a non-revoked key.
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, id string, in UpdateInput) (*Key, error) {
	return nil, configerr.ErrNotImplemented
}

// Revoke revokes a key (idempotent).
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, establishmentID, id, ip string) error {
	return configerr.ErrNotImplemented
}
