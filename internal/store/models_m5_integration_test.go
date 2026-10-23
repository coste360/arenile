//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00007_ModelsRoundtrip inserts and reloads the columns added by
// migration 00007 on both dialects: API key expiry and the panel fields of
// webhook endpoints (description, rotated secret with grace period, last
// delivery), and checks that deliveries still cascade with their endpoint.
func TestMigration00007_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m5", "Lido M5", t0)
		if err != nil {
			t.Fatal(err)
		}
		es, err := store.SystemEstablishmentScope(ctx, db, est.ID)
		if err != nil {
			t.Fatal(err)
		}

		exp := t0.Add(90 * 24 * time.Hour)
		key := &store.APIKey{
			ID: store.NewID(), Name: "Gestionale bar", Prefix: "abcd2345", KeyHash: "hash-m5",
			Scopes: []string{"bookings.read"}, ExpiresAt: &exp, CreatedAt: t0, UpdatedAt: t0,
		}
		if err := store.InsertAPIKey(ctx, db, es, key); err != nil {
			t.Fatal(err)
		}
		var gotKey store.APIKey
		if err := db.NewSelect().Model(&gotKey).Where("id = ?", key.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if gotKey.ExpiresAt == nil || !gotKey.ExpiresAt.Equal(exp) {
			t.Fatalf("expires_at = %v, want %v", gotKey.ExpiresAt, exp)
		}

		prev := "ciphertext-previous"
		grace := t0.Add(24 * time.Hour)
		ep := &store.WebhookEndpoint{
			ID: store.NewID(), URL: "https://example.test/hook", Secret: "ciphertext", Events: []string{"booking.*"},
			Active: true, Description: "CRM", PreviousSecret: &prev, PreviousSecretExpiresAt: &grace,
			LastDeliveryAt: &t0, CreatedAt: t0, UpdatedAt: t0,
		}
		if err := store.InsertWebhookEndpoint(ctx, db, es, ep); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetWebhookEndpoint(ctx, db, es, ep.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Description != "CRM" || got.PreviousSecret == nil || *got.PreviousSecret != prev ||
			got.PreviousSecretExpiresAt == nil || !got.PreviousSecretExpiresAt.Equal(grace) ||
			got.LastDeliveryAt == nil || !got.LastDeliveryAt.Equal(t0) {
			t.Fatalf("endpoint roundtrip = %+v", got)
		}

		evID := store.NewID()
		if err := store.InsertOutboxEvent(ctx, db, &store.OutboxEvent{
			ID: evID, EstablishmentID: &est.ID, Type: "webhook.test", Payload: store.JSON(`{}`), Version: 1,
			OccurredAt: t0, CreatedAt: t0, DispatchedAt: &t0,
		}); err != nil {
			t.Fatal(err)
		}
		del := &store.WebhookDelivery{ID: store.NewID(), EndpointID: ep.ID, EventID: evID, NextAttemptAt: &t0, CreatedAt: t0, UpdatedAt: t0}
		if err := store.InsertWebhookDeliveryIfAbsent(ctx, db, del); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewDelete().Model((*store.WebhookEndpoint)(nil)).Where("id = ?", ep.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		n, err := db.NewSelect().Model((*store.WebhookDelivery)(nil)).Where("endpoint_id = ?", ep.ID).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("deliveries after endpoint delete = %d, want 0 (cascade)", n)
		}
	})
}
