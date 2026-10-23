//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/apikey"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/webhook"
	httpserver "github.com/coste360/arenile/internal/http"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// intEnv runs the full HTTP stack with the M5A integration services (API
// keys, webhook endpoints) and a per-key budget of 6 requests per minute.
type intEnv struct {
	t    *testing.T
	srv  *httptest.Server
	c    *clock.Fake
	estA string
	estB string
}

func newIntEnv(t *testing.T, db *store.DB) *intEnv {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Now().UTC())
	aw := audit.NewWriter(c)
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{4}, 32))
	as, err := auth.NewService(db, c, kr, aw, nil, auth.Options{Argon2: &stFastArgon})
	if err != nil {
		t.Fatal(err)
	}
	res, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-a.it", Name: "Owner A", Password: stPassword, OrganizationName: "Org A"})
	if err != nil {
		t.Fatal(err)
	}
	estA := store.NewEstablishmentDefaults(res.Organization.ID, "lido-a", "Lido A", c.Now())
	if err := store.InsertEstablishment(ctx, db, estA); err != nil {
		t.Fatal(err)
	}
	resB, err := as.CreateOwner(ctx, auth.CreateOwnerInput{Email: "owner@lido-b.it", Name: "Owner B", Password: stPassword, OrganizationName: "Org B"})
	if err != nil {
		t.Fatal(err)
	}
	estB := store.NewEstablishmentDefaults(resB.Organization.ID, "lido-b", "Lido B", c.Now())
	if err := store.InsertEstablishment(ctx, db, estB); err != nil {
		t.Fatal(err)
	}
	hash, _ := as.Hasher().Hash(stPassword)
	mgr := &store.User{ID: store.NewID(), Email: "manager@lido-a.it", PasswordHash: hash, Name: "Manager", Locale: "it", Active: true, CreatedAt: c.Now(), UpdatedAt: c.Now()}
	if err := store.InsertUser(ctx, db, mgr); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertMembership(ctx, db, &store.Membership{ID: store.NewID(), UserID: mgr.ID, OrganizationID: res.Organization.ID, Role: "manager", CreatedAt: c.Now(), UpdatedAt: c.Now()}); err != nil {
		t.Fatal(err)
	}
	lan := event.EgressPolicy{AllowPrivate: true}
	del := event.NewDeliverer(db, c, kr, nil, aw, nil, event.DeliveryOptions{Egress: lan})
	h := httpserver.New(httpserver.Deps{
		DB: db, Clock: c, Version: "test", Auth: as, Bus: event.NewBroker(nil, 16), Readiness: stReadiness{db},
		Establishments:          establishment.NewService(db),
		APIKeys:                 apikey.NewService(apikey.Deps{DB: db, Clock: c, Audit: aw}),
		Webhooks:                webhook.NewService(webhook.Deps{DB: db, Clock: c, Audit: aw, Keys: kr, Deliverer: del, Egress: lan}),
		APIKeyRequestsPerMinute: 6,
	})
	e := &intEnv{t: t, srv: httptest.NewServer(h), c: c, estA: estA.ID, estB: estB.ID}
	t.Cleanup(e.srv.Close)
	return e
}

func (e *intEnv) as(email string) *stClient {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &stClient{t: e.t, base: e.srv.URL, http: &http.Client{Jar: jar}}
	if resp, body := c.do(http.MethodPost, "/api/admin/v1/auth/login", map[string]string{"email": email, "password": stPassword}, nil); resp.StatusCode != http.StatusOK {
		e.t.Fatalf("login %s: %d %v", email, resp.StatusCode, body)
	}
	return c
}

// bearer is a client authenticated only with an API key (no cookies).
func (e *intEnv) bearer(key string) *stClient {
	jar, _ := cookiejar.New(nil)
	return &stClient{t: e.t, base: e.srv.URL, http: &http.Client{Jar: jar, Transport: bearerTransport{key: key}}}
}

type bearerTransport struct{ key string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.key)
	return http.DefaultTransport.RoundTrip(r)
}

func TestIntegrationsHTTP_APIKeys(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newIntEnv(t, db)
		owner, manager, ownerB := e.as("owner@lido-a.it"), e.as("manager@lido-a.it"), e.as("owner@lido-b.it")
		base := "/api/admin/v1/establishments/" + e.estA + "/api-keys"

		resp, body := manager.do(http.MethodPost, base, map[string]any{"name": "x", "scopes": []string{"bookings.read"}}, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = owner.do(http.MethodPost, base, map[string]any{"name": "x", "scopes": []string{"users.manage"}}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = owner.do(http.MethodPost, base, map[string]any{"name": "Gestionale", "scopes": []string{"bookings.read", "reports.read"}}, nil)
		stExpect(t, resp, body, 201, "")
		secret, _ := body["secret"].(string)
		key := body["api_key"].(map[string]any)
		if !strings.HasPrefix(secret, "ar_live_") || resp.Header.Get("Cache-Control") != "no-store" || key["status"] != "active" ||
			!strings.HasPrefix(secret, key["prefix"].(string)+"_") || key["created_by_name"] != "Owner A" {
			t.Fatalf("created %v %v", resp.Header, body)
		}
		keyID := key["id"].(string)
		resp, body = ownerB.do(http.MethodGet, base+"/"+keyID, nil, nil)
		stExpect(t, resp, body, 404, "not_found")

		// The key works on what its scopes allow, on its establishment only.
		k := e.bearer(secret)
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 200, "")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estB, nil, nil)
		stExpect(t, resp, body, 404, "not_found")
		// ...and never on key or webhook management, even with users.manage
		// impossible to grant: always 403.
		resp, body = k.do(http.MethodGet, base, nil, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA+"/webhook-endpoints", nil, nil)
		stExpect(t, resp, body, 403, "forbidden")
		// Per-key rate limit (6/min in this env): 4 calls made so far.
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 200, "")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 200, "")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 429, "rate_limited")
		if resp.Header.Get("Retry-After") == "" {
			t.Fatal("Retry-After missing")
		}
		e.c.Advance(time.Minute)

		// Expiry in the past is refused; an expiry reached locks the key out.
		resp, body = owner.do(http.MethodPatch, base+"/"+keyID, map[string]any{"expires_at": e.c.Now().Add(-time.Hour)}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = owner.do(http.MethodPatch, base+"/"+keyID, map[string]any{"expires_at": e.c.Now().Add(time.Hour), "name": "Gestionale 2"}, nil)
		stExpect(t, resp, body, 200, "")
		if body["name"] != "Gestionale 2" || body["expires_at"] == nil {
			t.Fatalf("patched %v", body)
		}
		e.c.Advance(time.Hour)
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 401, "unauthorized")
		resp, body = owner.do(http.MethodGet, base+"/"+keyID, nil, nil)
		stExpect(t, resp, body, 200, "")
		if body["status"] != "expired" {
			t.Fatalf("status %v", body)
		}
		resp, body = owner.do(http.MethodPatch, base+"/"+keyID, map[string]any{"clear_expiry": true}, nil)
		stExpect(t, resp, body, 200, "")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 200, "")

		// Revoke: 204, idempotent, hidden from the default list.
		resp, body = owner.do(http.MethodDelete, base+"/"+keyID, nil, nil)
		stExpect(t, resp, body, 204, "")
		resp, body = owner.do(http.MethodDelete, base+"/"+keyID, nil, nil)
		stExpect(t, resp, body, 204, "")
		resp, body = k.do(http.MethodGet, "/api/admin/v1/establishments/"+e.estA, nil, nil)
		stExpect(t, resp, body, 401, "unauthorized")
		resp, body = owner.do(http.MethodGet, base, nil, nil)
		stExpect(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 0 {
			t.Fatalf("revoked listed by default: %v", body)
		}
		resp, body = owner.do(http.MethodGet, base+"?include_revoked=true", nil, nil)
		stExpect(t, resp, body, 200, "")
		if items := body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["status"] != "revoked" {
			t.Fatalf("include_revoked %v", body)
		}
		resp, body = owner.do(http.MethodPatch, base+"/"+keyID, map[string]any{"name": "x"}, nil)
		stExpect(t, resp, body, 409, "conflict")
	})
}

func TestIntegrationsHTTP_WebhookEndpoints(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newIntEnv(t, db)
		owner, manager := e.as("owner@lido-a.it"), e.as("manager@lido-a.it")
		var hits atomic.Int32
		rcv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer rcv.Close()
		base := "/api/admin/v1/establishments/" + e.estA + "/webhook-endpoints"

		resp, body := manager.do(http.MethodGet, base, nil, nil)
		stExpect(t, resp, body, 403, "forbidden")
		resp, body = owner.do(http.MethodPost, base, map[string]any{"url": "http://hooks.example.com/x", "events": []string{"*"}}, nil)
		stExpect(t, resp, body, 400, "validation_failed")
		resp, body = owner.do(http.MethodPost, base, map[string]any{"url": rcv.URL + "/hook", "events": []string{"booking.*"}, "description": "CRM", "active": false}, nil)
		stExpect(t, resp, body, 201, "")
		ep := body["endpoint"].(map[string]any)
		if !strings.HasPrefix(body["secret"].(string), "whsec_") || resp.Header.Get("Cache-Control") != "no-store" || ep["status"] != "paused" {
			t.Fatalf("created %v %v", resp.Header, body)
		}
		id := ep["id"].(string)
		resp, body = owner.do(http.MethodGet, base, nil, nil)
		stExpect(t, resp, body, 200, "")
		if items := body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["secret"] != nil {
			t.Fatalf("list %v", body)
		}

		// Test works on a paused endpoint.
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/test", nil, nil)
		stExpect(t, resp, body, 200, "")
		if body["status"] != "delivered" || body["event_type"] != "webhook.test" || hits.Load() != 1 {
			t.Fatalf("test %v hits=%d", body, hits.Load())
		}
		deliveryID := body["id"].(string)
		resp, body = owner.do(http.MethodGet, base+"/"+id+"/deliveries?status=delivered", nil, nil)
		stExpect(t, resp, body, 200, "")
		if len(body["items"].([]any)) != 1 {
			t.Fatalf("deliveries %v", body)
		}
		resp, body = owner.do(http.MethodGet, base+"/"+id+"/deliveries/"+deliveryID, nil, nil)
		stExpect(t, resp, body, 200, "")
		if rb := body["request_body"].(map[string]any); rb["type"] != "webhook.test" || body["response_excerpt"] != nil && body["response_excerpt"] != "" {
			t.Fatalf("detail %v", body)
		}
		// Redelivery needs an active endpoint.
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/deliveries/"+deliveryID+"/redeliver", nil, nil)
		stExpect(t, resp, body, 409, "conflict")
		resp, body = owner.do(http.MethodPatch, base+"/"+id, map[string]any{"active": true}, nil)
		stExpect(t, resp, body, 200, "")
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/deliveries/"+deliveryID+"/redeliver", nil, nil)
		stExpect(t, resp, body, 202, "")
		if body["status"] != "pending" {
			t.Fatalf("redeliver %v", body)
		}

		// Rotation: empty body = default grace; secret shown once.
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/rotate-secret", nil, nil)
		stExpect(t, resp, body, 200, "")
		if resp.Header.Get("Cache-Control") != "no-store" || body["endpoint"].(map[string]any)["previous_secret_expires_at"] == nil {
			t.Fatalf("rotate %v %v", resp.Header, body)
		}
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/rotate-secret", map[string]any{"grace_hours": 0}, nil)
		stExpect(t, resp, body, 200, "")
		if body["endpoint"].(map[string]any)["previous_secret_expires_at"] != nil {
			t.Fatalf("grace 0 %v", body)
		}
		resp, body = owner.do(http.MethodPost, base+"/"+id+"/rotate-secret", map[string]any{"grace_hours": 500}, nil)
		stExpect(t, resp, body, 400, "validation_failed")

		resp, body = owner.do(http.MethodDelete, base+"/"+id, nil, nil)
		stExpect(t, resp, body, 204, "")
		resp, body = owner.do(http.MethodGet, base+"/"+id, nil, nil)
		stExpect(t, resp, body, 404, "not_found")
	})
}
