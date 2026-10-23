//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00006_ModelsRoundtrip inserts and reloads the M6/M7 models on
// both dialects and checks the constraints the device and fiscal bridge
// services rely on: unique token and pairing code hashes, one active fiscal
// bridge per establishment, idempotent bridge jobs per (establishment, kind,
// idempotency key).
func TestMigration00006_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m6", "Lido M6", t0)
		if err != nil {
			t.Fatal(err)
		}
		insert := func(m any) {
			t.Helper()
			if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
				t.Fatalf("insert %T: %v", m, err)
			}
		}
		expectConflict := func(m any) {
			t.Helper()
			_, err := db.NewInsert().Model(m).Exec(ctx)
			if !store.IsUniqueViolation(err) {
				t.Fatalf("insert %T: want unique violation, got %v", m, err)
			}
		}
		u := &store.User{ID: store.NewID(), Email: "cassa@lido.it", PasswordHash: "x", Name: "Cassa", Locale: "it", Active: true, CreatedAt: t0, UpdatedAt: t0}
		insert(u)

		kind := store.DeviceKindMobile
		code := &store.DevicePairingCode{
			ID: store.NewID(), EstablishmentID: est.ID, UserID: u.ID, CreatedByUserID: &u.ID, Kind: &kind,
			CodeHash: "c0de", ExpiresAt: t0.Add(5 * time.Minute), CreatedAt: t0,
		}
		insert(code)
		expectConflict(&store.DevicePairingCode{
			ID: store.NewID(), EstablishmentID: est.ID, UserID: u.ID, CodeHash: "c0de", ExpiresAt: t0, CreatedAt: t0,
		})

		ver := "1.0.0"
		dev := &store.Device{
			ID: store.NewID(), EstablishmentID: est.ID, UserID: u.ID, Name: "iPhone di Anna",
			Platform: store.DevicePlatformIOS, Kind: store.DeviceKindMobile, AppVersion: &ver,
			TokenPrefix: "abcdefgh", TokenHash: "hash-1", PairingCodeID: &code.ID, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(dev)
		dup := *dev
		dup.ID = store.NewID()
		expectConflict(&dup) // same token hash

		var got store.Device
		if err := db.NewSelect().Model(&got).Where("token_hash = ?", "hash-1").Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if got.ID != dev.ID || got.AppVersion == nil || *got.AppVersion != ver || got.RevokedAt != nil {
			t.Fatalf("device roundtrip = %+v", got)
		}

		bridge := func(active bool) *store.FiscalBridge {
			return &store.FiscalBridge{
				ID: store.NewID(), EstablishmentID: est.ID, DeviceID: dev.ID, Driver: "none", Active: active,
				DeviceStatus: store.JSON(`{"online":true}`), CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(bridge(true))
		expectConflict(bridge(true)) // one active bridge per establishment
		insert(bridge(false))

		job := func(key string) *store.FiscalBridgeJob {
			return &store.FiscalBridgeJob{
				ID: store.NewID(), EstablishmentID: est.ID, Kind: store.FiscalBridgeJobIssue, IdempotencyKey: key,
				Status: store.FiscalBridgeJobQueued, Payload: store.JSON(`{"total":1220}`),
				NextAttemptAt: t0, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(job("pay-1"))
		expectConflict(job("pay-1"))
		insert(job("pay-2"))
		var jobs []store.FiscalBridgeJob
		if err := db.NewSelect().Model(&jobs).Where("establishment_id = ?", est.ID).Order("idempotency_key").Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 2 || jobs[0].Result != nil || jobs[0].Attempts != 0 {
			t.Fatalf("jobs = %+v", jobs)
		}
	})
}
