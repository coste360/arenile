//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestMigration00005_ModelsRoundtrip inserts and reloads the M4 models on
// both dialects and checks the constraints the online flow relies on:
// unique token hashes, one active payment provider per establishment,
// external ids unique per provider, webhook events processed once, and
// message deduplication per establishment.
func TestMigration00005_ModelsRoundtrip(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-m4", "Lido M4", t0)
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

		tv := &store.TermsVersion{
			ID: store.NewID(), EstablishmentID: est.ID, Version: "2027-1",
			Texts:       map[string]store.TermsTexts{"it": {Terms: "Condizioni", Privacy: "Informativa"}},
			PublishedAt: t0, CreatedAt: t0,
		}
		insert(tv)
		expectConflict(&store.TermsVersion{
			ID: store.NewID(), EstablishmentID: est.ID, Version: "2027-1", Texts: map[string]store.TermsTexts{},
			PublishedAt: t0, CreatedAt: t0,
		})
		insert(&store.OnlineSettings{
			EstablishmentID: est.ID, Config: store.JSON(`{"formulas":["day"]}`), EmailConfig: store.JSON(`{}`),
			CurrentTermsVersionID: &tv.ID, Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		var os store.OnlineSettings
		if err := db.NewSelect().Model(&os).Where("establishment_id = ?", est.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if os.CurrentTermsVersionID == nil || *os.CurrentTermsVersionID != tv.ID {
			t.Fatalf("current terms version = %v", os.CurrentTermsVersionID)
		}

		pps := func(mode string, active bool) *store.PaymentProviderSettings {
			return &store.PaymentProviderSettings{
				ID: store.NewID(), EstablishmentID: est.ID, Provider: "stripe", Mode: mode, Active: active,
				Credentials: "v1.k.ciphertext", PublicConfig: map[string]any{"secret_key_last4": "4242"},
				Version: 1, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(pps(store.ProviderModeTest, true))
		expectConflict(pps(store.ProviderModeTest, false)) // same provider and mode
		live := pps(store.ProviderModeLive, true)
		live.Provider = "other"
		expectConflict(live) // a second active provider
		live.Active = false
		insert(live)

		email := "anna@example.it"
		cust := &store.Customer{
			ID: store.NewID(), EstablishmentID: est.ID, FirstName: "Anna", LastName: "Bianchi", Email: &email, Locale: "it",
			Tags: []string{}, SearchText: "bianchi anna", Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(cust)
		locale := "en"
		accepted := t0
		bk := &store.Booking{
			ID: store.NewID(), EstablishmentID: est.ID, Code: "LM-2027-000001", CustomerID: cust.ID, Channel: store.ChannelOnline,
			Status: "held", StartsOn: calendar.MustParse("2027-07-10"), EndsOn: calendar.MustParse("2027-07-10"),
			Slots: calendar.SlotsFull, Formula: "day", PriceSnapshot: store.JSON(`{"total":3000}`), TotalAmount: 3000,
			PolicySnapshot: store.JSON(`{"rules":{}}`), HoldExpiresAt: ptr(t0.Add(10 * time.Minute)),
			TermsVersion: &tv.Version, TermsAcceptedAt: &accepted, Locale: &locale,
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(bk)
		var got store.Booking
		if err := db.NewSelect().Model(&got).Where("id = ?", bk.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if got.Locale == nil || *got.Locale != "en" || got.TermsAcceptedAt == nil {
			t.Fatalf("booking roundtrip: locale %v, terms accepted %v", got.Locale, got.TermsAcceptedAt)
		}

		cs := func(hash string) *store.CheckoutSession {
			return &store.CheckoutSession{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: bk.ID, TokenHash: hash, State: store.CheckoutOpen,
				ExpiresAt: t0.Add(10 * time.Minute), Locale: "it", CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(cs("hash-1"))
		expectConflict(cs("hash-1"))

		ml := func(hash string) *store.MagicLinkToken {
			return &store.MagicLinkToken{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: bk.ID, TokenHash: hash,
				ExpiresAt: t0.AddDate(0, 1, 10), CreatedAt: t0,
			}
		}
		insert(ml("magic-1"))
		expectConflict(ml("magic-1"))

		key := "bk-attempt-1"
		pi := &store.PaymentIntent{
			ID: store.NewID(), EstablishmentID: est.ID, BookingID: &bk.ID, Provider: "stripe", Mode: store.ProviderModeTest,
			Kind: store.IntentKindPayment, ExternalID: "cs_test_1", Status: store.IntentPending, Amount: 3000, Currency: "EUR",
			Attempt: 1, IdempotencyKey: &key, RawEventIDs: []string{}, CreatedAt: t0, UpdatedAt: t0,
		}
		insert(pi)
		dup := *pi
		dup.ID, dup.IdempotencyKey = store.NewID(), nil
		expectConflict(&dup) // same provider external id

		we := func() *store.WebhookEvent {
			return &store.WebhookEvent{
				ID: store.NewID(), EstablishmentID: est.ID, Provider: "stripe", EventID: "evt_1",
				EventType: "checkout.session.completed", PaymentIntentID: &pi.ID, Status: store.WebhookEventProcessed,
				ReceivedAt: t0, ProcessedAt: &t0,
			}
		}
		insert(we())
		expectConflict(we())

		insert(&store.NotificationTemplate{
			ID: store.NewID(), EstablishmentID: est.ID, Template: "booking_confirmed", Locale: "it",
			Subject: "Prenotazione {{booking.code}}", BodyMarkdown: "Ciao {{customer.first_name}}", Enabled: true,
			Version: 1, CreatedAt: t0, UpdatedAt: t0,
		})
		msg := func() *store.Message {
			return &store.Message{
				ID: store.NewID(), EstablishmentID: est.ID, BookingID: &bk.ID, CustomerID: &cust.ID,
				Channel: store.MessageChannelEmail, Template: "booking_confirmed", Locale: "it", RecipientHash: "sha256",
				DedupeKey: "booking.confirmed:evt", Status: store.MessageQueued, NextAttemptAt: &t0,
				Payload: map[string]any{"booking_id": bk.ID}, CreatedAt: t0, UpdatedAt: t0,
			}
		}
		insert(msg())
		expectConflict(msg())
	})
}

func ptr[T any](v T) *T { return &v }
