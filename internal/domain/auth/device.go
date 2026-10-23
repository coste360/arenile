package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/store"
)

// DeviceIdentity is the paired device a device token resolved to. It is
// built by internal/domain/device after checking the token hash and the
// revocation; this package only turns it into a principal.
type DeviceIdentity struct {
	DeviceID        string
	DeviceName      string
	UserID          string
	EstablishmentID string
}

// PrincipalForDevice builds the principal of a device token (spec 29.2:
// "token per dispositivo legato all'utente, con i permessi del suo ruolo").
// It is a user principal (Kind PrincipalUser, audit actor = the user) whose
// grants are narrowed to the device establishment: an organization-wide
// grant becomes a grant on that establishment only, grants on other
// establishments are dropped. An inactive user or an establishment the user
// no longer has a role on yields ErrUnauthenticated. TOTP is not asked
// again: pairing happened from an authenticated panel session.
func (s *Service) PrincipalForDevice(ctx context.Context, d DeviceIdentity) (*Principal, error) {
	user, err := store.GetUser(ctx, s.db, d.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth: device user: %w", err)
	}
	if !user.Active {
		return nil, ErrUnauthenticated
	}
	es, err := store.SystemEstablishmentScope(ctx, s.db, d.EstablishmentID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	p, err := s.userPrincipal(ctx, user, "")
	if err != nil {
		return nil, err
	}
	p.Grants = NarrowGrants(p.Grants, es.OrganizationID(), es.ID())
	if len(p.Grants) == 0 {
		return nil, ErrUnauthenticated
	}
	p.TOTPEnrolmentRequired = false
	p.DeviceID, p.DeviceName, p.DeviceEstablishmentID = d.DeviceID, d.DeviceName, es.ID()
	return p, nil
}

// NarrowGrants keeps the grants that apply to establishmentID (of
// organizationID) and rewrites them as establishment grants. Pure function.
func NarrowGrants(grants []Grant, organizationID, establishmentID string) []Grant {
	var out []Grant
	for _, g := range grants {
		if g.OrganizationID != organizationID || (g.EstablishmentID != "" && g.EstablishmentID != establishmentID) {
			continue
		}
		out = append(out, Grant{OrganizationID: organizationID, EstablishmentID: establishmentID, Role: g.Role})
	}
	return out
}
