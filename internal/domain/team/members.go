package team

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
)

// SupportedLocales are the panel languages (spec 20).
var SupportedLocales = []string{"it", "en", "de", "fr"}

// Member is a user with access to an establishment.
type Member struct {
	User store.User
	// Memberships apply to the establishment (organization-wide or bound to it).
	Memberships []store.Membership
	// Role is the effective (most privileged) role here, Scope the scope of
	// the membership granting it.
	Role  auth.Role
	Scope Scope
}

func memberFrom(tm store.TeamMember) Member {
	role, scope, _ := Effective(tm.Memberships)
	return Member{User: tm.User, Memberships: tm.Memberships, Role: role, Scope: scope}
}

// MemberPage is a page of members.
type MemberPage struct {
	Items      []Member
	NextCursor string
}

// List lists the users with access to the establishment (view permission).
func (s *Service) List(ctx context.Context, p *auth.Principal, establishmentID, cursor string, limit int) (MemberPage, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermBookingsRead)
	if err != nil {
		return MemberPage{}, err
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	page, err := store.ListTeamMembers(ctx, s.d.DB, es, cursor, min(limit, MaxPageSize))
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return MemberPage{}, err
		}
		return MemberPage{}, fmt.Errorf("team: list: %w", err)
	}
	out := MemberPage{Items: make([]Member, len(page.Items)), NextCursor: page.NextCursor}
	for i, tm := range page.Items {
		out.Items[i] = memberFrom(tm)
	}
	return out, nil
}

// CreateInput adds a user to the establishment.
type CreateInput struct {
	Email  string
	Name   string
	Locale string
	// Password is the initial password; empty generates a temporary one,
	// returned once in CreateResult.InitialPassword.
	Password string
	Role     auth.Role
	// Scope defaults to ScopeEstablishment. ScopeOrganization requires the
	// caller to be an organization-wide owner.
	Scope Scope
	IP    string
}

// CreateResult reports the outcome of Create.
type CreateResult struct {
	Member Member
	// InitialPassword is set only when the server generated it. It is shown
	// once and never stored in clear.
	InitialPassword string
	// UserCreated is false when an existing account was given access.
	UserCreated bool
}

// Create gives a user a role on the establishment (owner only). A new
// account is created when no user has the email; an existing account is
// reused (its name and password are not changed). The account must not
// already have access to the establishment.
//
// No email is sent: with a generated password the panel shows it once and
// the owner hands it over (the M1 "one-time" credential; password reset by
// email arrives with the notifications module).
func (s *Service) Create(ctx context.Context, p *auth.Principal, establishmentID string, in CreateInput) (*CreateResult, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermUsersManage)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	email := store.NormalizeEmail(in.Email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
		ve.Add("email", "Indirizzo email non valido.")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		ve.Add("name", "Obbligatorio, al massimo 200 caratteri.")
	}
	if in.Locale != "" && !slices.Contains(SupportedLocales, in.Locale) {
		ve.Add("locale", "Lingua non supportata (it, en, de, fr).")
	}
	if !in.Role.Valid() {
		ve.Add("role", "Ruolo sconosciuto.")
	}
	scope := in.Scope
	if scope == "" {
		scope = ScopeEstablishment
	}
	if !scope.Valid() {
		ve.Add("scope", "Valori ammessi: establishment, organization.")
	}
	if in.Password != "" {
		if err := auth.ValidatePassword(in.Password); err != nil {
			ve.Add("password", fmt.Sprintf("Almeno %d caratteri.", auth.MinPasswordLength))
		}
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	if scope == ScopeOrganization && !isOrgOwner(p, es) {
		return nil, auth.ErrForbidden
	}

	// Hash outside the transaction (Argon2id is deliberately slow), only when
	// the account does not exist yet.
	var hash, generated string
	if _, err := store.GetUserByEmail(ctx, s.d.DB, email); errors.Is(err, store.ErrNotFound) {
		pw := in.Password
		if pw == "" {
			if generated, err = GeneratePassword(); err != nil {
				return nil, err
			}
			pw = generated
		}
		if hash, err = s.d.Auth.Hasher().Hash(pw); err != nil {
			return nil, fmt.Errorf("team: hash password: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("team: create: %w", err)
	}

	res := &CreateResult{}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.TeamLockOrganization(ctx, tx, es); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		user, err := store.GetUserByEmail(ctx, tx, email)
		switch {
		case errors.Is(err, store.ErrNotFound):
			if hash == "" {
				return fmt.Errorf("%w: utente rimosso durante l'operazione, riprova", configerr.ErrConflict)
			}
			locale := in.Locale
			if locale == "" {
				e, err := store.GetEstablishment(ctx, tx, es)
				if err != nil {
					return err
				}
				locale = e.DefaultLocale
			}
			user = &store.User{ID: store.NewID(), Email: email, PasswordHash: hash, Name: name, Locale: locale, Active: true, CreatedAt: now, UpdatedAt: now}
			if err := store.InsertUser(ctx, tx, user); err != nil {
				return err
			}
			res.UserCreated, res.InitialPassword = true, generated
		case err != nil:
			return err
		default:
			if _, err := store.GetTeamMember(ctx, tx, es, user.ID); err == nil {
				return ErrAlreadyMember
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		m := &store.Membership{ID: store.NewID(), UserID: user.ID, OrganizationID: es.OrganizationID(), Role: string(in.Role), CreatedAt: now, UpdatedAt: now}
		if scope == ScopeEstablishment {
			id := es.ID()
			m.EstablishmentID = &id
		}
		if err := store.InsertMembership(ctx, tx, m); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return ErrAlreadyMember
			}
			return err
		}
		if err := s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: ActionMemberAdded, EntityType: "user", EntityID: user.ID,
			Diff: map[string]any{"role": in.Role, "scope": scope, "user_created": res.UserCreated, "membership_id": m.ID},
		}); err != nil {
			return err
		}
		tm, err := store.GetTeamMember(ctx, tx, es, user.ID)
		if err != nil {
			return err
		}
		res.Member = memberFrom(*tm)
		return nil
	})
	if err != nil {
		return nil, wrap("create", err)
	}
	return res, nil
}

// UpdateInput changes a member: nil fields are unchanged.
type UpdateInput struct {
	Role *auth.Role
	// Scope moves the role to the establishment or to the whole
	// organization; the member then has exactly one membership applying here.
	Scope *Scope
	// Active false deactivates the account (it can no longer log in and its
	// sessions end); true reactivates it.
	Active *bool
	IP     string
}

// Update changes the role, scope or active flag of a member (owner only).
// Changes involving organization-wide memberships, and deactivating an
// account with access beyond this establishment, require an
// organization-wide owner. It never leaves the organization or the
// establishment without an active owner (ErrLastOwner).
func (s *Service) Update(ctx context.Context, p *auth.Principal, establishmentID, userID string, in UpdateInput) (*Member, error) {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermUsersManage)
	if err != nil {
		return nil, err
	}
	ve := &configerr.ValidationError{}
	if in.Role != nil && !in.Role.Valid() {
		ve.Add("role", "Ruolo sconosciuto.")
	}
	if in.Scope != nil && !in.Scope.Valid() {
		ve.Add("scope", "Valori ammessi: establishment, organization.")
	}
	if err := ve.OrNil(); err != nil {
		return nil, err
	}
	var out *Member
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.TeamLockOrganization(ctx, tx, es); err != nil {
			return err
		}
		tm, err := store.GetTeamMember(ctx, tx, es, userID)
		if err != nil {
			return err
		}
		cur := memberFrom(*tm)
		rows, err := store.ListTeamGrants(ctx, tx, es)
		if err != nil {
			return err
		}
		before := GrantsFrom(rows)
		after := before
		now := s.d.Clock.Now()

		role, scope := cur.Role, cur.Scope
		if in.Role != nil {
			role = *in.Role
		}
		if in.Scope != nil {
			scope = *in.Scope
		}
		roleChange := role != cur.Role || scope != cur.Scope || len(tm.Memberships) > 1
		if in.Role == nil && in.Scope == nil {
			roleChange = false
		}
		if roleChange {
			if (scope == ScopeOrganization || hasOrgWide(tm.Memberships)) && !isOrgOwner(p, es) {
				return auth.ErrForbidden
			}
			after = replaceUser(after, userID, Grant{UserID: userID, OrgWide: scope == ScopeOrganization, Role: role, Active: tm.User.Active})
		}
		activeChange := in.Active != nil && *in.Active != tm.User.Active
		if activeChange {
			if !*in.Active && p.Kind == auth.PrincipalUser && p.UserID == userID {
				return ErrSelfDeactivation
			}
			all, err := store.ListUserMemberships(ctx, tx, userID)
			if err != nil {
				return err
			}
			for _, m := range all {
				if m.OrganizationID != es.OrganizationID() {
					return ErrOtherOrganizations
				}
				if (m.EstablishmentID == nil || *m.EstablishmentID != es.ID()) && !isOrgOwner(p, es) {
					return auth.ErrForbidden
				}
			}
			after = withActive(after, userID, *in.Active)
		}
		if err := CheckLastOwner(before, after); err != nil {
			return err
		}
		if roleChange {
			if err := s.applyRole(ctx, tx, es, tm.Memberships, userID, role, scope, now); err != nil {
				return err
			}
			if err := s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: ActionRoleChanged, EntityType: "user", EntityID: userID,
				Diff: map[string]any{"from": map[string]any{"role": cur.Role, "scope": cur.Scope}, "to": map[string]any{"role": role, "scope": scope}},
			}); err != nil {
				return err
			}
		}
		if activeChange {
			if err := store.TeamSetUserActive(ctx, tx, userID, *in.Active, now); err != nil {
				return err
			}
			action := ActionMemberReactivated
			if !*in.Active {
				action = ActionMemberDeactivated
				if err := store.DeleteUserSessions(ctx, tx, userID); err != nil {
					return err
				}
			}
			if err := s.d.Audit.Record(ctx, tx, audit.Entry{
				EstablishmentID: es.ID(), Actor: p.Actor(in.IP), Action: action, EntityType: "user", EntityID: userID,
			}); err != nil {
				return err
			}
		}
		tm, err = store.GetTeamMember(ctx, tx, es, userID)
		if err != nil {
			return err
		}
		m := memberFrom(*tm)
		out = &m
		return nil
	})
	if err != nil {
		return nil, wrap("update", err)
	}
	return out, nil
}

// applyRole leaves the user with exactly one membership applying to the
// establishment: organization-wide or bound to it, with role.
func (s *Service) applyRole(ctx context.Context, tx bun.IDB, es store.EstablishmentScope, ms []store.Membership, userID string, role auth.Role, scope Scope, now time.Time) error {
	var keep *store.Membership
	for i := range ms {
		if scopeOf(ms[i]) == scope && keep == nil {
			keep = &ms[i]
			continue
		}
		if err := store.TeamDeleteMembership(ctx, tx, es, ms[i].ID); err != nil {
			return err
		}
	}
	if keep != nil {
		if keep.Role == string(role) {
			return nil
		}
		return store.TeamUpdateMembershipRole(ctx, tx, es, keep.ID, string(role), now)
	}
	m := &store.Membership{ID: store.NewID(), UserID: userID, OrganizationID: es.OrganizationID(), Role: string(role), CreatedAt: now, UpdatedAt: now}
	if scope == ScopeEstablishment {
		id := es.ID()
		m.EstablishmentID = &id
	}
	return store.InsertMembership(ctx, tx, m)
}

// Remove revokes the user's access to the establishment (owner only): every
// membership applying here is deleted. An organization-wide membership also
// grants the other establishments of the organization, so removing it
// requires an organization-wide owner. The account itself is kept.
func (s *Service) Remove(ctx context.Context, p *auth.Principal, establishmentID, userID, ip string) error {
	es, err := p.Authorize(ctx, s.d.DB, establishmentID, auth.PermUsersManage)
	if err != nil {
		return err
	}
	err = store.RunInTx(ctx, s.d.DB, func(ctx context.Context, tx bun.Tx) error {
		if err := store.TeamLockOrganization(ctx, tx, es); err != nil {
			return err
		}
		tm, err := store.GetTeamMember(ctx, tx, es, userID)
		if err != nil {
			return err
		}
		if hasOrgWide(tm.Memberships) && !isOrgOwner(p, es) {
			return auth.ErrForbidden
		}
		rows, err := store.ListTeamGrants(ctx, tx, es)
		if err != nil {
			return err
		}
		before := GrantsFrom(rows)
		if err := CheckLastOwner(before, replaceUser(before, userID)); err != nil {
			return err
		}
		removed := make([]map[string]any, 0, len(tm.Memberships))
		for _, m := range tm.Memberships {
			if err := store.TeamDeleteMembership(ctx, tx, es, m.ID); err != nil {
				return err
			}
			removed = append(removed, map[string]any{"membership_id": m.ID, "role": m.Role, "scope": scopeOf(m)})
		}
		left, err := store.ListUserMemberships(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(left) == 0 {
			if err := store.DeleteUserSessions(ctx, tx, userID); err != nil {
				return err
			}
		}
		return s.d.Audit.Record(ctx, tx, audit.Entry{
			EstablishmentID: es.ID(), Actor: p.Actor(ip), Action: ActionMemberRemoved, EntityType: "user", EntityID: userID,
			Diff: map[string]any{"memberships": removed},
		})
	})
	return wrap("remove", err)
}

// isOrgOwner reports whether p is an organization-wide owner of the
// establishment's organization.
func isOrgOwner(p *auth.Principal, es store.EstablishmentScope) bool {
	if p == nil || p.Kind != auth.PrincipalUser {
		return false
	}
	for _, g := range p.Grants {
		if g.Role == auth.RoleOwner && g.EstablishmentID == "" && g.OrganizationID == es.OrganizationID() {
			return true
		}
	}
	return false
}

func hasOrgWide(ms []store.Membership) bool {
	return slices.ContainsFunc(ms, func(m store.Membership) bool { return m.EstablishmentID == nil })
}

// passwordAlphabet avoids look-alike characters (0/O, 1/l/I).
const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GeneratedPasswordLength is the length of generated temporary passwords.
const GeneratedPasswordLength = 16

// GeneratePassword returns a random temporary password (about 93 bits).
func GeneratePassword() (string, error) {
	var b strings.Builder
	n := big.NewInt(int64(len(passwordAlphabet)))
	for range GeneratedPasswordLength {
		i, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", fmt.Errorf("team: generate password: %w", err)
		}
		b.WriteByte(passwordAlphabet[i.Int64()])
	}
	return b.String(), nil
}

func wrap(op string, err error) error {
	if err == nil || errors.Is(err, configerr.ErrConflict) || errors.Is(err, configerr.ErrValidation) ||
		errors.Is(err, store.ErrNotFound) || errors.Is(err, auth.ErrForbidden) {
		return err
	}
	return fmt.Errorf("team: %s: %w", op, err)
}
