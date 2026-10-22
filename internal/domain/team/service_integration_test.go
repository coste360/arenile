//go:build integration

package team_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/team"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var fastArgon = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

type fixture struct {
	db    *store.DB
	clock *clock.Fake
	auth  *auth.Service
	svc   *team.Service
	orgA  *store.Organization
	estA  *store.Establishment
	estA2 *store.Establishment
	estB  *store.Establishment
	owner *auth.Principal // organization-wide owner of org A
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC))
	aw := audit.NewWriter(c)
	as, err := auth.NewService(db, c, nil, aw, nil, auth.Options{Argon2: &fastArgon})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner", Password: "password-owner-1", OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
	estA2 := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a2", "Lido A2", c.Now())
	for _, e := range []*store.Establishment{estA, estA2} {
		if err := store.InsertEstablishment(ctx, db, e); err != nil {
			t.Fatal(err)
		}
	}
	_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		db: db, clock: c, auth: as, orgA: res.Organization, estA: estA, estA2: estA2, estB: estB,
		svc: team.NewService(team.Deps{DB: db, Clock: c, Audit: aw, Outbox: event.NewOutbox(c), Auth: as}),
	}
	f.owner = f.principal(t, res.User.ID)
	return f
}

// principal builds the principal of a user from the stored memberships.
func (f *fixture) principal(t *testing.T, userID string) *auth.Principal {
	t.Helper()
	ms, err := store.ListUserMemberships(context.Background(), f.db, userID)
	if err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{Kind: auth.PrincipalUser, UserID: userID}
	for _, m := range ms {
		g := auth.Grant{OrganizationID: m.OrganizationID, Role: auth.Role(m.Role)}
		if m.EstablishmentID != nil {
			g.EstablishmentID = *m.EstablishmentID
		}
		p.Grants = append(p.Grants, g)
	}
	return p
}

func (f *fixture) add(t *testing.T, email string, role auth.Role, scope team.Scope) *team.CreateResult {
	t.Helper()
	res, err := f.svc.Create(context.Background(), f.owner, f.estA.ID, team.CreateInput{Email: email, Name: email, Role: role, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func count(t *testing.T, db *store.DB, table, where string, args ...any) int64 {
	t.Helper()
	n, err := db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestTeamService_CreateAndLogin(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		res := f.add(t, "Cassa@Lido-A.it", auth.RoleCashier, "")
		if !res.UserCreated || len(res.InitialPassword) != team.GeneratedPasswordLength {
			t.Fatalf("result %+v", res)
		}
		m := res.Member
		if m.User.Email != "cassa@lido-a.it" || m.Role != auth.RoleCashier || m.Scope != team.ScopeEstablishment || m.User.Locale != "it" || !m.User.Active {
			t.Fatalf("member %+v", m)
		}
		// The generated password works, and the user sees only estA.
		login, err := f.auth.Login(ctx, auth.LoginInput{Email: "cassa@lido-a.it", Password: res.InitialPassword})
		if err != nil {
			t.Fatal(err)
		}
		if !login.Principal.Scope().Allows(f.estA.ID, f.orgA.ID) || login.Principal.Scope().Allows(f.estA2.ID, "") {
			t.Fatalf("scope %+v", login.Principal.Grants)
		}
		// With an explicit password nothing is returned.
		res2, err := f.svc.Create(ctx, f.owner, f.estA.ID, team.CreateInput{Email: "mgr@lido-a.it", Name: "Mgr", Password: "a-long-password", Role: auth.RoleManager, Locale: "en"})
		if err != nil || res2.InitialPassword != "" || res2.Member.User.Locale != "en" {
			t.Fatalf("explicit password: %+v %v", res2, err)
		}
		// Already a member here.
		if _, err := f.svc.Create(ctx, f.owner, f.estA.ID, team.CreateInput{Email: "cassa@lido-a.it", Name: "X", Role: auth.RoleViewer}); !errors.Is(err, team.ErrAlreadyMember) {
			t.Fatalf("duplicate: %v", err)
		}
		// Existing account added to the second establishment: reused.
		res3, err := f.svc.Create(ctx, f.owner, f.estA2.ID, team.CreateInput{Email: "cassa@lido-a.it", Name: "Ignored", Role: auth.RoleViewer})
		if err != nil || res3.UserCreated || res3.InitialPassword != "" || res3.Member.User.Name != "Cassa@Lido-A.it" {
			t.Fatalf("reuse: %+v %v", res3, err)
		}
		// Validation.
		_, err = f.svc.Create(ctx, f.owner, f.estA.ID, team.CreateInput{Email: "nope", Name: "", Role: "boss", Password: "short"})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 4 {
			t.Fatalf("validation: %v", err)
		}
		if n := count(t, db, "audit_log", "action = ?", team.ActionMemberAdded); n != 3 {
			t.Fatalf("audit %d", n)
		}
		page, err := f.svc.List(ctx, f.principal(t, res.Member.User.ID), f.estA.ID, "", 2)
		if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
			t.Fatalf("list page 1 %+v %v", page, err)
		}
		page, err = f.svc.List(ctx, f.owner, f.estA.ID, page.NextCursor, 2)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("list page 2 %+v %v", page, err)
		}
	})
}

func TestTeamService_Permissions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		mgr := f.principal(t, f.add(t, "mgr@lido-a.it", auth.RoleManager, team.ScopeOrganization).Member.User.ID)
		cashier := f.principal(t, f.add(t, "cassa@lido-a.it", auth.RoleCashier, "").Member.User.ID)
		viewer := f.principal(t, f.add(t, "viewer@lido-a.it", auth.RoleViewer, "").Member.User.ID)
		in := team.CreateInput{Email: "x@lido-a.it", Name: "X", Role: auth.RoleViewer}
		for _, p := range []*auth.Principal{mgr, cashier, viewer} {
			if _, err := f.svc.Create(ctx, p, f.estA.ID, in); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s create: %v", p.UserID, err)
			}
			if _, err := f.svc.Update(ctx, p, f.estA.ID, f.owner.UserID, team.UpdateInput{Role: ptr(auth.RoleViewer)}); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s update: %v", p.UserID, err)
			}
			if err := f.svc.Remove(ctx, p, f.estA.ID, f.owner.UserID, ""); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("%s remove: %v", p.UserID, err)
			}
			// Everybody can see the team.
			if _, err := f.svc.List(ctx, p, f.estA.ID, "", 10); err != nil {
				t.Fatalf("%s list: %v", p.UserID, err)
			}
		}
		// Cross tenant: the establishment of another organization is not found.
		if _, err := f.svc.List(ctx, f.owner, f.estB.ID, "", 10); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		// A user of org A is not a member of estB: not found.
		bOwner := &auth.Principal{Kind: auth.PrincipalUser, UserID: "b", Grants: []auth.Grant{{OrganizationID: f.estB.OrganizationID, Role: auth.RoleOwner}}}
		if _, err := f.svc.Update(ctx, bOwner, f.estB.ID, cashier.UserID, team.UpdateInput{Active: ptr(false)}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign user: %v", err)
		}
		// An establishment-level owner cannot grant organization-wide roles nor
		// change organization-wide members.
		estOwner := f.principal(t, f.add(t, "eo@lido-a.it", auth.RoleOwner, "").Member.User.ID)
		if _, err := f.svc.Create(ctx, estOwner, f.estA.ID, team.CreateInput{Email: "y@lido-a.it", Name: "Y", Role: auth.RoleViewer, Scope: team.ScopeOrganization}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("est owner org scope: %v", err)
		}
		if _, err := f.svc.Update(ctx, estOwner, f.estA.ID, mgr.UserID, team.UpdateInput{Role: ptr(auth.RoleViewer)}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("est owner on org member: %v", err)
		}
		if _, err := f.svc.Update(ctx, estOwner, f.estA.ID, cashier.UserID, team.UpdateInput{Role: ptr(auth.RoleManager)}); err != nil {
			t.Fatalf("est owner on est member: %v", err)
		}
	})
}

func TestTeamService_RolesDeactivationAndLastOwner(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		// The only owner cannot demote, deactivate or remove themselves.
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, f.owner.UserID, team.UpdateInput{Role: ptr(auth.RoleManager)}); !errors.Is(err, team.ErrLastOwner) {
			t.Fatalf("demote last owner: %v", err)
		}
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, f.owner.UserID, team.UpdateInput{Scope: ptr(team.ScopeEstablishment)}); !errors.Is(err, team.ErrLastOwner) {
			t.Fatalf("narrow last owner: %v", err)
		}
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, f.owner.UserID, team.UpdateInput{Active: ptr(false)}); !errors.Is(err, team.ErrSelfDeactivation) {
			t.Fatalf("self deactivate: %v", err)
		}
		if err := f.svc.Remove(ctx, f.owner, f.estA.ID, f.owner.UserID, ""); !errors.Is(err, team.ErrLastOwner) {
			t.Fatalf("remove last owner: %v", err)
		}
		// A second organization owner makes demotion possible.
		second := f.add(t, "owner2@lido-a.it", auth.RoleOwner, team.ScopeOrganization).Member.User.ID
		m, err := f.svc.Update(ctx, f.owner, f.estA.ID, second, team.UpdateInput{Role: ptr(auth.RoleManager), Scope: ptr(team.ScopeEstablishment)})
		if err != nil || m.Role != auth.RoleManager || m.Scope != team.ScopeEstablishment || len(m.Memberships) != 1 {
			t.Fatalf("demote second: %+v %v", m, err)
		}
		// Moving to establishment scope removed the org-wide grant: no access to estA2.
		if p := f.principal(t, second); p.Scope().Allows(f.estA2.ID, "") {
			t.Fatal("still sees estA2")
		}
		// Deactivate a cashier: sessions end, login is refused.
		cash := f.add(t, "cassa@lido-a.it", auth.RoleCashier, "")
		if _, err := f.auth.Login(ctx, auth.LoginInput{Email: "cassa@lido-a.it", Password: cash.InitialPassword}); err != nil {
			t.Fatal(err)
		}
		m, err = f.svc.Update(ctx, f.owner, f.estA.ID, cash.Member.User.ID, team.UpdateInput{Active: ptr(false)})
		if err != nil || m.User.Active {
			t.Fatalf("deactivate: %+v %v", m, err)
		}
		if n := count(t, db, "sessions", "user_id = ?", cash.Member.User.ID); n != 0 {
			t.Fatalf("sessions left %d", n)
		}
		if _, err := f.auth.Login(ctx, auth.LoginInput{Email: "cassa@lido-a.it", Password: cash.InitialPassword}); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("login of deactivated user: %v", err)
		}
		if m, err = f.svc.Update(ctx, f.owner, f.estA.ID, cash.Member.User.ID, team.UpdateInput{Active: ptr(true)}); err != nil || !m.User.Active {
			t.Fatalf("reactivate: %v", err)
		}
		// A user who also works for another organization cannot be deactivated here.
		bOwner := &auth.Principal{Kind: auth.PrincipalUser, UserID: "b", Grants: []auth.Grant{{OrganizationID: f.estB.OrganizationID, Role: auth.RoleOwner}}}
		if _, err := f.svc.Create(ctx, bOwner, f.estB.ID, team.CreateInput{Email: "cassa@lido-a.it", Name: "C", Role: auth.RoleCashier}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, cash.Member.User.ID, team.UpdateInput{Active: ptr(false)}); !errors.Is(err, team.ErrOtherOrganizations) {
			t.Fatalf("multi-org deactivation: %v", err)
		}
		// Remove access: the membership in org B stays.
		if err := f.svc.Remove(ctx, f.owner, f.estA.ID, cash.Member.User.ID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, cash.Member.User.ID, team.UpdateInput{Role: ptr(auth.RoleViewer)}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("removed member: %v", err)
		}
		if n := count(t, db, "memberships", "user_id = ?", cash.Member.User.ID); n != 1 {
			t.Fatalf("memberships left %d", n)
		}
		for action, want := range map[string]int64{
			team.ActionRoleChanged: 1, team.ActionMemberDeactivated: 1, team.ActionMemberReactivated: 1, team.ActionMemberRemoved: 1,
		} {
			if n := count(t, db, "audit_log", "action = ? AND establishment_id = ?", action, f.estA.ID); n != want {
				t.Fatalf("%s: %d", action, n)
			}
		}
	})
}

// TestConcurrency_LastOwnerNeverRemoved: two owners demote each other 50
// times each, concurrently; at least one owner always remains.
func TestConcurrency_LastOwnerNeverRemoved(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		second := f.add(t, "owner2@lido-a.it", auth.RoleOwner, team.ScopeOrganization).Member.User.ID
		p2 := f.principal(t, second)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var unexpected []error
		for i := range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				actor, target := f.owner, second
				if i%2 == 1 {
					actor, target = p2, f.owner.UserID
				}
				_, err := f.svc.Update(ctx, actor, f.estA.ID, target, team.UpdateInput{Role: ptr(auth.RoleManager)})
				if err != nil && !errors.Is(err, team.ErrLastOwner) && !errors.Is(err, auth.ErrForbidden) {
					mu.Lock()
					unexpected = append(unexpected, err)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if len(unexpected) > 0 {
			t.Fatal(unexpected)
		}
		if n := count(t, db, "memberships", "organization_id = ? AND role = 'owner' AND establishment_id IS NULL", f.orgA.ID); n != 1 {
			t.Fatalf("owners left %d", n)
		}
	})
}
