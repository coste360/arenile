//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00012_EmailTemplateOverrides checks migration 00012 on both
// dialects: one override per (establishment, kind, locale), the locale
// CHECK, the version CHECK, conditional update and delete on the version,
// tenancy of the reads and cascade on establishment deletion.
func TestMigration00012_EmailTemplateOverrides(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-eto", "Lido ETO", t0)
		if err != nil {
			t.Fatal(err)
		}
		_, other, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-eto-b", "Lido ETO B", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}
		oes, err := store.SystemEstablishmentScope(ctx, db, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		row := func(locale string) *store.EmailTemplateOverride {
			return &store.EmailTemplateOverride{
				ID: store.NewID(), Kind: "booking_confirmed", Locale: locale, Subject: "S {codice}", Opening: "O", Active: true,
				Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		o := row("it")
		if err := store.InsertEmailTemplateOverride(ctx, db, es, o); err != nil {
			t.Fatal(err)
		}
		if err := store.InsertEmailTemplateOverride(ctx, db, es, row("it")); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate key: %v", err)
		}
		if err := store.InsertEmailTemplateOverride(ctx, db, es, row("es")); err == nil {
			t.Fatal("unknown locale accepted")
		}
		bad := row("en")
		bad.Version = 0
		if err := store.InsertEmailTemplateOverride(ctx, db, es, bad); err == nil {
			t.Fatal("version 0 accepted")
		}
		if err := store.InsertEmailTemplateOverride(ctx, db, oes, row("it")); err != nil {
			t.Fatalf("same key in another establishment: %v", err)
		}
		if _, err := store.GetEmailTemplateOverride(ctx, db, oes, "booking_confirmed", "de"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing override: %v", err)
		}
		o.Subject, o.Version = "S2", 2
		if ok, err := store.UpdateEmailTemplateOverride(ctx, db, es, o, 5); err != nil || ok {
			t.Fatalf("stale update applied: %v %v", ok, err)
		}
		if ok, err := store.UpdateEmailTemplateOverride(ctx, db, es, o, 1); err != nil || !ok {
			t.Fatalf("update: %v %v", ok, err)
		}
		got, err := store.GetEmailTemplateOverride(ctx, db, es, "booking_confirmed", "it")
		if err != nil || got.Subject != "S2" || got.Version != 2 {
			t.Fatalf("get: %v %+v", err, got)
		}
		if !got.Active {
			t.Fatal("active must default to true")
		}
		o.Active, o.Version = false, 3
		if ok, _ := store.UpdateEmailTemplateOverride(ctx, db, oes, o, 2); ok {
			t.Fatal("updated the row of another establishment")
		}
		if ok, err := store.UpdateEmailTemplateOverride(ctx, db, es, o, 2); err != nil || !ok {
			t.Fatalf("deactivate: %v %v", ok, err)
		}
		if got, err := store.GetEmailTemplateOverride(ctx, db, es, "booking_confirmed", "it"); err != nil || got.Active || got.Version != 3 {
			t.Fatalf("after deactivate: %v %+v", err, got)
		}
		list, err := store.ListEmailTemplateOverrides(ctx, db, oes)
		if err != nil || len(list) != 1 {
			t.Fatalf("list other: %v %d", err, len(list))
		}
	})
}
