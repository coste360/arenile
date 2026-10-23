package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/store"
)

// Domain errors of the auth package.
var (
	ErrInvalidCredentials  = errors.New("auth: invalid credentials")
	ErrTOTPRequired        = errors.New("auth: totp code required")
	ErrInvalidTOTP         = errors.New("auth: invalid totp code")
	ErrTOTPAlreadyEnabled  = errors.New("auth: totp already enabled")
	ErrTOTPNotEnrolled     = errors.New("auth: totp enrolment not started")
	ErrTOTPEnrolmentNeeded = errors.New("auth: totp enrolment required")
	ErrUnauthenticated     = errors.New("auth: unauthenticated")
	ErrForbidden           = errors.New("auth: forbidden")
	ErrRateLimited         = errors.New("auth: too many attempts")
	ErrWeakPassword        = errors.New("auth: password does not meet policy")
	ErrUserExists          = errors.New("auth: user already exists")
	ErrUsersExist          = errors.New("auth: users already exist, bootstrap skipped")
	ErrInvalidInput        = errors.New("auth: invalid input")
	ErrNoSecretKey         = errors.New("auth: secret key not configured")
)

// RateLimitError carries the wait time of a rate-limited operation.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("auth: too many attempts, retry after %s", e.RetryAfter.Round(time.Second))
}

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// PrincipalKind distinguishes users from API keys.
type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"
	PrincipalAPIKey PrincipalKind = "api_key"
)

// Grant is a role granted on an organization (EstablishmentID empty = all of
// its establishments) or on one establishment.
type Grant struct {
	OrganizationID  string
	EstablishmentID string
	Role            Role
}

// Principal is the authenticated caller. It is built only by Service.
type Principal struct {
	Kind PrincipalKind

	// User principals.
	UserID      string
	Email       string
	Name        string
	Locale      string
	TOTPEnabled bool
	LastLoginAt *time.Time
	SessionID   string
	Grants      []Grant
	// TOTPEnrolmentRequired is true when the installation requires TOTP for
	// owners and this owner has not enrolled yet: only auth operations are allowed.
	TOTPEnrolmentRequired bool
	// Device principals (M6/M7, spec 29.2) are user principals authenticated
	// with a device token: DeviceID is the paired device and Grants are
	// narrowed to DeviceEstablishmentID (see Service.PrincipalForDevice).
	DeviceID              string
	DeviceName            string
	DeviceEstablishmentID string

	// API key principals.
	APIKeyID              string
	APIKeyName            string
	APIKeyPrefix          string
	APIKeyEstablishmentID string
	APIKeyOrganizationID  string
	Scopes                []Permission
}

// ID returns a stable identifier of the principal ("user:<id>" / "api_key:<id>").
func (p *Principal) ID() string {
	if p.Kind == PrincipalAPIKey {
		return "api_key:" + p.APIKeyID
	}
	return "user:" + p.UserID
}

// Scope returns the establishments the principal may access.
func (p *Principal) Scope() store.Scope {
	if p == nil {
		return store.Scope{}
	}
	if p.Kind == PrincipalAPIKey {
		return store.NewScope(nil, []string{p.APIKeyEstablishmentID})
	}
	var orgs, ests []string
	for _, g := range p.Grants {
		if g.EstablishmentID == "" {
			orgs = append(orgs, g.OrganizationID)
		} else {
			ests = append(ests, g.EstablishmentID)
		}
	}
	return store.NewScope(orgs, ests)
}

// Can reports whether the principal holds perm on the establishment proven
// by es. Permissions of all applicable grants are combined.
func (p *Principal) Can(es store.EstablishmentScope, perm Permission) bool {
	if p == nil || !es.Valid() || p.TOTPEnrolmentRequired {
		return false
	}
	if p.Kind == PrincipalAPIKey {
		return p.APIKeyEstablishmentID == es.ID() && slices.Contains(p.Scopes, perm)
	}
	for _, g := range p.Grants {
		applies := g.OrganizationID == es.OrganizationID() && (g.EstablishmentID == "" || g.EstablishmentID == es.ID())
		if applies && Can(g.Role, perm) {
			return true
		}
	}
	return false
}

// Authorize resolves establishmentID within the principal's scope and checks
// perm. It returns store.ErrNotFound for unknown or foreign establishments
// (no existence leak) and ErrForbidden when the role lacks the permission.
// Domain services call it before any operational query.
func (p *Principal) Authorize(ctx context.Context, db bun.IDB, establishmentID string, perm Permission) (store.EstablishmentScope, error) {
	if p == nil {
		return store.EstablishmentScope{}, ErrUnauthenticated
	}
	if p.TOTPEnrolmentRequired {
		return store.EstablishmentScope{}, ErrTOTPEnrolmentNeeded
	}
	es, err := store.ResolveEstablishment(ctx, db, p.Scope(), establishmentID)
	if err != nil {
		return store.EstablishmentScope{}, err
	}
	if !p.Can(es, perm) {
		return store.EstablishmentScope{}, ErrForbidden
	}
	return es, nil
}

// HasRole reports whether any grant has role.
func (p *Principal) HasRole(role Role) bool {
	for _, g := range p.Grants {
		if g.Role == role {
			return true
		}
	}
	return false
}

// Actor returns the audit actor of the principal.
func (p *Principal) Actor(ip string) audit.Actor {
	if p == nil {
		return audit.Actor{Type: audit.ActorSystem, IP: ip}
	}
	if p.Kind == PrincipalAPIKey {
		return audit.Actor{Type: audit.ActorAPIKey, ID: p.APIKeyID, IP: ip}
	}
	return audit.Actor{Type: audit.ActorUser, ID: p.UserID, IP: ip}
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal stored in ctx, if any.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok && p != nil
}
