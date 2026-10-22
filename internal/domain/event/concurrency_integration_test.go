//go:build integration

package event_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
	"github.com/coste360/arenile/pkg/ext"
)

func writeEvents(t *testing.T, db *store.DB, c clock.Clock, establishmentID string, n int) {
	t.Helper()
	ob := event.NewOutbox(c)
	for i := 0; i < n; i++ {
		if err := store.RunInTx(context.Background(), db, func(ctx context.Context, tx bun.Tx) error {
			_, err := ob.Write(ctx, tx, event.BookingCreated, establishmentID, 1, map[string]int{"i": i})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestConcurrency_OutboxDispatch runs several dispatchers (as several
// instances would) on the same outbox: every event is marked dispatched
// exactly once and gets exactly one webhook delivery per matching endpoint.
func TestConcurrency_OutboxDispatch(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		if _, _, err := event.NewDeliverer(db, c, kr, nil, nil, nil, event.DeliveryOptions{}).
			CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/hook"}); err != nil {
			t.Fatal(err)
		}
		const events, workers = 30, 6
		writeEvents(t, db, c, est.ID, events)

		var dispatched atomic.Int64
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d := event.NewDispatcher(db, c, nil, nil, nil)
				for {
					n, err := d.DispatchPending(ctx)
					if err != nil {
						errs <- err
						return
					}
					if n == 0 {
						return
					}
					dispatched.Add(int64(n))
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		if got := dispatched.Load(); got != events {
			t.Fatalf("dispatched %d events, want %d (each exactly once)", got, events)
		}
		if n := count(t, db, "webhook_deliveries"); n != events {
			t.Fatalf("deliveries = %d, want %d", n, events)
		}
		if pending, _ := store.ListPendingOutboxEvents(ctx, db, 100); len(pending) != 0 {
			t.Fatalf("still pending: %d", len(pending))
		}
	})
}

// TestConcurrency_WebhookDelivery_ClaimedOnce runs several deliverers on the
// same due delivery: the customer endpoint receives it exactly once.
func TestConcurrency_WebhookDelivery_ClaimedOnce(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		var received atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			received.Add(1)
			time.Sleep(20 * time.Millisecond) // widen the race window
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()
		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		if _, _, err := event.NewDeliverer(db, c, kr, srv.Client(), nil, nil, event.DeliveryOptions{}).
			CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: srv.URL}); err != nil {
			t.Fatal(err)
		}
		const events, workers = 5, 8
		writeEvents(t, db, c, est.ID, events)
		if n, err := event.NewDispatcher(db, c, nil, nil, nil).DispatchPending(ctx); err != nil || n != events {
			t.Fatalf("dispatch: %d %v", n, err)
		}

		var attempts atomic.Int64
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				del := event.NewDeliverer(db, c, kr, srv.Client(), nil, nil, event.DeliveryOptions{})
				n, err := del.DeliverDue(ctx, 50)
				if err != nil {
					t.Error(err)
				}
				attempts.Add(int64(n))
			}()
		}
		wg.Wait()
		if got := received.Load(); got != events {
			t.Fatalf("endpoint received %d requests, want %d", got, events)
		}
		if got := attempts.Load(); got != events {
			t.Fatalf("attempts = %d, want %d", got, events)
		}
		var maxAttempt int
		if err := db.NewSelect().TableExpr("webhook_deliveries").ColumnExpr("max(attempt)").Scan(ctx, &maxAttempt); err != nil || maxAttempt != 1 {
			t.Fatalf("max attempt = %d (%v), want 1", maxAttempt, err)
		}
	})
}

// TestDispatcher_RedeliversAfterRestart: an event whose dispatch did not
// commit (crash, cancelled context) stays pending and a new dispatcher
// delivers it again (at-least-once).
func TestDispatcher_RedeliversAfterRestart(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(context.Background(), db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		writeEvents(t, db, c, est.ID, 2)

		ctx, crash := context.WithCancel(context.Background())
		first := &cancellingSink{cancel: crash}
		if n, err := event.NewDispatcher(db, c, []ext.EventSink{first}, nil, nil).DispatchPending(ctx); err == nil || n != 0 {
			t.Fatalf("crashed dispatch: n=%d err=%v", n, err)
		}
		if len(first.events) != 1 {
			t.Fatalf("first run saw %d events", len(first.events))
		}

		restarted := &recordingSink{}
		n, err := event.NewDispatcher(db, c, []ext.EventSink{restarted}, nil, nil).DispatchPending(context.Background())
		if err != nil || n != 2 {
			t.Fatalf("after restart: n=%d err=%v", n, err)
		}
		if len(restarted.events) != 2 || restarted.events[0].ID != first.events[0].ID {
			t.Fatalf("event not redelivered: %+v", restarted.events)
		}
	})
}

type cancellingSink struct {
	cancel context.CancelFunc
	events []ext.Event
}

func (s *cancellingSink) Name() string { return "cancelling" }
func (s *cancellingSink) Handle(_ context.Context, ev ext.Event) error {
	s.events = append(s.events, ev)
	s.cancel()
	return nil
}

// TestWebhookEndpoint_SecretBoundToRow: the encrypted signing secret uses the
// endpoint id as AAD, so a ciphertext copied onto another row is rejected.
func TestWebhookEndpoint_SecretBoundToRow(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		del := event.NewDeliverer(db, c, kr, nil, nil, nil, event.DeliveryOptions{})
		a, secretA, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/a"})
		if err != nil {
			t.Fatal(err)
		}
		b, _, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/b"})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := kr.Decrypt(a.Secret, []byte(a.ID)); err != nil || string(got) != secretA {
			t.Fatalf("own row: %q %v", got, err)
		}
		if _, err := kr.Decrypt(a.Secret, []byte(b.ID)); err == nil {
			t.Fatal("ciphertext copied to another endpoint must not decrypt")
		}
		if _, err := kr.DecryptString(a.Secret); err == nil {
			t.Fatal("ciphertext without AAD must not decrypt")
		}
	})
}
