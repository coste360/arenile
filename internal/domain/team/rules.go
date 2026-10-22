package team

import (
	"slices"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
)

// Scope tells whether a role applies to one establishment or to every
// establishment of the organization.
type Scope string

// Scopes of a membership.
const (
	ScopeEstablishment Scope = "establishment"
	ScopeOrganization  Scope = "organization"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool { return s == ScopeEstablishment || s == ScopeOrganization }

func scopeOf(m store.Membership) Scope {
	if m.EstablishmentID == nil {
		return ScopeOrganization
	}
	return ScopeEstablishment
}

// rank orders roles from the most privileged (0).
func rank(r auth.Role) int {
	if i := slices.Index(auth.Roles, r); i >= 0 {
		return i
	}
	return len(auth.Roles)
}

// Effective returns the most privileged role among memberships and the scope
// of the membership granting it (organization wins a tie). ok is false for
// no memberships.
func Effective(ms []store.Membership) (role auth.Role, scope Scope, ok bool) {
	best := -1
	for i, m := range ms {
		if best < 0 || rank(auth.Role(m.Role)) < rank(auth.Role(ms[best].Role)) ||
			(m.Role == ms[best].Role && m.EstablishmentID == nil) {
			best = i
		}
	}
	if best < 0 {
		return "", "", false
	}
	return auth.Role(ms[best].Role), scopeOf(ms[best]), true
}

// Grant is a membership in the last-owner computation.
type Grant struct {
	UserID string
	// OrgWide is true for organization-wide memberships.
	OrgWide bool
	Role    auth.Role
	Active  bool
}

// GrantsFrom converts the store rows.
func GrantsFrom(rows []store.TeamGrant) []Grant {
	out := make([]Grant, len(rows))
	for i, r := range rows {
		out[i] = Grant{UserID: r.UserID, OrgWide: r.EstablishmentID == nil, Role: auth.Role(r.Role), Active: r.UserActive}
	}
	return out
}

// CheckLastOwner enforces the rule of spec 16: a change may never leave the
// organization without an active organization-wide owner, nor the
// establishment without an active owner, when it had one before. before are
// the grants applying to the establishment; after is the same set with the
// change applied.
func CheckLastOwner(before, after []Grant) error {
	count := func(gs []Grant, orgOnly bool) int {
		users := map[string]bool{}
		for _, g := range gs {
			if g.Role == auth.RoleOwner && g.Active && (g.OrgWide || !orgOnly) {
				users[g.UserID] = true
			}
		}
		return len(users)
	}
	if count(before, true) > 0 && count(after, true) == 0 {
		return ErrLastOwner
	}
	if count(before, false) > 0 && count(after, false) == 0 {
		return ErrLastOwner
	}
	return nil
}

// replaceUser returns grants with the grants of userID replaced by repl.
func replaceUser(grants []Grant, userID string, repl ...Grant) []Grant {
	out := make([]Grant, 0, len(grants)+len(repl))
	for _, g := range grants {
		if g.UserID != userID {
			out = append(out, g)
		}
	}
	return append(out, repl...)
}

// withActive returns grants with the active flag of userID set.
func withActive(grants []Grant, userID string, active bool) []Grant {
	out := slices.Clone(grants)
	for i := range out {
		if out[i].UserID == userID {
			out[i].Active = active
		}
	}
	return out
}
