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

// TestMigration00013_PushNotifications checks migration 00013 on both
// dialects: settings with optimistic version, one registration per device
// and per (establishment, token hash), replacement on re-registration, the
// platform/environment/locale CHECKs, one delivery per (registration,
// event), the claim lease, cascades on device and registration deletion and
// the tenancy of the reads.
func TestMigration00013_PushNotifications(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-push", "Lido Push", t0)
		if err != nil {
			t.Fatal(err)
		}
		_, other, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-push-b", "Lido Push B", t0)
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

		// Settings: no row = defaults; conditional update on the version.
		if _, err := store.GetPushSettings(ctx, db, es); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("settings before insert: %v", err)
		}
		ps := &store.PushSettings{Enabled: true, NewOnlineBooking: true, Version: 1, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertPushSettings(ctx, db, es, ps); err != nil {
			t.Fatal(err)
		}
		if err := store.InsertPushSettings(ctx, db, es, &store.PushSettings{Version: 1, CreatedAt: t0, UpdatedAt: t0}); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("second settings row: %v", err)
		}
		ps.PaymentFailed, ps.Version = true, 2
		if ok, err := store.UpdatePushSettings(ctx, db, es, ps, 1); err != nil || !ok {
			t.Fatalf("update: %v %v", ok, err)
		}
		if ok, _ := store.UpdatePushSettings(ctx, db, es, ps, 1); ok {
			t.Fatal("stale version applied")
		}
		got, err := store.GetPushSettings(ctx, db, es)
		if err != nil || !got.Enabled || !got.NewOnlineBooking || got.ApprovalRequested || !got.PaymentFailed || got.Version != 2 {
			t.Fatalf("settings %+v %v", got, err)
		}
		if _, err := store.GetPushSettings(ctx, db, oes); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("settings of another establishment: %v", err)
		}

		u := &store.User{ID: store.NewID(), Email: "spiaggia@lido.it", PasswordHash: "x", Name: "Spiaggia", Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		if _, err := db.NewInsert().Model(u).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		device := func(prefix string) *store.Device {
			d := &store.Device{
				ID: store.NewID(), UserID: u.ID, Name: "Telefono", Platform: store.DevicePlatformIOS, Kind: store.DeviceKindMobile,
				TokenPrefix: prefix, TokenHash: "hash-" + prefix, CreatedAt: t0, UpdatedAt: t0,
			}
			if err := store.InsertDevice(ctx, db, es, d); err != nil {
				t.Fatal(err)
			}
			return d
		}
		d1, d2 := device("p1"), device("p2")
		reg := func(d *store.Device, tokenHash string) *store.PushRegistration {
			return &store.PushRegistration{
				ID: store.NewID(), EstablishmentID: est.ID, DeviceID: d.ID, UserID: u.ID, Platform: "ios", Environment: store.PushEnvironmentProduction,
				Locale: "it", Topics: []string{store.PushTopicNewOnlineBooking}, TokenCiphertext: "v1:k:x", TokenHash: tokenHash,
				CreatedAt: t0, UpdatedAt: t0,
			}
		}
		r1 := reg(d1, "token-a")
		if err := store.ReplacePushRegistration(ctx, db, es, r1); err != nil {
			t.Fatal(err)
		}
		// Re-registration of the device replaces its row.
		r1b := reg(d1, "token-b")
		if err := store.ReplacePushRegistration(ctx, db, es, r1b); err != nil {
			t.Fatal(err)
		}
		// The same token on another device (phone paired again) moves there.
		r2 := reg(d2, "token-b")
		if err := store.ReplacePushRegistration(ctx, db, es, r2); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetPushRegistrationByDevice(ctx, db, es, d1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("old device registration still there: %v", err)
		}
		recipients, err := store.ListPushRecipients(ctx, db, es)
		if err != nil || len(recipients) != 1 || recipients[0].ID != r2.ID || !recipients[0].UserActive ||
			recipients[0].DeviceRevokedAt != nil || len(recipients[0].Topics) != 1 {
			t.Fatalf("recipients %+v %v", recipients, err)
		}
		if rs, err := store.ListPushRecipients(ctx, db, oes); err != nil || len(rs) != 0 {
			t.Fatalf("recipients of another establishment: %v %v", rs, err)
		}
		// Direct duplicates violate the unique keys.
		if _, err := db.NewInsert().Model(reg(d2, "token-c")).Exec(ctx); !store.IsUniqueViolation(err) {
			t.Fatalf("second registration of a device: %v", err)
		}
		bad := reg(d1, "token-d")
		bad.Platform = "windows"
		if _, err := db.NewInsert().Model(bad).Exec(ctx); err == nil {
			t.Fatal("desktop platform accepted")
		}
		bad = reg(d1, "token-d")
		bad.Locale = "es"
		if _, err := db.NewInsert().Model(bad).Exec(ctx); err == nil {
			t.Fatal("unknown locale accepted")
		}

		// Deliveries: one per (registration, event); claim with a lease.
		now := t0.Add(time.Minute)
		ev := store.NewID()
		del := func() *store.PushDelivery {
			return &store.PushDelivery{
				ID: store.NewID(), RegistrationID: r2.ID, EventID: ev, MessageKey: store.PushTopicNewOnlineBooking,
				Status: store.PushDeliveryQueued, NextAttemptAt: &now, CreatedAt: now, UpdatedAt: now,
			}
		}
		first := del()
		if ok, err := store.InsertPushDelivery(ctx, db, es, first); err != nil || !ok {
			t.Fatalf("insert delivery: %v %v", ok, err)
		}
		if ok, err := store.InsertPushDelivery(ctx, db, es, del()); err != nil || ok {
			t.Fatalf("duplicate delivery: %v %v", ok, err)
		}
		badKey := del()
		badKey.EventID, badKey.MessageKey = store.NewID(), "Nuova prenotazione di Rossi"
		if _, err := store.InsertPushDelivery(ctx, db, es, badKey); err == nil {
			t.Fatal("free text message key accepted")
		}
		due, err := store.DuePushDeliveries(ctx, db, now, 10)
		if err != nil || len(due) != 1 || due[0].ID != first.ID || due[0].EstablishmentID != est.ID {
			t.Fatalf("due %v %v", due, err)
		}
		lease := now.Add(time.Minute)
		if _, err := store.ClaimPushDelivery(ctx, db, first.ID, now, lease); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ClaimPushDelivery(ctx, db, first.ID, now, lease); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("second claim: %v", err)
		}
		if ok, err := store.FinishPushDeliverySent(ctx, db, first.ID, now, now); err != nil || ok {
			t.Fatalf("finish with a foreign lease: %v %v", ok, err)
		}
		if ok, err := store.FinishPushDeliverySent(ctx, db, first.ID, lease, now); err != nil || !ok {
			t.Fatalf("finish: %v %v", ok, err)
		}
		if n, err := store.DeleteClosedPushDeliveries(ctx, db, now.Add(time.Hour)); err != nil || n != 1 {
			t.Fatalf("retention: %d %v", n, err)
		}

		// Deleting the device removes its registration and deliveries.
		if ok, err := store.InsertPushDelivery(ctx, db, es, del()); err != nil || !ok {
			t.Fatal(err)
		}
		if _, err := db.NewDelete().Model((*store.Device)(nil)).Where("id = ?", d2.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if rs, _ := store.ListPushRecipients(ctx, db, es); len(rs) != 0 {
			t.Fatalf("registration survived its device: %v", rs)
		}
		if ds, _ := store.ListPushDeliveries(ctx, db, es); len(ds) != 0 {
			t.Fatalf("deliveries survived their registration: %v", ds)
		}
	})
}
