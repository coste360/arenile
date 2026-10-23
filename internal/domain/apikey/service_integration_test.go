//go:build integration

package apikey_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/apikey"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type env struct {
	db      *store.DB
	c       *clock.Fake
	auth    *auth.Service
	svc     *apikey.Service
	owner   *auth.Principal
	manager *auth.Principal
	ownerB  *auth.Principal
	estA    string
	estB    string
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{3}, 32))
	aw := audit.NewWriter(c)
	as, err := auth.NewService(db, c, kr, aw, nil, auth.Options{Argon2: &auth.Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "o@lido-a.it", Name: "Owner A", Password: "supersecret1", OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
	if err := store.InsertEstablishment(ctx, db, estA); err != nil {
		t.Fatal(err)
	}
	resB, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "o@lido-b.it", Name: "Owner B", Password: "supersecret1", OrganizationName: "Org B"})
	if err != nil {
		t.Fatal(err)
	}
	estB := store.NewEstablishmentDefaults(resB.Organization.ID, "lido-b", "Lido B", c.Now())
	if err := store.InsertEstablishment(ctx, db, estB); err != nil {
		t.Fatal(err)
	}
	login := func(email string) *auth.Principal {
		l, err := as.Login(ctx, auth.LoginInput{Email: email, Password: "supersecret1", IP: "1.1.1.1"})
		if err != nil {
			t.Fatal(err)
		}
		return l.Principal
	}
	return &env{
		db: db, c: c, auth: as, svc: apikey.NewService(apikey.Deps{DB: db, Clock: c, Audit: aw}),
		owner: login("o@lido-a.it"), ownerB: login("o@lido-b.it"),
		manager: &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: res.Organization.ID, Role: auth.RoleManager}}},
		estA:    estA.ID, estB: estB.ID,
	}
}

func auditActions(t *testing.T, db *store.DB, entityID string) []string {
	t.Helper()
	var actions []string
	if err := db.NewSelect().Model((*store.AuditEntry)(nil)).Column("action").Where("entity_id = ?", entityID).OrderExpr("id ASC").Scan(context.Background(), &actions); err != nil {
		t.Fatal(err)
	}
	return actions
}

func TestAPIKeyService_Lifecycle(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		exp := e.c.Now().Add(time.Hour)
		k, secret, err := e.svc.Create(ctx, e.owner, e.estA, apikey.CreateInput{
			Name: "  Gestionale  ", Scopes: []auth.Permission{auth.PermReportsRead, auth.PermBookingsRead, auth.PermBookingsRead}, ExpiresAt: &exp, IP: "1.2.3.4",
		})
		if err != nil {
			t.Fatal(err)
		}
		if k.Name != "Gestionale" || len(k.Scopes) != 2 || k.Scopes[0] != auth.PermBookingsRead || k.Status != apikey.StatusActive ||
			k.CreatedByName != "Owner A" || k.DisplayPrefix == "" || k.ExpiresAt == nil {
			t.Fatalf("created %+v", k)
		}
		if _, ok := auth.ParseAPIKey(secret); !ok || len(secret) < 50 {
			t.Fatalf("secret format %q", secret)
		}
		var stored store.APIKey
		if err := db.NewSelect().Model(&stored).Where("id = ?", k.ID).Scan(ctx); err != nil || stored.KeyHash != auth.HashToken(secret) {
			t.Fatalf("only the SHA-256 is stored: %v", err)
		}

		// The key authenticates with exactly its scopes, on its establishment.
		kp, err := e.auth.AuthenticateAPIKey(ctx, secret)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := kp.Authorize(ctx, db, e.estA, auth.PermReportsRead); err != nil {
			t.Fatal(err)
		}
		if _, err := kp.Authorize(ctx, db, e.estA, auth.PermBookingsWrite); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("scope not granted: %v", err)
		}
		// A key never manages keys, even of its own establishment.
		if _, err := e.svc.List(ctx, kp, e.estA, false); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("key lists keys: %v", err)
		}
		if _, _, err := e.svc.Create(ctx, kp, e.estA, apikey.CreateInput{Name: "x", Scopes: []auth.Permission{auth.PermBookingsRead}}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("key creates keys: %v", err)
		}

		// Update: scopes, name, expiry.
		name := "Gestionale 2"
		k2, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{Name: &name, Scopes: []auth.Permission{auth.PermBookingsWrite, auth.PermBookingsRead}, ClearExpiry: true})
		if err != nil || k2.Name != name || k2.ExpiresAt != nil || len(k2.Scopes) != 2 || k2.Scopes[0] != auth.PermBookingsWrite {
			t.Fatalf("update %+v %v", k2, err)
		}
		kp, _ = e.auth.AuthenticateAPIKey(ctx, secret)
		if _, err := kp.Authorize(ctx, db, e.estA, auth.PermBookingsWrite); err != nil {
			t.Fatalf("new scope not effective: %v", err)
		}
		if _, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{ExpiresAt: &exp, ClearExpiry: true}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("clear and set expiry: %v", err)
		}
		if _, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{Scopes: []auth.Permission{auth.PermUsersManage}}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("users.manage scope: %v", err)
		}
		// A no-op update writes no audit entry.
		if _, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{Name: &name}); err != nil {
			t.Fatal(err)
		}

		// Revoke: immediate, idempotent, then unchangeable.
		if err := e.svc.Revoke(ctx, e.owner, e.estA, k.ID, "1.2.3.4"); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Revoke(ctx, e.owner, e.estA, k.ID, "1.2.3.4"); err != nil {
			t.Fatalf("second revoke: %v", err)
		}
		if _, err := e.auth.AuthenticateAPIKey(ctx, secret); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("revoked key authenticates: %v", err)
		}
		if _, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{Name: &name}); !errors.Is(err, apikey.ErrRevoked) {
			t.Fatalf("update revoked: %v", err)
		}
		if got := auditActions(t, db, k.ID); len(got) != 3 || got[0] != "api_key.created" || got[1] != "api_key.updated" || got[2] != "api_key.revoked" {
			t.Fatalf("audit %v", got)
		}
		list, err := e.svc.List(ctx, e.owner, e.estA, false)
		if err != nil || len(list) != 0 {
			t.Fatalf("revoked hidden: %v %v", list, err)
		}
		list, err = e.svc.List(ctx, e.owner, e.estA, true)
		if err != nil || len(list) != 1 || list[0].Status != apikey.StatusRevoked {
			t.Fatalf("revoked listed: %+v %v", list, err)
		}
	})
}

// TestAPIKeyService_ExpiredKeyRejected: auth.AuthenticateAPIKey refuses a
// key from its expires_at on (fails without the M5A expiry check).
func TestAPIKeyService_ExpiredKeyRejected(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		exp := e.c.Now().Add(time.Hour)
		k, secret, err := e.svc.Create(ctx, e.owner, e.estA, apikey.CreateInput{Name: "Scade", Scopes: []auth.Permission{auth.PermBookingsRead}, ExpiresAt: &exp})
		if err != nil {
			t.Fatal(err)
		}
		e.c.Advance(time.Hour - time.Second)
		if _, err := e.auth.AuthenticateAPIKey(ctx, secret); err != nil {
			t.Fatalf("before expiry: %v", err)
		}
		e.c.Advance(time.Second)
		if _, err := e.auth.AuthenticateAPIKey(ctx, secret); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("expired key authenticates: %v", err)
		}
		got, _ := e.svc.Get(ctx, e.owner, e.estA, k.ID)
		if got.Status != apikey.StatusExpired {
			t.Fatalf("status %s", got.Status)
		}
		// Extending the expiry revives it.
		later := e.c.Now().Add(24 * time.Hour)
		if _, err := e.svc.Update(ctx, e.owner, e.estA, k.ID, apikey.UpdateInput{ExpiresAt: &later}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.auth.AuthenticateAPIKey(ctx, secret); err != nil {
			t.Fatalf("after extension: %v", err)
		}
		past := e.c.Now().Add(-time.Minute)
		if _, _, err := e.svc.Create(ctx, e.owner, e.estA, apikey.CreateInput{Name: "x", Scopes: []auth.Permission{auth.PermBookingsRead}, ExpiresAt: &past}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("past expiry: %v", err)
		}
	})
}

func TestAPIKeyService_PermissionsAndTenancy(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		in := apikey.CreateInput{Name: "x", Scopes: []auth.Permission{auth.PermBookingsRead}}
		if _, _, err := e.svc.Create(ctx, e.manager, e.estA, in); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("manager: %v", err)
		}
		if _, _, err := e.svc.Create(ctx, nil, e.estA, in); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("anonymous: %v", err)
		}
		if _, _, err := e.svc.Create(ctx, e.ownerB, e.estA, in); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant create: %v", err)
		}
		k, _, err := e.svc.Create(ctx, e.owner, e.estA, in)
		if err != nil {
			t.Fatal(err)
		}
		// Owner B reaches A's key neither through A nor through B.
		if _, err := e.svc.Get(ctx, e.ownerB, e.estA, k.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant get: %v", err)
		}
		if _, err := e.svc.Get(ctx, e.ownerB, e.estB, k.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign key through own establishment: %v", err)
		}
		if err := e.svc.Revoke(ctx, e.ownerB, e.estB, k.ID, ""); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant revoke: %v", err)
		}
		for _, bad := range []apikey.CreateInput{
			{Name: "", Scopes: []auth.Permission{auth.PermBookingsRead}},
			{Name: "x"},
			{Name: "x", Scopes: []auth.Permission{auth.PermUsersManage}},
			{Name: "x", Scopes: []auth.Permission{"everything"}},
		} {
			if _, _, err := e.svc.Create(ctx, e.owner, e.estA, bad); !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("invalid %+v accepted: %v", bad, err)
			}
		}
	})
}
