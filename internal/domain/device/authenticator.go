package device

import (
	"context"

	"github.com/coste360/arenile/internal/domain/auth"
)

// SessionsAndKeys is the part of auth.Service used by BearerAuthenticator.
type SessionsAndKeys interface {
	AuthenticateSession(ctx context.Context, token string) (*auth.Principal, error)
	AuthenticateAPIKey(ctx context.Context, key string) (*auth.Principal, error)
}

// TokenAuthenticator resolves device tokens (Service).
type TokenAuthenticator interface {
	AuthenticateDevice(ctx context.Context, token string) (*auth.Principal, error)
}

// BearerAuthenticator implements middleware.Authenticator for /api/admin:
// the Authorization: Bearer credential is dispatched on its scheme
// (strategy): device tokens (ar_dev_) to Devices, everything else to the
// API keys of auth.Service. Session cookies are unchanged.
type BearerAuthenticator struct {
	Auth    SessionsAndKeys
	Devices TokenAuthenticator
}

// AuthenticateSession delegates to auth.Service.
func (b BearerAuthenticator) AuthenticateSession(ctx context.Context, token string) (*auth.Principal, error) {
	return b.Auth.AuthenticateSession(ctx, token)
}

// AuthenticateAPIKey dispatches a bearer credential on its scheme.
func (b BearerAuthenticator) AuthenticateAPIKey(ctx context.Context, key string) (*auth.Principal, error) {
	if IsToken(key) {
		if b.Devices == nil {
			return nil, auth.ErrUnauthenticated
		}
		return b.Devices.AuthenticateDevice(ctx, key)
	}
	return b.Auth.AuthenticateAPIKey(ctx, key)
}

// DeviceOnly implements middleware.Authenticator for /api/device: only
// device tokens authenticate (no cookies, no API keys).
type DeviceOnly struct{ Devices TokenAuthenticator }

// AuthenticateSession always fails: cookies are not accepted on /api/device.
func (DeviceOnly) AuthenticateSession(context.Context, string) (*auth.Principal, error) {
	return nil, auth.ErrUnauthenticated
}

// AuthenticateAPIKey accepts device tokens only.
func (d DeviceOnly) AuthenticateAPIKey(ctx context.Context, key string) (*auth.Principal, error) {
	if !IsToken(key) || d.Devices == nil {
		return nil, auth.ErrUnauthenticated
	}
	return d.Devices.AuthenticateDevice(ctx, key)
}
