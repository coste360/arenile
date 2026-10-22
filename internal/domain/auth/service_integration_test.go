//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var fast = auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func newService(t *testing.T, db *store.DB, c clock.Clock, requireTOTP bool) *auth.Service {
	t.Helper()
	kr, err := crypto.NewKeyRing(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewService(db, c, kr, audit.NewWriter(c), nil, auth.Options{Argon2: &fast, RequireTOTPForOwners: requireTOTP})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestService_LoginSessionRotationLogout(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		s := newService(t, db, c, false)
		res, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "Owner@Lido.it", Name: "Owner", Password: "supersecret1", OrganizationName: "Lido Srl"})
		if err != nil || !res.OrganizationCreated {
			t.Fatalf("create owner: %+v %v", res, err)
		}
		if _, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido.it", Name: "X", Password: "supersecret1", OrganizationName: "Lido Srl"}); !errors.Is(err, auth.ErrUserExists) {
			t.Fatalf("duplicate: %v", err)
		}
		if _, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "b@lido.it", Name: "X", Password: "short", OrganizationName: "Lido Srl"}); !errors.Is(err, auth.ErrWeakPassword) {
			t.Fatalf("weak: %v", err)
		}

		if _, err := s.Login(ctx, auth.LoginInput{Email: "owner@lido.it", Password: "wrong-password", IP: "1.1.1.1"}); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("wrong password: %v", err)
		}
		if _, err := s.Login(ctx, auth.LoginInput{Email: "nobody@lido.it", Password: "whatever123", IP: "1.1.1.1"}); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("unknown user: %v", err)
		}
		first, err := s.Login(ctx, auth.LoginInput{Email: "OWNER@lido.it", Password: "supersecret1", IP: "1.1.1.1", UserAgent: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if first.Principal.Kind != auth.PrincipalUser || len(first.Principal.Grants) != 1 || first.Principal.Grants[0].Role != auth.RoleOwner {
			t.Fatalf("principal: %+v", first.Principal)
		}
		// The raw token is never stored.
		var stored int
		_ = db.NewSelect().TableExpr("sessions").ColumnExpr("count(*)").Where("token_hash = ?", first.Token).Scan(ctx, &stored)
		if stored != 0 {
			t.Fatal("raw token must not be stored")
		}
		p, err := s.AuthenticateSession(ctx, first.Token)
		if err != nil || p.UserID != first.Principal.UserID {
			t.Fatalf("authenticate: %v", err)
		}

		// Login again presenting the old cookie: token rotates, old one is revoked.
		second, err := s.Login(ctx, auth.LoginInput{Email: "owner@lido.it", Password: "supersecret1", IP: "1.1.1.1", PreviousToken: first.Token})
		if err != nil {
			t.Fatal(err)
		}
		if second.Token == first.Token {
			t.Fatal("token must rotate")
		}
		if _, err := s.AuthenticateSession(ctx, first.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("old token must be revoked: %v", err)
		}

		// Expiry.
		c.Advance(8 * 24 * time.Hour)
		if _, err := s.AuthenticateSession(ctx, second.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("expired: %v", err)
		}
		n, err := s.CleanupExpiredSessions(ctx)
		if err != nil || n != 1 {
			t.Fatalf("cleanup: %d %v", n, err)
		}

		third, _ := s.Login(ctx, auth.LoginInput{Email: "owner@lido.it", Password: "supersecret1", IP: "1.1.1.1"})
		if err := s.Logout(ctx, third.Token, third.Principal.Actor("1.1.1.1")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AuthenticateSession(ctx, third.Token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatal("logged out session must not authenticate")
		}

		var audits int
		_ = db.NewSelect().TableExpr("audit_log").ColumnExpr("count(*)").Where("action = 'auth.login'").Scan(ctx, &audits)
		if audits != 3 {
			t.Fatalf("login audit entries: %d", audits)
		}
	})
}

func TestService_LoginRateLimit(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	s := newService(t, db, c, false)
	if _, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "o@lido.it", Name: "O", Password: "supersecret1", OrganizationName: "Org"}); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		_, _ = s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "bad-password", IP: "9.9.9.9"})
	}
	_, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", IP: "8.8.8.8"})
	var rl *auth.RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("expected rate limit, got %v", err)
	}
	c.Advance(16 * time.Minute)
	if _, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", IP: "8.8.8.8"}); err != nil {
		t.Fatalf("after window: %v", err)
	}
}

func TestService_TOTPEnrolmentAndLogin(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		s := newService(t, db, c, true)
		if _, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "o@lido.it", Name: "O", Password: "supersecret1", OrganizationName: "Org"}); err != nil {
			t.Fatal(err)
		}
		res, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", IP: "1.1.1.1"})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Principal.TOTPEnrolmentRequired {
			t.Fatal("owner without TOTP must be asked to enrol")
		}
		if _, err := res.Principal.Authorize(ctx, db, "any", auth.PermBookingsRead); !errors.Is(err, auth.ErrTOTPEnrolmentNeeded) {
			t.Fatalf("authorize before enrolment: %v", err)
		}
		enr, err := s.BeginTOTPEnrolment(ctx, res.Principal, "1.1.1.1")
		if err != nil {
			t.Fatal(err)
		}
		// Secret is encrypted at rest.
		u, _ := store.GetUser(ctx, db, res.Principal.UserID)
		if u.TOTPSecret == nil || *u.TOTPSecret == enr.Secret || u.TOTPEnabled() {
			t.Fatal("secret must be stored encrypted and pending")
		}
		if err := s.ConfirmTOTP(ctx, res.Principal, "000000", ""); !errors.Is(err, auth.ErrInvalidTOTP) {
			t.Fatalf("wrong code: %v", err)
		}
		code, _ := auth.TOTPCode(enr.Secret, auth.TOTPStep(c.Now()))
		if err := s.ConfirmTOTP(ctx, res.Principal, code, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := s.BeginTOTPEnrolment(ctx, res.Principal, ""); !errors.Is(err, auth.ErrTOTPAlreadyEnabled) {
			t.Fatalf("re-enrol: %v", err)
		}

		if _, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", IP: "1.1.1.1"}); !errors.Is(err, auth.ErrTOTPRequired) {
			t.Fatalf("missing code: %v", err)
		}
		// The code used for confirmation cannot be replayed.
		if _, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", TOTPCode: code, IP: "1.1.1.1"}); !errors.Is(err, auth.ErrInvalidTOTP) {
			t.Fatalf("replay: %v", err)
		}
		c.Advance(30 * time.Second)
		code2, _ := auth.TOTPCode(enr.Secret, auth.TOTPStep(c.Now()))
		res2, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", TOTPCode: code2, IP: "1.1.1.1"})
		if err != nil {
			t.Fatal(err)
		}
		if res2.Principal.TOTPEnrolmentRequired || !res2.Principal.TOTPEnabled {
			t.Fatal("enrolled owner must have full access")
		}
	})
}

func TestService_APIKeysAndCrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		s := newService(t, db, c, false)
		res, err := s.CreateOwner(ctx, auth.CreateOwnerInput{Email: "o@lido.it", Name: "O", Password: "supersecret1", OrganizationName: "Org A"})
		if err != nil {
			t.Fatal(err)
		}
		estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
		if err := store.InsertEstablishment(ctx, db, estA); err != nil {
			t.Fatal(err)
		}
		_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		login, err := s.Login(ctx, auth.LoginInput{Email: "o@lido.it", Password: "supersecret1", IP: "1.1.1.1"})
		if err != nil {
			t.Fatal(err)
		}
		p := login.Principal
		if _, err := p.Authorize(ctx, db, estA.ID, auth.PermUsersManage); err != nil {
			t.Fatalf("owner on own establishment: %v", err)
		}
		if _, err := p.Authorize(ctx, db, estB.ID, auth.PermBookingsRead); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant must be not found: %v", err)
		}
		if _, _, err := s.CreateAPIKey(ctx, p, auth.CreateAPIKeyInput{EstablishmentID: estB.ID, Name: "x", Scopes: []auth.Permission{auth.PermBookingsRead}}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("api key on foreign establishment: %v", err)
		}
		key, row, err := s.CreateAPIKey(ctx, p, auth.CreateAPIKeyInput{EstablishmentID: estA.ID, Name: "POS", Scopes: []auth.Permission{auth.PermBookingsRead}})
		if err != nil {
			t.Fatal(err)
		}
		if row.KeyHash == key {
			t.Fatal("key must be stored hashed")
		}
		kp, err := s.AuthenticateAPIKey(ctx, key)
		if err != nil || kp.Kind != auth.PrincipalAPIKey || kp.APIKeyEstablishmentID != estA.ID {
			t.Fatalf("api key auth: %+v %v", kp, err)
		}
		if _, err := kp.Authorize(ctx, db, estA.ID, auth.PermBookingsRead); err != nil {
			t.Fatal(err)
		}
		if _, err := kp.Authorize(ctx, db, estA.ID, auth.PermBookingsWrite); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("scope not granted: %v", err)
		}
		if _, err := kp.Authorize(ctx, db, estB.ID, auth.PermBookingsRead); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("api key cross tenant: %v", err)
		}
		last := byte('a')
		if key[len(key)-1] == 'a' {
			last = 'b'
		}
		if _, err := s.AuthenticateAPIKey(ctx, key[:len(key)-1]+string(last)); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("tampered key: %v", err)
		}

		// A cashier limited to establishment A.
		cashier := &store.User{ID: store.NewID(), Email: "c@lido.it", PasswordHash: "x", Name: "C", Locale: "it", Active: true, CreatedAt: c.Now(), UpdatedAt: c.Now()}
		_ = store.InsertUser(ctx, db, cashier)
		_ = store.InsertMembership(ctx, db, &store.Membership{ID: store.NewID(), UserID: cashier.ID, OrganizationID: res.Organization.ID, EstablishmentID: &estA.ID, Role: "cashier", CreatedAt: c.Now(), UpdatedAt: c.Now()})
		cp := &auth.Principal{Kind: auth.PrincipalUser, UserID: cashier.ID, Grants: []auth.Grant{{OrganizationID: res.Organization.ID, EstablishmentID: estA.ID, Role: auth.RoleCashier}}}
		if _, err := cp.Authorize(ctx, db, estA.ID, auth.PermRefundsIssue); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier refunds: %v", err)
		}
		if _, err := cp.Authorize(ctx, db, estA.ID, auth.PermCashOperate); err != nil {
			t.Fatalf("cashier cash: %v", err)
		}
	})
}
