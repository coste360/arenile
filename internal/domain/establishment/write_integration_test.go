//go:build integration

package establishment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type fixture struct {
	svc     *establishment.Service
	orgA    *store.Organization
	estA    *store.Establishment
	estB    *store.Establishment
	owner   *auth.Principal
	manager *auth.Principal
	cashier *auth.Principal
	ownerB  *auth.Principal
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC))
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	user := func(id, org string, role auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: id, Grants: []auth.Grant{{OrganizationID: org, Role: role}}}
	}
	return &fixture{
		svc: establishment.NewService(db, establishment.WithWriteDeps(establishment.WriteDeps{
			Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c),
		})),
		orgA: orgA, estA: estA, estB: estB,
		owner: user("u-owner", orgA.ID, auth.RoleOwner), manager: user("u-manager", orgA.ID, auth.RoleManager),
		cashier: user("u-cashier", orgA.ID, auth.RoleCashier), ownerB: user("u-owner-b", orgB.ID, auth.RoleOwner),
	}
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

func TestEstablishmentService_Create(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		e, err := f.svc.Create(ctx, f.owner, establishment.CreateInput{Slug: "Lido-Due", Name: " Lido Due ", IP: "203.0.113.1"})
		if err != nil {
			t.Fatal(err)
		}
		if e.Slug != "lido-due" || e.Name != "Lido Due" || e.OrganizationID != f.orgA.ID || e.Timezone != "Europe/Rome" ||
			e.Currency != "EUR" || e.DefaultLocale != "it" || len(e.EnabledLocales) != 1 || e.Version != 1 || e.BookingEnabled {
			t.Fatalf("defaults %+v", e)
		}
		s, err := establishment.ParseSettings(e.Settings, e.Currency)
		if err != nil || s.Pricing.HalfDayFactor.String() != "0.5" || s.DefaultPolicy.MaxLeadTimeDays == nil {
			t.Fatalf("settings %s %v", e.Settings, err)
		}
		// The organization owner (and every org-wide member) sees it at once.
		if _, err := f.svc.Get(ctx, f.owner, e.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Get(ctx, f.manager, e.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Get(ctx, f.ownerB, e.ID); !errors.Is(err, establishment.ErrNotFound) {
			t.Fatalf("other org: %v", err)
		}
		if count(t, db, "audit_log", "action = ? AND establishment_id = ?", establishment.ActionCreated, e.ID) != 1 {
			t.Fatal("audit missing")
		}
		// Slug taken (also by another organization's establishment).
		if _, err := f.svc.Create(ctx, f.owner, establishment.CreateInput{Slug: "lido-b", Name: "X"}); !errors.Is(err, establishment.ErrSlugTaken) {
			t.Fatalf("taken: %v", err)
		}
		// Reserved slug, bad locale and timezone.
		_, err = f.svc.Create(ctx, f.owner, establishment.CreateInput{Slug: "admin", Name: "X", Timezone: "Mars/Base", EnabledLocales: []string{"es"}})
		var ve *configerr.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 4 {
			t.Fatalf("validation: %v", err)
		}
		// Managers cannot create establishments.
		if _, err := f.svc.Create(ctx, f.manager, establishment.CreateInput{Slug: "lido-tre", Name: "X"}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("manager: %v", err)
		}
		// Owners cannot create in an organization they do not own.
		if _, err := f.svc.Create(ctx, f.owner, establishment.CreateInput{OrganizationID: f.estB.OrganizationID, Slug: "lido-tre", Name: "X"}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("foreign org: %v", err)
		}
	})
}

func TestEstablishmentService_UpdateAndArchive(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		origins := []string{"https://www.lido-a.it/", "https://booking.lido-a.it"}
		e, err := f.svc.Update(ctx, f.manager, f.estA.ID, establishment.UpdateInput{
			Name: ptr("Lido A Mare"), AllowedEmbedOrigins: &origins, BookingEnabled: ptr(true),
			Theme: &store.Theme{PrimaryColor: "#0E6BA8", Radius: "md"}, IfMatch: ptr(int64(1)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if e.Version != 2 || e.Name != "Lido A Mare" || e.AllowedEmbedOrigins[0] != "https://www.lido-a.it" || e.Theme.PrimaryColor != "#0e6ba8" {
			t.Fatalf("updated %+v", e)
		}
		// No settings change: no pricing event.
		if n := count(t, db, "outbox_events", "establishment_id = ?", f.estA.ID); n != 0 {
			t.Fatalf("events %d", n)
		}
		// Stale If-Match.
		_, err = f.svc.Update(ctx, f.manager, f.estA.ID, establishment.UpdateInput{Name: ptr("X"), IfMatch: ptr(int64(1))})
		var vc *configerr.VersionConflictError
		if !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale: %v", err)
		}
		// Settings change emits config.pricing_changed.
		s := establishment.DefaultSettings()
		s.Pricing.DefaultVATRateBP = 2200
		s.Pricing.Rounding = pricing.RoundingEuro
		if e, err = f.svc.Update(ctx, f.manager, f.estA.ID, establishment.UpdateInput{Settings: &s}); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "outbox_events", "establishment_id = ? AND type = ?", f.estA.ID, event.ConfigPricingChanged); n != 1 {
			t.Fatalf("pricing events %d", n)
		}
		got, _ := establishment.ParseSettings(e.Settings, e.Currency)
		if got.Pricing.DefaultVATRateBP != 2200 || got.Pricing.Rounding != pricing.RoundingEuro {
			t.Fatalf("settings %s", e.Settings)
		}
		// Invalid settings.
		bad := establishment.DefaultSettings()
		bad.Pricing.HalfDayFactor = pricing.MustParseDecimal("1.5")
		if _, err := f.svc.Update(ctx, f.manager, f.estA.ID, establishment.UpdateInput{Settings: &bad}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("bad settings: %v", err)
		}
		// Slug change to a taken slug.
		if _, err := f.svc.Update(ctx, f.owner, f.estA.ID, establishment.UpdateInput{Slug: ptr("lido-b")}); !errors.Is(err, establishment.ErrSlugTaken) {
			t.Fatalf("slug: %v", err)
		}
		// Permission checks: cashier cannot configure, manager cannot archive,
		// another tenant cannot see it.
		if _, err := f.svc.Update(ctx, f.cashier, f.estA.ID, establishment.UpdateInput{Name: ptr("X")}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier: %v", err)
		}
		if _, err := f.svc.Update(ctx, f.manager, f.estA.ID, establishment.UpdateInput{Archived: ptr(true)}); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("manager archive: %v", err)
		}
		if _, err := f.svc.Update(ctx, f.ownerB, f.estA.ID, establishment.UpdateInput{Name: ptr("X")}); !errors.Is(err, establishment.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		// Archive and restore.
		if e, err = f.svc.Update(ctx, f.owner, f.estA.ID, establishment.UpdateInput{Archived: ptr(true)}); err != nil || e.ArchivedAt == nil {
			t.Fatalf("archive: %v", err)
		}
		page, err := f.svc.List(ctx, f.owner, "", 50)
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("archived establishment listed: %+v %v", page.Items, err)
		}
		if _, err := f.svc.GetPublic(ctx, "lido-a"); !errors.Is(err, establishment.ErrNotFound) {
			t.Fatalf("archived public page: %v", err)
		}
		if e, err = f.svc.Update(ctx, f.owner, f.estA.ID, establishment.UpdateInput{Archived: ptr(false)}); err != nil || e.ArchivedAt != nil {
			t.Fatalf("restore: %v", err)
		}
		for action, want := range map[string]int64{establishment.ActionUpdated: 2, establishment.ActionArchived: 1, establishment.ActionRestored: 1} {
			if n := count(t, db, "audit_log", "action = ? AND establishment_id = ?", action, f.estA.ID); n != want {
				t.Fatalf("%s: %d audit entries", action, n)
			}
		}
	})
}
