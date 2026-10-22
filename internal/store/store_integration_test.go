//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

var t0 = time.Date(2027, 7, 1, 9, 12, 44, 0, time.UTC)

func TestMigrations_UpAndStatus(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		r, err := db.Migrator()
		if err != nil {
			t.Fatal(err)
		}
		// Already migrated by storetest: a second Up is a no-op.
		applied, err := r.Up(ctx)
		if err != nil || len(applied) != 0 {
			t.Fatalf("second up: %v %v", applied, err)
		}
		ok, err := r.Aligned(ctx)
		if err != nil || !ok {
			t.Fatalf("aligned: %v %v", ok, err)
		}
		st, err := r.Status(ctx)
		if err != nil || len(st) == 0 || !st[0].Applied {
			t.Fatalf("status: %+v %v", st, err)
		}
		for _, table := range []string{
			"organizations", "establishments", "users", "memberships", "sessions", "api_keys",
			"audit_log", "outbox_events", "webhook_endpoints", "webhook_deliveries", "idempotency_keys",
		} {
			var n int
			if err := db.NewSelect().TableExpr(table).ColumnExpr("count(*)").Scan(ctx, &n); err != nil {
				t.Fatalf("table %s: %v", table, err)
			}
		}
	})
}

func TestEstablishments_RoundtripAndCrossTenant(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", t0)
		if err != nil {
			t.Fatal(err)
		}
		_, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", t0)
		if err != nil {
			t.Fatal(err)
		}
		// Second establishment of org A.
		estA2 := store.NewEstablishmentDefaults(orgA.ID, "lido-a2", "Lido A2", t0)
		estA2.BookingEnabled = true
		estA2.EnabledLocales = []string{"it", "en"}
		estA2.AllowedEmbedOrigins = []string{"https://www.lido-a.it"}
		show := true
		estA2.Theme = store.Theme{PrimaryColor: "#0E6BA8", ShowPricesOnMap: &show, CustomTexts: map[string]map[string]string{"it": {"intro": "Ciao"}}}
		estA2.Address = store.Address{City: "Gaeta", Country: "IT"}
		if err := store.InsertEstablishment(ctx, db, estA2); err != nil {
			t.Fatal(err)
		}

		// Duplicate slug is a conflict.
		dup := store.NewEstablishmentDefaults(orgA.ID, "lido-a", "Dup", t0)
		if err := store.InsertEstablishment(ctx, db, dup); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate slug: %v", err)
		}

		scopeOnlyA := store.NewScope(nil, []string{estA.ID})
		scopeOrgA := store.NewScope([]string{orgA.ID}, nil)

		// Repository-level isolation: A cannot resolve B.
		if _, err := store.ResolveEstablishment(ctx, db, scopeOnlyA, estB.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant resolve must be not found, got %v", err)
		}
		if _, err := store.ResolveEstablishment(ctx, db, scopeOrgA, estB.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("org scope cross tenant: %v", err)
		}
		if _, err := store.ResolveEstablishment(ctx, db, store.Scope{}, estA.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("empty scope: %v", err)
		}
		es, err := store.ResolveEstablishment(ctx, db, scopeOrgA, estA2.ID)
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.GetEstablishment(ctx, db, es)
		if err != nil {
			t.Fatal(err)
		}
		if !got.BookingEnabled || len(got.EnabledLocales) != 2 || got.Theme.PrimaryColor != "#0E6BA8" ||
			got.Theme.ShowPricesOnMap == nil || !*got.Theme.ShowPricesOnMap || got.Address.City != "Gaeta" ||
			got.Theme.CustomTexts["it"]["intro"] != "Ciao" || !got.CreatedAt.Equal(t0) {
			t.Fatalf("roundtrip mismatch: %+v", got)
		}
		// Zero-value scope is rejected.
		if _, err := store.GetEstablishment(ctx, db, store.EstablishmentScope{}); err == nil {
			t.Fatal("zero scope must be rejected")
		}

		// Listing is scoped.
		page, err := store.ListEstablishments(ctx, db, scopeOnlyA, "", 10)
		if err != nil || len(page.Items) != 1 || page.Items[0].ID != estA.ID {
			t.Fatalf("list only A: %+v %v", page, err)
		}
		page, err = store.ListEstablishments(ctx, db, scopeOrgA, "", 1)
		if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatalf("list org A page 1: %+v %v", page, err)
		}
		page2, err := store.ListEstablishments(ctx, db, scopeOrgA, page.NextCursor, 1)
		if err != nil || len(page2.Items) != 1 || page2.NextCursor != "" || page2.Items[0].ID == page.Items[0].ID {
			t.Fatalf("list org A page 2: %+v %v", page2, err)
		}
		for _, it := range append(page.Items, page2.Items...) {
			if it.OrganizationID != orgA.ID {
				t.Fatalf("leaked establishment %s", it.ID)
			}
		}
		if _, err := store.ListEstablishments(ctx, db, scopeOrgA, "!!", 1); !errors.Is(err, store.ErrInvalidCursor) {
			t.Fatalf("bad cursor: %v", err)
		}

		// Update in scope.
		got.Name = "Lido A2 bis"
		got.UpdatedAt = t0.Add(time.Hour)
		if err := store.UpdateEstablishment(ctx, db, es, got); err != nil {
			t.Fatal(err)
		}
		bySlug, err := store.GetEstablishmentBySlug(ctx, db, "LIDO-A2")
		if err != nil || bySlug.Name != "Lido A2 bis" {
			t.Fatalf("by slug: %+v %v", bySlug, err)
		}
	})
}

func TestUsersSessionsAPIKeys_Roundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		org, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", t0)
		if err != nil {
			t.Fatal(err)
		}
		u := &store.User{ID: store.NewID(), Email: " Mario@Example.IT ", PasswordHash: "x", Name: "Mario", Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertUser(ctx, db, u); err != nil {
			t.Fatal(err)
		}
		dupe := *u
		dupe.ID = store.NewID()
		if err := store.InsertUser(ctx, db, &dupe); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate email: %v", err)
		}
		got, err := store.GetUserByEmail(ctx, db, "MARIO@example.it")
		if err != nil || got.ID != u.ID || !got.Active {
			t.Fatalf("by email: %+v %v", got, err)
		}
		m := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: org.ID, Role: "owner", CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertMembership(ctx, db, m); err != nil {
			t.Fatal(err)
		}
		m2 := *m
		m2.ID = store.NewID()
		if err := store.InsertMembership(ctx, db, &m2); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate org-wide membership must conflict: %v", err)
		}
		m3 := &store.Membership{ID: store.NewID(), UserID: u.ID, OrganizationID: org.ID, EstablishmentID: &est.ID, Role: "cashier", CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertMembership(ctx, db, m3); err != nil {
			t.Fatal(err)
		}
		ms, err := store.ListUserMemberships(ctx, db, u.ID)
		if err != nil || len(ms) != 2 {
			t.Fatalf("memberships: %v %v", ms, err)
		}

		// Sessions.
		s := &store.Session{ID: store.NewID(), TokenHash: "h1", UserID: u.ID, ExpiresAt: t0.Add(time.Hour), LastSeenAt: t0, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertSession(ctx, db, s); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetSessionByTokenHash(ctx, db, "h1", t0.Add(30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetSessionByTokenHash(ctx, db, "h1", t0.Add(2*time.Hour)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expired session: %v", err)
		}
		n, err := store.DeleteExpiredSessions(ctx, db, t0.Add(2*time.Hour))
		if err != nil || n != 1 {
			t.Fatalf("cleanup: %d %v", n, err)
		}

		// TOTP replay guard.
		ok, err := store.AdvanceUserTOTPStep(ctx, db, u.ID, 100, t0)
		if err != nil || !ok {
			t.Fatal("first step must be accepted")
		}
		ok, _ = store.AdvanceUserTOTPStep(ctx, db, u.ID, 100, t0)
		if ok {
			t.Fatal("replayed step must be rejected")
		}

		// API keys.
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		k := &store.APIKey{ID: store.NewID(), Name: "pos", Prefix: "abcd1234", KeyHash: "kh", Scopes: []string{"bookings.read"}, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertAPIKey(ctx, db, es, k); err != nil {
			t.Fatal(err)
		}
		gk, err := store.GetActiveAPIKeyByPrefix(ctx, db, "abcd1234")
		if err != nil || gk.EstablishmentID != est.ID || len(gk.Scopes) != 1 {
			t.Fatalf("api key: %+v %v", gk, err)
		}
		if err := store.RevokeAPIKey(ctx, db, es, k.ID, t0); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetActiveAPIKeyByPrefix(ctx, db, "abcd1234"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("revoked key: %v", err)
		}
	})
}

func TestAuditLog_InsertOnly(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", t0)
		if err != nil {
			t.Fatal(err)
		}
		diff, _ := store.MarshalJSONValue(map[string]any{"name": []string{"a", "b"}})
		e := &store.AuditEntry{ID: store.NewID(), EstablishmentID: &est.ID, ActorType: "system", Action: "establishment.updated", EntityType: "establishment", EntityID: &est.ID, Diff: diff, CreatedAt: t0}
		if err := store.InsertAuditEntry(ctx, db, e); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewUpdate().Model((*store.AuditEntry)(nil)).Set("action = 'x'").Where("id = ?", e.ID).Exec(ctx); err == nil {
			t.Fatal("audit update must be rejected")
		}
		if _, err := db.NewDelete().Model((*store.AuditEntry)(nil)).Where("id = ?", e.ID).Exec(ctx); err == nil {
			t.Fatal("audit delete must be rejected")
		}
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		page, err := store.ListAuditEntries(ctx, db, es, "", 10)
		if err != nil || len(page.Items) != 1 || page.Items[0].Action != "establishment.updated" {
			t.Fatalf("list audit: %+v %v", page, err)
		}
	})
}

func TestIdempotencyKeys_Claim(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		k := &store.IdempotencyKey{Scope: "user:1", Key: "k-12345678", RequestHash: "h", CreatedAt: t0, ExpiresAt: t0.Add(24 * time.Hour)}
		claimed, _, err := store.ClaimIdempotencyKey(ctx, db, k)
		if err != nil || !claimed {
			t.Fatalf("first claim: %v %v", claimed, err)
		}
		again := *k
		claimed, existing, err := store.ClaimIdempotencyKey(ctx, db, &again)
		if err != nil || claimed || existing.ResponseStatus != nil {
			t.Fatalf("second claim: %v %+v %v", claimed, existing, err)
		}
		if err := store.CompleteIdempotencyKey(ctx, db, k.Scope, k.Key, 201, "application/json", []byte(`{"ok":true}`)); err != nil {
			t.Fatal(err)
		}
		_, existing, err = store.ClaimIdempotencyKey(ctx, db, &again)
		if err != nil || existing.ResponseStatus == nil || *existing.ResponseStatus != 201 || string(existing.ResponseBody) != `{"ok":true}` {
			t.Fatalf("completed: %+v %v", existing, err)
		}
		n, err := store.DeleteExpiredIdempotencyKeys(ctx, db, t0.Add(25*time.Hour))
		if err != nil || n != 1 {
			t.Fatalf("expire: %d %v", n, err)
		}
	})
}
