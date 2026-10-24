//go:build integration

package online_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

// fakeProviders is a registry with the manual driver and an online
// "stripe" driver that verifies credentials.
type fakeProviders struct{}

func (fakeProviders) PaymentProviderCodes() []string { return []string{"manual", "stripe"} }
func (fakeProviders) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	switch code {
	case "stripe":
		return fakeDriver{code: code, online: true}, true
	case "manual":
		return fakeDriver{code: code}, true
	}
	return nil, false
}

type fakeDriver struct {
	code   string
	online bool
}

func (d fakeDriver) Code() string { return d.code }
func (d fakeDriver) Capabilities() ext.PaymentCapabilities {
	return ext.PaymentCapabilities{Online: d.online, Refunds: d.online}
}

func (fakeDriver) CreatePayment(context.Context, ext.CreatePaymentRequest) (*ext.CreatePaymentResult, error) {
	return nil, ext.ErrNotSupported
}

func (fakeDriver) ParseWebhook(context.Context, *http.Request, ext.Credentials) (*ext.PaymentEvent, error) {
	return nil, ext.ErrNotSupported
}

func (fakeDriver) Refund(context.Context, ext.RefundRequest) (*ext.RefundResult, error) {
	return nil, ext.ErrNotSupported
}

func (fakeDriver) VerifyCredentials(_ context.Context, creds ext.Credentials) (string, error) {
	if creds["secret_key"] == "" {
		return "", errors.New("no key")
	}
	return "Lido Test Account", nil
}

func newService(t *testing.T, f *bookingtest.Fixture) *online.Service {
	t.Helper()
	keys, err := crypto.NewKeyRing(make([]byte, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return online.NewService(online.Deps{
		DB: f.DB, Clock: f.Clock, Audit: audit.NewWriter(f.Clock), Outbox: event.NewOutbox(f.Clock), Keys: keys,
		Providers: fakeProviders{}, BaseURL: "https://prenota.example.org/", Transport: online.Transport{Kind: "log", MailFrom: "noreply@example.org"},
	})
}

func TestOnlineSettings_UpdateVersionedAndPublic(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		svc := newService(t, f)
		ctx, est := f.Ctx, f.Est.ID

		s, err := svc.Get(ctx, f.Viewer, est)
		if err != nil || s.Version != 1 || s.PageURL != "https://prenota.example.org/b/lido-delfino" || len(s.AllowedEmbedOrigins) != 0 {
			t.Fatalf("defaults: %v %+v", err, s)
		}
		if _, err := svc.Get(ctx, f.Outsider, est); err == nil {
			t.Fatal("other organization reads the settings")
		}
		in := online.Input{
			BookingEnabled: true, AllowedEmbedOrigins: []string{"https://www.lido-delfino.it/"},
			Flow: online.Config{Formulas: []string{"day"}, PromoCodesEnabled: true, RequirePhone: true},
		}
		if _, err := svc.Update(ctx, f.Cashier, est, nil, in); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier update: %v", err)
		}
		v1 := int64(1)
		s, err = svc.Update(ctx, f.Manager, est, &v1, in)
		if err != nil || s.Version != 2 || s.AllowedEmbedOrigins[0] != "https://www.lido-delfino.it" || !s.Flow.RequirePhone {
			t.Fatalf("update: %v %+v", err, s)
		}
		var vc *configerr.VersionConflictError
		if _, err := svc.Update(ctx, f.Manager, est, &v1, in); !errors.As(err, &vc) || vc.Current != 2 {
			t.Fatalf("stale If-Match: %v", err)
		}
		e, _ := store.GetEstablishmentBySlug(ctx, db, "lido-delfino")
		if !e.BookingEnabled || len(e.AllowedEmbedOrigins) != 1 {
			t.Fatalf("establishment columns %+v", e)
		}
		if n := f.Count((*store.OutboxEvent)(nil), "type = ?", event.ConfigOnlineChanged); n != 1 {
			t.Fatalf("outbox events %d", n)
		}

		// Terms: immutable versions, the current one is public.
		tv, err := svc.PublishTerms(ctx, f.Manager, est, online.TermsInput{
			Version: "2027.1", Texts: map[string]store.TermsTexts{"it": {Terms: "Condizioni"}}, MakeCurrent: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PublishTerms(ctx, f.Manager, est, online.TermsInput{Version: "2027.1", Texts: map[string]store.TermsTexts{"it": {Terms: "x"}}}); !errors.Is(err, online.ErrTermsVersionExists) {
			t.Fatalf("duplicate version: %v", err)
		}
		items, current, err := svc.ListTerms(ctx, f.Viewer, est)
		if err != nil || len(items) != 1 || current != tv.ID {
			t.Fatalf("list terms %v %d %s", err, len(items), current)
		}
		pe, err := svc.ResolvePublic(ctx, "LIDO-DELFINO")
		if err != nil || pe.Terms == nil || pe.Terms.Version != "2027.1" || pe.Provider != "manual" || pe.Online || !pe.Flow.RequirePhone {
			t.Fatalf("resolve public %v %+v", err, pe)
		}
		if _, err := svc.ResolvePublic(ctx, "lido-altro"); !errors.Is(err, online.ErrBookingDisabled) {
			t.Fatalf("disabled establishment: %v", err)
		}
		if _, err := svc.ResolvePublic(ctx, "nope"); !errors.Is(err, online.ErrEstablishmentNotFound) {
			t.Fatalf("unknown slug: %v", err)
		}

		// Email settings share the version.
		es, err := svc.UpdateEmail(ctx, f.Manager, est, nil, online.EmailConfig{SenderName: "Lido Delfino", ReplyTo: "info@lido.it"})
		if err != nil || es.Version != 4 || es.Transport.Kind != "log" {
			t.Fatalf("email %v %+v", err, es)
		}
	})
}

func TestOnlinePayments_EncryptedCredentials(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		svc := newService(t, f)
		ctx, est := f.Ctx, f.Est.ID
		if _, err := svc.Update(ctx, f.Owner, est, nil, online.Input{BookingEnabled: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdatePayments(ctx, f.Owner, est, nil, online.PaymentSettingsInput{Provider: "paypal"}); !errors.Is(err, online.ErrUnknownProvider) {
			t.Fatalf("unknown provider: %v", err)
		}
		sk, wh, pk := "sk_test_51abcdefghijklmnopqrstuv9876", "whsec_abcdefghijklmn", "pk_test_51abcdefghijklmnop"
		if _, err := svc.UpdatePayments(ctx, f.Owner, est, nil, online.PaymentSettingsInput{Provider: "stripe", Stripe: &online.StripeInput{SecretKey: &sk}}); err == nil {
			t.Fatal("stripe activated without webhook secret")
		}
		ps, err := svc.UpdatePayments(ctx, f.Owner, est, nil, online.PaymentSettingsInput{
			Provider: "stripe", Mode: "test", Stripe: &online.StripeInput{SecretKey: &sk, WebhookSecret: &wh, PublishableKey: &pk},
		})
		if err != nil {
			t.Fatal(err)
		}
		if ps.Provider != "stripe" || !ps.Stripe.Configured || ps.Stripe.SecretKeyLast4 != "9876" || ps.WebhookURL != "https://prenota.example.org/webhooks/payments/stripe/"+est {
			t.Fatalf("view %+v", ps)
		}
		row, err := store.GetActivePaymentProvider(ctx, db, mustScope(t, f))
		if err != nil || row.Credentials == "" || containsAny(row.Credentials, sk, wh) {
			t.Fatalf("credentials stored in clear or missing: %v", err)
		}
		cfg, err := svc.ActiveProvider(ctx, db, mustScope(t, f), false)
		if err != nil || cfg.Credentials["secret_key"] != sk || cfg.Credentials["webhook_secret"] != wh || !cfg.Online {
			t.Fatalf("decrypted %v %+v", err, cfg.Code)
		}
		// A partial update keeps the stored secrets.
		emb := true
		if _, err := svc.UpdatePayments(ctx, f.Owner, est, nil, online.PaymentSettingsInput{Provider: "stripe", Mode: "test", Stripe: &online.StripeInput{EmbeddedMode: &emb}}); err != nil {
			t.Fatal(err)
		}
		if cfg, _ = svc.ActiveProvider(ctx, db, mustScope(t, f), false); cfg.Credentials["secret_key"] != sk || !cfg.Embedded {
			t.Fatalf("secret lost on partial update %+v", cfg.Embedded)
		}
		res, err := svc.TestPayments(ctx, f.Owner, est)
		if err != nil || !res.OK || res.AccountLabel != "Lido Test Account" {
			t.Fatalf("test %v %+v", err, res)
		}
		pe, err := svc.ResolvePublic(ctx, "lido-delfino")
		// Test keys with test mode off: real customers cannot pay (INV-88).
		if err != nil || pe.Provider != "stripe" || pe.Online || !pe.LivePaymentsBlocked || !pe.TestMode || !pe.Embedded || pe.PublishableKey != pk {
			t.Fatalf("public payment %v %+v", err, pe)
		}
		if ps, err := svc.GetPayments(ctx, f.Owner, est); err != nil || !ps.LivePaymentsBlocked {
			t.Fatalf("payment settings must report the block: %v %+v", err, ps)
		}
		// Back to manual: webhooks of stripe still verify.
		if _, err := svc.UpdatePayments(ctx, f.Owner, est, nil, online.PaymentSettingsInput{Provider: "manual"}); err != nil {
			t.Fatal(err)
		}
		if cfg, _ = svc.ActiveProvider(ctx, db, mustScope(t, f), false); cfg.Code != "manual" {
			t.Fatalf("active %s", cfg.Code)
		}
		if ps, err := svc.GetPayments(ctx, f.Owner, est); err != nil || ps.LivePaymentsBlocked {
			t.Fatalf("manual is never blocked: %v %+v", err, ps)
		}
		if cfg, err = svc.ProviderConfig(ctx, db, mustScope(t, f), "stripe", store.ProviderModeTest); err != nil || cfg.Credentials["webhook_secret"] != wh {
			t.Fatalf("inactive provider config %v", err)
		}
		// Audit never carries the secrets.
		var entries []store.AuditEntry
		if err := db.NewSelect().Model(&entries).Where("action = ?", "payments.settings_updated").Scan(ctx); err != nil || len(entries) != 3 {
			t.Fatalf("audit %v %d", err, len(entries))
		}
		for _, e := range entries {
			if containsAny(string(e.Diff), sk, wh) {
				t.Fatal("secret in audit diff")
			}
		}
		if _, err := svc.GetPayments(ctx, f.Cashier, est); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("cashier reads payment settings: %v", err)
		}
	})
}

func mustScope(t *testing.T, f *bookingtest.Fixture) store.EstablishmentScope {
	t.Helper()
	es, err := store.SystemEstablishmentScope(f.Ctx, f.DB, f.Est.ID)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if x != "" && strings.Contains(s, x) {
			return true
		}
	}
	return false
}
