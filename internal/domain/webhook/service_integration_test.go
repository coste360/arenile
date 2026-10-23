//go:build integration

package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/domain/webhook"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// receiver is a webhook receiver that verifies signatures with the secrets
// it knows, like a real integration would.
type receiver struct {
	mu      sync.Mutex
	srv     *httptest.Server
	secrets []string
	status  int
	got     []received
}

type received struct {
	typ      string
	verified bool
	body     []byte
}

func newReceiver(t *testing.T, c clock.Clock) *receiver {
	r := &receiver{status: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		defer r.mu.Unlock()
		ok := false
		for _, s := range r.secrets {
			if event.VerifySignature(s, req.Header.Get(event.SignatureHeader), b, c.Now(), 5*time.Minute) == nil {
				ok = true
			}
		}
		r.got = append(r.got, received{typ: req.Header.Get("Arenile-Event-Type"), verified: ok, body: b})
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) last(t *testing.T) received {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.got) == 0 {
		t.Fatal("nothing received")
	}
	return r.got[len(r.got)-1]
}

type env struct {
	db     *store.DB
	c      *clock.Fake
	svc    *webhook.Service
	del    *event.Deliverer
	owner  *auth.Principal
	ownerB *auth.Principal
	key    *auth.Principal
	estA   string
	estB   string
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{9}, 32))
	aw := audit.NewWriter(c)
	orgA, estA, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org A", "lido-a", "Lido A", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	orgB, estB, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org B", "lido-b", "Lido B", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Receivers run on 127.0.0.1: this is the LAN configuration.
	lan := event.EgressPolicy{AllowPrivate: true}
	del := event.NewDeliverer(db, c, kr, nil, aw, nil, event.DeliveryOptions{Egress: lan, DisableAfter: 3})
	return &env{
		db: db, c: c, del: del,
		svc:    webhook.NewService(webhook.Deps{DB: db, Clock: c, Audit: aw, Keys: kr, Deliverer: del, Egress: lan}),
		owner:  &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgA.ID, Role: auth.RoleOwner}}},
		ownerB: &auth.Principal{Kind: auth.PrincipalUser, UserID: store.NewID(), Grants: []auth.Grant{{OrganizationID: orgB.ID, Role: auth.RoleOwner}}},
		key: &auth.Principal{
			Kind: auth.PrincipalAPIKey, APIKeyID: store.NewID(), APIKeyEstablishmentID: estA.ID, APIKeyOrganizationID: orgA.ID,
			Scopes: []auth.Permission{auth.PermBookingsRead, auth.PermEstablishmentConfigure},
		},
		estA: estA.ID, estB: estB.ID,
	}
}

func (e *env) emit(t *testing.T, typ string) {
	t.Helper()
	ob := event.NewOutbox(e.c)
	if err := store.RunInTx(context.Background(), e.db, func(ctx context.Context, tx bun.Tx) error {
		_, err := ob.Write(ctx, tx, typ, e.estA, 1, map[string]string{"booking_id": "b1"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := event.NewDispatcher(e.db, e.c, nil, nil, nil).DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func actions(t *testing.T, db *store.DB, entityID string) string {
	t.Helper()
	var out []string
	if err := db.NewSelect().Model((*store.AuditEntry)(nil)).Column("action").Where("entity_id = ?", entityID).OrderExpr("id ASC").Scan(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	return strings.Join(out, ",")
}

func TestWebhookService_Flow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		rcv := newReceiver(t, e.c)

		ep, secret, err := e.svc.Create(ctx, e.owner, e.estA, webhook.CreateInput{
			URL: rcv.srv.URL + "/hook", Description: " Gestionale ", Events: []string{"booking.*", "booking.created"}, Active: true, IP: "1.2.3.4",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(secret, "whsec_") || ep.Status != webhook.EndpointActive || ep.Description != "Gestionale" || len(ep.Events) != 2 {
			t.Fatalf("created %+v %q", ep, secret)
		}
		rcv.secrets = []string{secret}

		// Test event: synchronous, signed, only to this endpoint.
		d, err := e.svc.SendTest(ctx, e.owner, e.estA, ep.ID, "1.2.3.4")
		if err != nil {
			t.Fatal(err)
		}
		if d.Status != webhook.DeliveryDelivered || d.EventType != webhook.TestEventType || d.Attempts != 1 || *d.StatusCode != 200 {
			t.Fatalf("test delivery %+v", d)
		}
		if got := rcv.last(t); !got.verified || got.typ != webhook.TestEventType || !bytes.Contains(got.body, []byte(ep.ID)) {
			t.Fatalf("receiver %+v", got)
		}

		// A real event: matched by the filter, delivered by the job.
		e.emit(t, event.BookingConfirmed)
		e.emit(t, event.PaymentSucceeded) // not subscribed
		if n, err := e.del.DeliverDue(ctx, 10); err != nil || n != 1 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		if got := rcv.last(t); got.typ != event.BookingConfirmed || !got.verified {
			t.Fatalf("receiver %+v", got)
		}

		// Delivery log: newest first, filters, cursor, detail with envelope.
		items, next, err := e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{Limit: 1})
		if err != nil || len(items) != 1 || next == "" || items[0].EventType != event.BookingConfirmed {
			t.Fatalf("page 1: %+v %q %v", items, next, err)
		}
		items, next, err = e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{Limit: 1, Cursor: next})
		if err != nil || len(items) != 1 || next != "" || items[0].EventType != webhook.TestEventType {
			t.Fatalf("page 2: %+v %q %v", items, next, err)
		}
		if items, _, _ := e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{Status: webhook.DeliveryFailed}); len(items) != 0 {
			t.Fatalf("failed filter %+v", items)
		}
		if _, _, err := e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{Status: "lost"}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("bad status: %v", err)
		}
		detail, err := e.svc.GetDelivery(ctx, e.owner, e.estA, ep.ID, items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		var env map[string]any
		if err := json.Unmarshal(detail.RequestBody, &env); err != nil || env["type"] != webhook.TestEventType || env["id"] != detail.EventID {
			t.Fatalf("envelope %s %v", detail.RequestBody, err)
		}

		// Redeliver a delivered delivery: due again, sent again by the job.
		rd, err := e.svc.Redeliver(ctx, e.owner, e.estA, ep.ID, items[0].ID, "1.2.3.4")
		if err != nil || rd.Status != webhook.DeliveryPending {
			t.Fatalf("redeliver %+v %v", rd, err)
		}
		if _, err := e.svc.Redeliver(ctx, e.owner, e.estA, ep.ID, items[0].ID, ""); !errors.Is(err, webhook.ErrDeliveryPending) {
			t.Fatalf("redeliver pending: %v", err)
		}
		if n, _ := e.del.DeliverDue(ctx, 10); n != 1 {
			t.Fatalf("redelivery not sent: %d", n)
		}

		// Rotation with grace: both secrets verify, the old one only in grace.
		ep2, newSecret, err := e.svc.RotateSecret(ctx, e.owner, e.estA, ep.ID, 2*time.Hour, "1.2.3.4")
		if err != nil || newSecret == secret || ep2.PreviousSecretExpiresAt == nil {
			t.Fatalf("rotate %+v %v", ep2, err)
		}
		rcv.secrets = []string{secret} // receiver not updated yet
		if d, err := e.svc.SendTest(ctx, e.owner, e.estA, ep.ID, ""); err != nil || d.Status != webhook.DeliveryDelivered || !rcv.last(t).verified {
			t.Fatalf("old secret during grace: %+v %v", d, err)
		}
		e.c.Advance(3 * time.Hour)
		_, _ = e.svc.SendTest(ctx, e.owner, e.estA, ep.ID, "")
		if rcv.last(t).verified {
			t.Fatal("old secret verifies after the grace period")
		}
		rcv.secrets = []string{newSecret}
		_, _ = e.svc.SendTest(ctx, e.owner, e.estA, ep.ID, "")
		if !rcv.last(t).verified {
			t.Fatal("new secret does not verify")
		}
		if _, _, err := e.svc.RotateSecret(ctx, e.owner, e.estA, ep.ID, 169*time.Hour, ""); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("grace too long: %v", err)
		}

		// Auto-disable after failures, then re-activation resets it.
		rcv.status = http.StatusInternalServerError
		for i := 0; i < 3; i++ {
			e.emit(t, event.BookingCreated)
			if n, _ := e.del.DeliverDue(ctx, 10); n < 1 {
				t.Fatalf("attempt %d not made", i)
			}
		}
		got, _ := e.svc.Get(ctx, e.owner, e.estA, ep.ID)
		if got.Status != webhook.EndpointDisabled || got.DisabledReason == "" {
			t.Fatalf("not disabled: %+v", got)
		}
		failed, _, _ := e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{Status: webhook.DeliveryPending})
		if len(failed) == 0 {
			t.Fatal("pending retries expected")
		}
		if _, err := e.svc.Redeliver(ctx, e.owner, e.estA, ep.ID, items[0].ID, ""); !errors.Is(err, webhook.ErrEndpointInactive) {
			t.Fatalf("redeliver to disabled endpoint: %v", err)
		}
		active := true
		got, err = e.svc.Update(ctx, e.owner, e.estA, ep.ID, webhook.UpdateInput{Active: &active})
		if err != nil || got.Status != webhook.EndpointActive || got.ConsecutiveFailures != 0 || got.DisabledAt != nil {
			t.Fatalf("reactivate %+v %v", got, err)
		}
		paused := false
		if got, _ = e.svc.Update(ctx, e.owner, e.estA, ep.ID, webhook.UpdateInput{Active: &paused}); got.Status != webhook.EndpointPaused {
			t.Fatalf("pause %+v", got)
		}

		// Delete removes the endpoint and its log.
		if err := e.svc.Delete(ctx, e.owner, e.estA, ep.ID, "1.2.3.4"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Get(ctx, e.owner, e.estA, ep.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("deleted: %v", err)
		}
		var n int
		if err := db.NewSelect().TableExpr("webhook_deliveries").ColumnExpr("count(*)").Where("endpoint_id = ?", ep.ID).Scan(ctx, &n); err != nil || n != 0 {
			t.Fatalf("deliveries left: %d %v", n, err)
		}
		want := "webhook.created,webhook.tested,webhook.redelivered,webhook.secret_rotated,webhook.tested,webhook.tested,webhook.tested,webhook.disabled,webhook.updated,webhook.updated,webhook.deleted"
		if got := actions(t, db, ep.ID); got != want {
			t.Fatalf("audit\n got %s\nwant %s", got, want)
		}
		// Secrets never reach the audit log.
		var diffs []string
		_ = db.NewSelect().Model((*store.AuditEntry)(nil)).Column("diff").Where("entity_id = ?", ep.ID).Scan(ctx, &diffs)
		for _, d := range diffs {
			if strings.Contains(d, "whsec_") {
				t.Fatalf("secret in audit diff: %s", d)
			}
		}
	})
}

func TestWebhookService_PermissionsAndValidation(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		in := webhook.CreateInput{URL: "https://hooks.example.com/a", Events: []string{"*"}, Active: true}
		if _, _, err := e.svc.Create(ctx, e.key, e.estA, in); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("api key: %v", err)
		}
		if _, _, err := e.svc.Create(ctx, e.ownerB, e.estA, in); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("cross tenant: %v", err)
		}
		ep, _, err := e.svc.Create(ctx, e.owner, e.estA, in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.List(ctx, e.key, e.estA); !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("api key list: %v", err)
		}
		if _, err := e.svc.Get(ctx, e.ownerB, e.estB, ep.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign endpoint via own establishment: %v", err)
		}
		if _, _, err := e.svc.ListDeliveries(ctx, e.ownerB, e.estB, ep.ID, webhook.DeliveryFilter{}); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign delivery log: %v", err)
		}
		if _, err := e.svc.SendTest(ctx, e.ownerB, e.estB, ep.ID, ""); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign test: %v", err)
		}
		for _, bad := range []webhook.CreateInput{
			{URL: "http://hooks.example.com/a", Events: []string{"*"}},
			{URL: "https://hooks.example.com/a", Events: nil},
			{URL: "https://hooks.example.com/a", Events: []string{"booking.lost"}},
			{URL: "https://169.254.169.254/latest/meta-data", Events: []string{"*"}},
			{URL: "https://user:pw@hooks.example.com/a", Events: []string{"*"}},
			{URL: "https://hooks.example.com/a", Events: []string{"*"}, Description: strings.Repeat("x", 201)},
		} {
			if _, _, err := e.svc.Create(ctx, e.owner, e.estA, bad); !errors.Is(err, configerr.ErrValidation) {
				t.Fatalf("invalid %+v accepted: %v", bad, err)
			}
		}
		badURL := "ftp://x"
		if _, err := e.svc.Update(ctx, e.owner, e.estA, ep.ID, webhook.UpdateInput{URL: &badURL}); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("invalid update: %v", err)
		}
	})
}

func TestWebhookService_PurgeHistory(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newEnv(t, db)
		rcv := newReceiver(t, e.c)
		ep, _, err := e.svc.Create(ctx, e.owner, e.estA, webhook.CreateInput{URL: rcv.srv.URL, Events: []string{"*"}, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.SendTest(ctx, e.owner, e.estA, ep.ID, ""); err != nil {
			t.Fatal(err)
		}
		e.emit(t, event.BookingCreated) // stays pending (not delivered)
		e.c.Advance(webhook.DefaultRetention + time.Hour)
		n, err := e.svc.PurgeHistory(ctx)
		if err != nil || n != 1 {
			t.Fatalf("purged %d %v", n, err)
		}
		items, _, _ := e.svc.ListDeliveries(ctx, e.owner, e.estA, ep.ID, webhook.DeliveryFilter{})
		if len(items) != 1 || items[0].Status != webhook.DeliveryPending {
			t.Fatalf("pending delivery must survive: %+v", items)
		}
		var tests int
		if err := db.NewSelect().TableExpr("outbox_events").ColumnExpr("count(*)").Where("type = ?", webhook.TestEventType).Scan(ctx, &tests); err != nil || tests != 0 {
			t.Fatalf("test events left: %d %v", tests, err)
		}
	})
}
