//go:build integration

package webhooks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/online"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/payment/stripe"
	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/webhooks"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

var t0 = time.Date(2027, 7, 15, 8, 0, 0, 0, time.UTC)

type settings struct{}

func (settings) ActiveProvider(context.Context, bun.IDB, store.EstablishmentScope, bool) (*online.ProviderConfig, error) {
	return nil, store.ErrNotFound
}

func (settings) ProviderConfig(_ context.Context, _ bun.IDB, es store.EstablishmentScope, provider, mode string) (*online.ProviderConfig, error) {
	if provider != stripe.Code || mode == store.ProviderModeLive {
		return nil, store.ErrNotFound
	}
	return &online.ProviderConfig{
		EstablishmentID: es.ID(), Code: stripe.Code, Mode: "test", Online: true,
		Credentials: ext.Credentials{stripe.CredSecretKey: stripetest.SecretKey, stripe.CredWebhookSecret: stripetest.WebhookSecret},
	}, nil
}

type providers struct{ d ext.PaymentProvider }

func (p providers) PaymentProvider(code string) (ext.PaymentProvider, bool) {
	if code == stripe.Code {
		return p.d, true
	}
	return nil, false
}

// TestReceivePaymentWebhook_HTTP: the endpoint answers 200 with the ack
// (duplicate flag on redelivery), 400 webhook_signature_invalid for a bad
// signature, 404 for unknown targets, with the raw body bounded by the handler.
func TestReceivePaymentWebhook_HTTP(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido-w", "Lido W", t0)
		if err != nil {
			t.Fatal(err)
		}
		c := clock.NewFake(t0)
		gw := gateway.NewService(gateway.Deps{
			DB: db, Clock: c, Audit: audit.NewWriter(c), Outbox: event.NewOutbox(c),
			Providers: providers{stripe.New(stripe.Options{Clock: c})}, Settings: settings{},
		})
		h := &webhooks.Handlers{Gateway: gw}
		r := chi.NewRouter()
		r.Post("/webhooks/payments/{provider}/{establishmentId}", func(w http.ResponseWriter, req *http.Request) {
			h.ReceivePaymentWebhook(w, req, chi.URLParam(req, "provider"), chi.URLParam(req, "establishmentId"))
		})

		// An authentic event for a reference Arenile does not know (no
		// Arenile metadata): acknowledged and recorded as ignored.
		payload := stripetest.Event("evt_http_1", "payment_intent.succeeded", t0, map[string]any{
			"id": "pi_foreign", "object": "payment_intent", "amount": 100, "currency": "eur", "status": "succeeded", "metadata": map[string]string{},
		})
		url := "/webhooks/payments/stripe/" + est.ID
		send := func(req *http.Request) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w
		}
		w := send(stripetest.WebhookRequest(url, payload, t0, stripetest.WebhookSecret))
		var ack api.WebhookAck
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &ack) != nil || !ack.Received || ack.Duplicate != nil {
			t.Fatalf("first delivery: %d %s", w.Code, w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache control")
		}
		w = send(stripetest.WebhookRequest(url, payload, t0, stripetest.WebhookSecret))
		ack = api.WebhookAck{}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &ack) != nil || ack.Duplicate == nil || !*ack.Duplicate {
			t.Fatalf("redelivery: %d %s", w.Code, w.Body)
		}
		w = send(stripetest.WebhookRequest(url, payload, t0, "whsec_wrong"))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "webhook_signature_invalid") || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/problem+json") {
			t.Fatalf("bad signature: %d %s", w.Code, w.Body)
		}
		w = send(stripetest.WebhookRequest("/webhooks/payments/stripe/"+store.NewID(), payload, t0, stripetest.WebhookSecret))
		if w.Code != http.StatusNotFound {
			t.Fatalf("unknown establishment: %d %s", w.Code, w.Body)
		}
		// Body above the limit: refused before verification.
		big := append([]byte(`{"x":"`), make([]byte, gateway.MaxWebhookBody)...)
		w = send(stripetest.WebhookRequest(url, big, t0, stripetest.WebhookSecret))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("oversized body: %d", w.Code)
		}
		n, err := db.NewSelect().Model((*store.WebhookEvent)(nil)).Where("status = ?", store.WebhookEventIgnored).Count(ctx)
		if err != nil || n != 1 {
			t.Fatalf("webhook events: %d %v", n, err)
		}
	})
}
