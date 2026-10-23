//go:build integration

package event_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

type m5Env struct {
	db  *store.DB
	c   *clock.Fake
	kr  *crypto.KeyRing
	es  store.EstablishmentScope
	est string
}

func newM5Env(t *testing.T, db *store.DB) *m5Env {
	t.Helper()
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
	if err != nil {
		t.Fatal(err)
	}
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{7}, 32))
	es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
	return &m5Env{db: db, c: c, kr: kr, es: es, est: est.ID}
}

func (e *m5Env) endpoint(t *testing.T, d *event.Deliverer, url string) (*store.WebhookEndpoint, string) {
	t.Helper()
	ep, secret, err := d.CreateEndpoint(context.Background(), e.db, e.es, event.CreateEndpointInput{URL: url})
	if err != nil {
		t.Fatal(err)
	}
	return ep, secret
}

func (e *m5Env) dispatch(t *testing.T, n int) {
	t.Helper()
	writeEvents(t, e.db, e.c, e.est, n)
	if got, err := event.NewDispatcher(e.db, e.c, nil, nil, nil).DispatchPending(context.Background()); err != nil || got != n {
		t.Fatalf("dispatch: %d %v", got, err)
	}
}

func onlyDelivery(t *testing.T, db *store.DB) store.WebhookDelivery {
	t.Helper()
	var ds []store.WebhookDelivery
	if err := db.NewSelect().Model(&ds).Scan(context.Background()); err != nil || len(ds) != 1 {
		t.Fatalf("deliveries: %d %v", len(ds), err)
	}
	return ds[0]
}

// TestDeliverer_SignsWithPreviousSecretDuringGrace: after a rotation with a
// grace period the request carries a v1 signature for the new and the old
// secret; once the grace period ends, only the new one.
func TestDeliverer_SignsWithPreviousSecretDuringGrace(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newM5Env(t, db)
		var headers []string
		var bodies [][]byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			headers, bodies = append(headers, r.Header.Get(event.SignatureHeader)), append(bodies, b)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		d := event.NewDeliverer(db, e.c, e.kr, srv.Client(), nil, nil, event.DeliveryOptions{})
		ep, oldSecret := e.endpoint(t, d, srv.URL)

		// Rotate by hand as webhook.Service.RotateSecret does.
		newSecret := "whsec_rotated"
		enc, _ := e.kr.Encrypt([]byte(newSecret), []byte(ep.ID))
		prev, _ := e.kr.Encrypt([]byte(oldSecret), []byte(event.PreviousSecretAAD(ep.ID)))
		until := e.c.Now().Add(time.Hour)
		ep.Secret, ep.PreviousSecret, ep.PreviousSecretExpiresAt = enc, &prev, &until
		if err := store.UpdateWebhookEndpoint(ctx, db, e.es, ep, "secret", "previous_secret", "previous_secret_expires_at"); err != nil {
			t.Fatal(err)
		}

		e.dispatch(t, 1)
		if n, err := d.DeliverDue(ctx, 10); err != nil || n != 1 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		if strings.Count(headers[0], "v1=") != 2 {
			t.Fatalf("grace period: want two signatures, got %q", headers[0])
		}
		for _, s := range []string{newSecret, oldSecret} {
			if err := event.VerifySignature(s, headers[0], bodies[0], e.c.Now(), 5*time.Minute); err != nil {
				t.Fatalf("verify with %s: %v", s, err)
			}
		}
		got, _ := store.GetWebhookEndpointByID(ctx, db, ep.ID)
		if got.LastDeliveryAt == nil || !got.LastDeliveryAt.Equal(e.c.Now()) {
			t.Fatalf("last_delivery_at = %v", got.LastDeliveryAt)
		}

		e.c.Advance(2 * time.Hour)
		e.dispatch(t, 1)
		if n, err := d.DeliverDue(ctx, 10); err != nil || n != 1 {
			t.Fatalf("deliver after grace: %d %v", n, err)
		}
		if strings.Count(headers[1], "v1=") != 1 {
			t.Fatalf("after grace: want one signature, got %q", headers[1])
		}
		if err := event.VerifySignature(oldSecret, headers[1], bodies[1], e.c.Now(), 5*time.Minute); err == nil {
			t.Fatal("the old secret must not verify after the grace period")
		}
		if err := event.VerifySignature(newSecret, headers[1], bodies[1], e.c.Now(), 5*time.Minute); err != nil {
			t.Fatal(err)
		}
	})
}

// TestDeliverer_RetryBackoffAndDeadLetter: a failing receiver is retried at
// 30s, 1m, 2m, 4m... (fake clock) and the delivery becomes failed (dead
// letter) once the next attempt would fall outside MaxAge.
func TestDeliverer_RetryBackoffAndDeadLetter(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newM5Env(t, db)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()
		d := event.NewDeliverer(db, e.c, e.kr, srv.Client(), nil, nil, event.DeliveryOptions{MaxAge: 10 * time.Minute, DisableAfter: 1000})
		e.endpoint(t, d, srv.URL)
		e.dispatch(t, 1)
		start := e.c.Now()

		// Attempt n happens at start + sum(Backoff(1..n-1)).
		wantGaps := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute}
		for i := 0; i <= len(wantGaps); i++ {
			if n, err := d.DeliverDue(ctx, 10); err != nil || n != 1 {
				t.Fatalf("attempt %d: n=%d err=%v", i+1, n, err)
			}
			if n, _ := d.DeliverDue(ctx, 10); n != 0 {
				t.Fatalf("attempt %d: retried before its backoff", i+1)
			}
			del := onlyDelivery(t, db)
			if del.Attempt != i+1 || del.StatusCode == nil || *del.StatusCode != 500 || del.ResponseExcerpt == nil {
				t.Fatalf("attempt %d row: %+v", i+1, del)
			}
			if i == len(wantGaps) {
				// 7m30s elapsed; the next gap (8m) would end after MaxAge.
				if del.FailedAt == nil || del.NextAttemptAt != nil {
					t.Fatalf("want dead letter after attempt %d: %+v", i+1, del)
				}
				break
			}
			if del.FailedAt != nil || del.NextAttemptAt == nil || !del.NextAttemptAt.Equal(e.c.Now().Add(wantGaps[i])) {
				t.Fatalf("attempt %d: next=%v want now+%s", i+1, del.NextAttemptAt, wantGaps[i])
			}
			e.c.Advance(wantGaps[i] - time.Second)
			if n, _ := d.DeliverDue(ctx, 10); n != 0 {
				t.Fatalf("attempt %d: retried 1s early", i+2)
			}
			e.c.Advance(time.Second)
		}
		if e.c.Now().Sub(start) != 7*time.Minute+30*time.Second || hits.Load() != 5 {
			t.Fatalf("elapsed %s hits %d", e.c.Now().Sub(start), hits.Load())
		}
		e.c.Advance(time.Hour)
		if n, _ := d.DeliverDue(ctx, 10); n != 0 {
			t.Fatal("a dead letter is never retried automatically")
		}
	})
}

// testDelivery writes a webhook.test event already dispatched and its
// delivery, as webhook.Service.SendTest does.
func testDelivery(t *testing.T, e *m5Env, endpointID string) string {
	t.Helper()
	now := e.c.Now()
	est := e.est
	row := &store.OutboxEvent{
		ID: store.NewID(), EstablishmentID: &est, Type: event.TestEventType, Payload: store.JSON(`{"message":"prova"}`),
		Version: 1, OccurredAt: now, CreatedAt: now, DispatchedAt: &now,
	}
	del := &store.WebhookDelivery{ID: store.NewID(), EndpointID: endpointID, EventID: row.ID, CreatedAt: now, UpdatedAt: now}
	if err := store.RunInTx(context.Background(), e.db, func(ctx context.Context, tx bun.Tx) error {
		if err := store.InsertOutboxEvent(ctx, tx, row); err != nil {
			return err
		}
		return store.InsertWebhookDelivery(ctx, tx, del)
	}); err != nil {
		t.Fatal(err)
	}
	return del.ID
}

// TestDeliverer_AttemptNowTestEvent: a test delivery is attempted once,
// synchronously, also on a paused endpoint; a failure is final (no retry)
// and does not count towards the automatic disable.
func TestDeliverer_AttemptNowTestEvent(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newM5Env(t, db)
		status := http.StatusOK
		var types []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			types = append(types, r.Header.Get("Arenile-Event-Type"))
			w.WriteHeader(status)
		}))
		defer srv.Close()
		d := event.NewDeliverer(db, e.c, e.kr, srv.Client(), nil, nil, event.DeliveryOptions{DisableAfter: 1})
		ep, _ := e.endpoint(t, d, srv.URL)
		ep.Active = false
		if err := store.UpdateWebhookEndpoint(ctx, db, e.es, ep, "active"); err != nil {
			t.Fatal(err)
		}

		ok := testDelivery(t, e, ep.ID)
		if err := d.AttemptNow(ctx, ok); err != nil {
			t.Fatal(err)
		}
		got, _ := store.GetWebhookDelivery(ctx, db, ok)
		if got.DeliveredAt == nil || got.Attempt != 1 || len(types) != 1 || types[0] != event.TestEventType {
			t.Fatalf("test on paused endpoint: %+v %v", got, types)
		}
		if err := d.AttemptNow(ctx, ok); !errors.Is(err, event.ErrDeliveryNotPending) {
			t.Fatalf("second attempt: %v", err)
		}

		status = http.StatusBadGateway
		ko := testDelivery(t, e, ep.ID)
		if err := d.AttemptNow(ctx, ko); err != nil {
			t.Fatal(err)
		}
		got, _ = store.GetWebhookDelivery(ctx, db, ko)
		if got.FailedAt == nil || got.NextAttemptAt != nil || got.LastError == nil || *got.LastError != "HTTP 502" {
			t.Fatalf("failed test: %+v", got)
		}
		after, _ := store.GetWebhookEndpointByID(ctx, db, ep.ID)
		if after.ConsecutiveFailures != 0 || after.DisabledAt != nil || after.LastDeliveryAt == nil {
			t.Fatalf("a test must not count as a failure: %+v", after)
		}
		e.c.Advance(time.Hour)
		if n, _ := d.DeliverDue(ctx, 10); n != 0 {
			t.Fatal("a failed test is never retried")
		}
	})
}

// TestDeliverer_PayloadTooLarge: an envelope above MaxPayload is failed
// without any HTTP request.
func TestDeliverer_PayloadTooLarge(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newM5Env(t, db)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
		defer srv.Close()
		d := event.NewDeliverer(db, e.c, e.kr, srv.Client(), nil, nil, event.DeliveryOptions{MaxPayload: 16})
		e.endpoint(t, d, srv.URL)
		e.dispatch(t, 1)
		if n, err := d.DeliverDue(context.Background(), 10); err != nil || n != 1 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		del := onlyDelivery(t, db)
		if hits.Load() != 0 || del.FailedAt == nil || del.LastError == nil || !strings.HasPrefix(*del.LastError, "payload too large") {
			t.Fatalf("hits=%d %+v", hits.Load(), del)
		}
	})
}

// TestDeliverer_DefaultClientRefusesLoopback: with the production client
// (nil client, strict egress policy) a receiver on 127.0.0.1 is never
// contacted; the attempt is recorded as failed with the reason.
func TestDeliverer_DefaultClientRefusesLoopback(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		e := newM5Env(t, db)
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
		defer srv.Close()
		d := event.NewDeliverer(db, e.c, e.kr, nil, nil, nil, event.DeliveryOptions{DisableAfter: 1000})
		e.endpoint(t, d, srv.URL)
		e.dispatch(t, 1)
		if n, err := d.DeliverDue(context.Background(), 10); err != nil || n != 1 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		del := onlyDelivery(t, db)
		if hits.Load() != 0 || del.LastError == nil || !strings.Contains(*del.LastError, "not allowed") || strings.Contains(*del.LastError, srv.URL) {
			t.Fatalf("hits=%d err=%v", hits.Load(), del.LastError)
		}
	})
}

// TestConcurrency_WebhookDelivery: 100 workers (background DeliverDue
// passes and synchronous AttemptNow calls, as several instances and the
// panel would) race on the same due deliveries: every event reaches the
// receiver exactly once and every delivery records exactly one attempt.
func TestConcurrency_WebhookDelivery(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		e := newM5Env(t, db)
		var mu sync.Mutex
		received := map[string]int{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			received[r.Header.Get("Arenile-Event-Id")]++
			mu.Unlock()
			time.Sleep(5 * time.Millisecond) // widen the race window
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		client := srv.Client()
		client.Transport.(*http.Transport).MaxIdleConnsPerHost = 100
		d := event.NewDeliverer(db, e.c, e.kr, client, nil, nil, event.DeliveryOptions{})
		e.endpoint(t, d, srv.URL)
		const events, workers = 20, 100
		e.dispatch(t, events)
		var ids []string
		if err := db.NewSelect().Model((*store.WebhookDelivery)(nil)).Column("id").Scan(ctx, &ids); err != nil || len(ids) != events {
			t.Fatalf("deliveries: %d %v", len(ids), err)
		}

		var attempts atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				del := event.NewDeliverer(db, e.c, e.kr, client, nil, nil, event.DeliveryOptions{})
				<-start
				if w%2 == 0 {
					n, err := del.DeliverDue(ctx, events)
					if err != nil {
						t.Error(err)
					}
					attempts.Add(int64(n))
					return
				}
				switch err := del.AttemptNow(ctx, ids[rand.IntN(len(ids))]); {
				case err == nil:
					attempts.Add(1)
				case errors.Is(err, event.ErrDeliveryNotPending):
				default:
					t.Error(err)
				}
			}(w)
		}
		close(start)
		wg.Wait()

		if len(received) != events {
			t.Fatalf("receiver got %d distinct events, want %d", len(received), events)
		}
		for id, n := range received {
			if n != 1 {
				t.Fatalf("event %s received %d times", id, n)
			}
		}
		if got := attempts.Load(); got != events {
			t.Fatalf("attempts = %d, want %d", got, events)
		}
		var maxAttempt, delivered int
		if err := db.NewSelect().TableExpr("webhook_deliveries").ColumnExpr("max(attempt)").Scan(ctx, &maxAttempt); err != nil || maxAttempt != 1 {
			t.Fatalf("max attempt = %d (%v)", maxAttempt, err)
		}
		if err := db.NewSelect().TableExpr("webhook_deliveries").ColumnExpr("count(*)").Where("delivered_at IS NOT NULL").Scan(ctx, &delivered); err != nil || delivered != events {
			t.Fatalf("delivered = %d (%v)", delivered, err)
		}
	})
}
