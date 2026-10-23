package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/store"
)

// Listing limits.
const (
	DefaultListLimit = 50
	MaxListLimit     = 200
	// codeRetention keeps expired codes for a day (audit trail of pairings).
	codeRetention = 24 * time.Hour
)

// access resolves establishmentID for a device operation and tells whether
// p manages every device (auth.PermUsersManage) or only its own. API keys
// have no devices of their own: they need users.manage.
func (s *Service) access(ctx context.Context, p *auth.Principal, establishmentID string) (store.EstablishmentScope, bool, error) {
	if p == nil {
		return store.EstablishmentScope{}, false, auth.ErrUnauthenticated
	}
	perm := auth.PermBookingsRead
	if p.Kind == auth.PrincipalAPIKey {
		perm = auth.PermUsersManage
	}
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, perm)
	if err != nil {
		return store.EstablishmentScope{}, false, err
	}
	return es, p.Can(es, auth.PermUsersManage), nil
}

// List returns the devices of establishmentID visible to p (all of them
// with users.manage, otherwise p's own), most recently used first.
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID string, f ListFilter) (store.Page[View], error) {
	es, manage, err := s.access(ctx, p, establishmentID)
	if err != nil {
		return store.Page[View]{}, err
	}
	after, err := store.DecodeCursor(f.Cursor)
	if err != nil {
		return store.Page[View]{}, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	limit = min(limit, MaxListLimit)
	filter := store.DeviceFilter{IncludeRevoked: f.IncludeRevoked}
	if !manage {
		filter.UserID = p.UserID
	}
	views, err := s.views(ctx, s.d.DB, es, p, filter)
	if err != nil {
		return store.Page[View]{}, err
	}
	sortViews(views)
	page, next, ok := paginate(views, after, limit)
	if !ok {
		return store.Page[View]{}, store.ErrInvalidCursor
	}
	out := store.Page[View]{Items: page}
	if next != "" {
		out.NextCursor = store.EncodeCursor(next)
	}
	return out, nil
}

// views loads the devices matching f with user name, effective role and
// fiscal bridge flag. A user without a role left on the establishment
// keeps their devices listed (so they can be revoked) with the least
// privileged role; such devices do not authenticate anyway.
func (s *Service) views(ctx context.Context, db bun.IDB, es store.EstablishmentScope, p *auth.Principal, f store.DeviceFilter) ([]View, error) {
	rows, err := store.ListDevices(ctx, db, es, f)
	if err != nil {
		return nil, err
	}
	grants, err := store.ListTeamGrants(ctx, db, es)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		role, ok := effectiveRole(grants, r.UserID)
		if !ok {
			role = auth.RoleViewer
		}
		out[i] = View{
			Device: r.Device, UserName: r.UserName, Role: role, FiscalBridge: r.FiscalBridge,
			Current: p != nil && p.DeviceID == r.ID,
		}
	}
	return out, nil
}

// Revoke revokes deviceID (idempotent) and deactivates its fiscal bridge
// registration in the same transaction. Audit "device.revoked". Devices of
// other users need users.manage; without it they are reported as not found
// (no existence leak). The token stops working on the next request: every
// authentication reads revoked_at, there is no cache.
func (s *Service) Revoke(ctx context.Context, p *auth.Principal, establishmentID, deviceID string) error {
	es, manage, err := s.access(ctx, p, establishmentID)
	if err != nil {
		return err
	}
	return s.revoke(ctx, p, es, deviceID, func(d *store.Device) bool { return manage || d.UserID == p.UserID }, "panel")
}

// Current returns the device of a device principal (p.DeviceID).
func (s *Service) Current(ctx context.Context, p *auth.Principal) (*View, error) {
	if p == nil || p.DeviceID == "" {
		return nil, auth.ErrUnauthenticated
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, p.DeviceEstablishmentID)
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	vs, err := s.views(ctx, s.d.DB, es, p, store.DeviceFilter{DeviceID: p.DeviceID})
	if err != nil {
		return nil, err
	}
	if len(vs) != 1 {
		return nil, auth.ErrUnauthenticated
	}
	return &vs[0], nil
}

// UnpairCurrent revokes the device of a device principal (logout of the
// app). Audit "device.revoked" with reason "self".
func (s *Service) UnpairCurrent(ctx context.Context, p *auth.Principal) error {
	if p == nil || p.DeviceID == "" {
		return auth.ErrUnauthenticated
	}
	es, err := store.SystemEstablishmentScope(ctx, s.d.DB, p.DeviceEstablishmentID)
	if err != nil {
		return auth.ErrUnauthenticated
	}
	return s.revoke(ctx, p, es, p.DeviceID, func(d *store.Device) bool { return d.UserID == p.UserID }, "self")
}

func (s *Service) revoke(ctx context.Context, p *auth.Principal, es store.EstablishmentScope, deviceID string, allowed func(*store.Device) bool, reason string) error {
	err := store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		d, err := store.GetDevice(ctx, tx, es, deviceID)
		if err != nil {
			return err
		}
		if !allowed(d) {
			return store.ErrNotFound
		}
		now := s.d.Clock.Now().UTC()
		revoked, err := store.RevokeDevice(ctx, tx, es, deviceID, now)
		if err != nil || !revoked {
			return err // already revoked: idempotent, nothing to audit
		}
		if s.d.Bridge != nil {
			if err := s.d.Bridge.DeactivateDevice(ctx, tx, es, deviceID); err != nil {
				return err
			}
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(""), Action: "device.revoked",
			EntityType: "device", EntityID: deviceID,
			Diff: map[string]any{"user_id": d.UserID, "name": d.Name, "reason": reason},
		})
	})
	if err != nil {
		return fmt.Errorf("device: revoke: %w", err)
	}
	return nil
}

// AuthenticateDevice resolves a device token to its principal: shape check
// (ParseToken), lookup by token_hash with revoked_at IS NULL, idle expiry
// (IdleExpired: unused for IdleExpiry), then Principals.PrincipalForDevice
// (active user with a role on the device's establishment, grants narrowed
// to it); touches last_used_at at most every TouchInterval. Any failure to
// authenticate is auth.ErrUnauthenticated; database errors are returned as
// such.
func (s *Service) AuthenticateDevice(ctx context.Context, token string) (*auth.Principal, error) {
	prefix, ok := ParseToken(token)
	if !ok || s.d.DB == nil || s.d.Principals == nil {
		return nil, auth.ErrUnauthenticated
	}
	d, err := store.GetActiveDeviceByTokenHash(ctx, s.d.DB, HashToken(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, auth.ErrUnauthenticated
		}
		return nil, fmt.Errorf("device: authenticate: %w", err)
	}
	if d.TokenPrefix != prefix {
		return nil, auth.ErrUnauthenticated
	}
	now := s.d.Clock.Now().UTC()
	if IdleExpired(d, now) {
		s.d.Log.InfoContext(ctx, "device: token expired for inactivity", slog.String("device_id", d.ID))
		return nil, auth.ErrUnauthenticated
	}
	p, err := s.d.Principals.PrincipalForDevice(ctx, auth.DeviceIdentity{
		DeviceID: d.ID, DeviceName: d.Name, UserID: d.UserID, EstablishmentID: d.EstablishmentID,
	})
	if err != nil {
		return nil, err
	}
	if d.LastUsedAt == nil || now.Sub(*d.LastUsedAt) >= TouchInterval {
		if _, err := store.TouchDevice(ctx, s.d.DB, d.ID, now, now.Add(-TouchInterval)); err != nil {
			// Bookkeeping only: never fail the request for it.
			s.d.Log.WarnContext(ctx, "device: touch last_used_at", slog.String("device_id", d.ID), slog.String("error", err.Error()))
		}
	}
	return p, nil
}

// CleanupExpiredCodes deletes pairing codes expired for more than a day.
func (s *Service) CleanupExpiredCodes(ctx context.Context) error {
	if s.d.DB == nil {
		return nil
	}
	n, err := store.DeleteExpiredPairingCodes(ctx, s.d.DB, s.d.Clock.Now().UTC().Add(-codeRetention))
	if err != nil {
		return fmt.Errorf("device: cleanup pairing codes: %w", err)
	}
	if n > 0 {
		s.d.Log.InfoContext(ctx, "device: expired pairing codes deleted", slog.Int64("count", n))
	}
	return nil
}

// Jobs are the background jobs of the device service.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "device.cleanup_pairing_codes", Interval: time.Hour, Run: s.CleanupExpiredCodes}}
}
