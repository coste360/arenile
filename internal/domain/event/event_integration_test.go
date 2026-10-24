//go:build integration

package event_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/crypto"
	"github.com/coste360/arenile/internal/domain/audit"
	"github.com/coste360/arenile/internal/domain/booking/bookingtest"
	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
	"github.com/coste360/arenile/pkg/ext"
)

type recordingSink struct {
	mu     sync.Mutex
	events []ext.Event
	fail   bool
}

func (s *recordingSink) Name() string { return "recording" }
func (s *recordingSink) Handle(_ context.Context, ev ext.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	if s.fail {
		return errors.New("boom")
	}
	return nil
}

type panickingSink struct{}

func (panickingSink) Name() string                            { return "panicking" }
func (panickingSink) Handle(context.Context, ext.Event) error { panic("sink panic") }

var errRollback = errors.New("rollback")

func TestOutbox_SameTransactionSemantics(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 12, 44, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		ob := event.NewOutbox(c)
		aw := audit.NewWriter(c)

		// Rolled back change: neither the change nor the event nor the audit entry exist.
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.NewUpdate().Model((*store.Establishment)(nil)).Set("name = 'Changed'").Where("id = ?", est.ID).Exec(ctx); err != nil {
				return err
			}
			if _, err := ob.Write(ctx, tx, event.ConfigMapChanged, est.ID, 1, map[string]any{"map_id": "m1"}); err != nil {
				return err
			}
			if err := aw.Record(ctx, tx, audit.Entry{EstablishmentID: est.ID, Actor: audit.SystemActor, Action: "map.updated", EntityType: "map", EntityID: "m1"}); err != nil {
				return err
			}
			return errRollback
		})
		if !errors.Is(err, errRollback) {
			t.Fatal(err)
		}
		if n := count(t, db, "outbox_events"); n != 0 {
			t.Fatalf("rolled back event persisted: %d", n)
		}
		if n := count(t, db, "audit_log"); n != 0 {
			t.Fatalf("rolled back audit persisted: %d", n)
		}

		// Committed change: event exists with the spec 18.1 envelope.
		var written ext.Event
		err = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			var err error
			written, err = ob.Write(ctx, tx, event.BookingConfirmed, est.ID, 1, map[string]any{"booking": map[string]string{"code": "AR-1"}, "spot_ids": []string{"s1"}})
			if err != nil {
				return err
			}
			return aw.Record(ctx, tx, audit.Entry{EstablishmentID: est.ID, Actor: audit.SystemActor, Action: "booking.confirmed", EntityType: "booking", EntityID: "b1"})
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "outbox_events"); n != 1 {
			t.Fatalf("committed event missing: %d", n)
		}
		if written.ID[:4] != "evt_" || written.Version != 1 || written.EstablishmentID != est.ID || !written.OccurredAt.Equal(c.Now()) {
			t.Fatalf("envelope: %+v", written)
		}
		if _, err := ob.Write(ctx, db, "Not A Type", est.ID, 1, nil); !errors.Is(err, event.ErrInvalidEvent) {
			t.Fatalf("invalid type: %v", err)
		}

		// Dispatch: sinks (one failing, one panicking) and bus receive it, a
		// webhook delivery is scheduled, the event is marked dispatched.
		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		del := event.NewDeliverer(db, c, kr, nil, aw, nil, event.DeliveryOptions{})
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		if _, _, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/hook", Events: []string{"booking.*"}}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/other", Events: []string{"payment.succeeded"}}); err != nil {
			t.Fatal(err)
		}
		sink := &recordingSink{fail: true}
		broker := event.NewBroker(nil, 8)
		sub, cancel, _ := broker.Subscribe(ctx, est.ID)
		defer cancel()
		d := event.NewDispatcher(db, c, []ext.EventSink{panickingSink{}, sink}, broker, nil)
		n, err := d.DispatchPending(ctx)
		if err != nil || n != 1 {
			t.Fatalf("dispatch: %d %v", n, err)
		}
		if len(sink.events) != 1 || sink.events[0].ID != written.ID || string(sink.events[0].Data) == "" {
			t.Fatalf("sink: %+v", sink.events)
		}
		select {
		case got := <-sub:
			if got.ID != written.ID {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("bus did not receive")
		}
		if n := count(t, db, "webhook_deliveries"); n != 1 {
			t.Fatalf("deliveries scheduled: %d (only the booking.* endpoint matches)", n)
		}
		n, err = d.DispatchPending(ctx)
		if err != nil || n != 0 {
			t.Fatalf("second dispatch must find nothing: %d %v", n, err)
		}
	})
}

func TestDeliverer_SignedDeliveryRetryAndDisable(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		_, est, err := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
		if err != nil {
			t.Fatal(err)
		}
		var failing atomic.Bool
		var received atomic.Int32
		var lastBody []byte
		var lastSig string
		var mu sync.Mutex
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			lastBody, lastSig = b, r.Header.Get(event.SignatureHeader)
			mu.Unlock()
			received.Add(1)
			if failing.Load() {
				http.Error(w, "nope", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		aw := audit.NewWriter(c)
		del := event.NewDeliverer(db, c, kr, srv.Client(), aw, nil, event.DeliveryOptions{DisableAfter: 3})
		es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
		ep, secret, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		if ep.Secret == secret {
			t.Fatal("endpoint secret must be stored encrypted")
		}
		ob := event.NewOutbox(c)
		disp := event.NewDispatcher(db, c, nil, nil, nil)
		write := func() {
			t.Helper()
			if err := store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
				_, err := ob.Write(ctx, tx, event.BookingCreated, est.ID, 1, map[string]string{"code": "AR-1"})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := disp.DispatchPending(ctx); err != nil {
				t.Fatal(err)
			}
		}

		// Success with a verifiable signature.
		write()
		if n, err := del.DeliverDue(ctx, 10); err != nil || n != 1 {
			t.Fatalf("deliver: %d %v", n, err)
		}
		mu.Lock()
		if err := event.VerifySignature(secret, lastSig, lastBody, c.Now(), 5*time.Minute); err != nil {
			t.Fatalf("signature: %v", err)
		}
		var env ext.Event
		if err := json.Unmarshal(lastBody, &env); err != nil || env.Type != event.BookingCreated || env.Version != 1 {
			t.Fatalf("body: %s", lastBody)
		}
		mu.Unlock()

		// Failures: exponential backoff, then auto-disable after 3 consecutive failures.
		failing.Store(true)
		write()
		if _, err := del.DeliverDue(ctx, 10); err != nil {
			t.Fatal(err)
		}
		if n, _ := del.DeliverDue(ctx, 10); n != 0 {
			t.Fatal("retry must wait for backoff")
		}
		c.Advance(30 * time.Second)
		if n, _ := del.DeliverDue(ctx, 10); n != 1 {
			t.Fatal("retry after 30s")
		}
		c.Advance(time.Minute)
		if n, _ := del.DeliverDue(ctx, 10); n != 1 {
			t.Fatal("retry after 1m")
		}
		got, err := store.GetWebhookEndpoint(ctx, db, es, ep.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Active || got.DisabledAt == nil || got.ConsecutiveFailures != 3 {
			t.Fatalf("endpoint must be disabled: %+v", got)
		}
		var audits int
		_ = db.NewSelect().TableExpr("audit_log").ColumnExpr("count(*)").Where("action = 'webhook.disabled'").Scan(ctx, &audits)
		if audits != 1 {
			t.Fatalf("disable audit: %d", audits)
		}
		// Pending delivery is closed as failed on the next run.
		c.Advance(2 * time.Minute)
		_, _ = del.DeliverDue(ctx, 10)
		if received.Load() != 4 {
			t.Fatalf("requests: %d", received.Load())
		}
	})
}

func TestDeliverer_GivesUpAfterMaxAge(t *testing.T) {
	db := storetest.SQLite(t)
	ctx := context.Background()
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	_, est, _ := store.CreateOrganizationWithEstablishment(ctx, db, "Org", "lido", "Lido", c.Now())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer srv.Close()
	kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
	del := event.NewDeliverer(db, c, kr, srv.Client(), nil, nil, event.DeliveryOptions{DisableAfter: 1000})
	es, _ := store.SystemEstablishmentScope(ctx, db, est.ID)
	if _, _, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	_ = store.RunInTx(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		_, err := event.NewOutbox(c).Write(ctx, tx, event.PaymentFailed, est.ID, 1, nil)
		return err
	})
	if _, err := event.NewDispatcher(db, c, nil, nil, nil).DispatchPending(ctx); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	for range 100 {
		n, _ := del.DeliverDue(ctx, 10)
		attempts += n
		c.Advance(6 * time.Hour)
	}
	var failed int
	_ = db.NewSelect().TableExpr("webhook_deliveries").ColumnExpr("count(*)").Where("failed_at IS NOT NULL").Scan(ctx, &failed)
	if failed != 1 || attempts < 5 || attempts > 20 {
		t.Fatalf("failed=%d attempts=%d", failed, attempts)
	}
}

func count(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var n int
	if err := db.NewSelect().TableExpr(table).ColumnExpr("count(*)").Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestDispatcher_TestModeEventsNotDelivered (INV-74): the events of a test
// booking are flagged when they are written, so they never reach the
// outgoing webhooks, even when the purge deleted the booking before the
// dispatcher ran; the events of a real booking are delivered. The sinks and
// the bus still see every event (the panel shows test rows).
func TestDispatcher_TestModeEventsNotDelivered(t *testing.T) {
	storetest.Each(t, func(t *testing.T, db *store.DB) {
		f := bookingtest.New(t, db)
		ctx := f.Ctx
		es, err := store.SystemEstablishmentScope(ctx, db, f.Est.ID)
		if err != nil {
			t.Fatal(err)
		}
		kr, _ := crypto.NewKeyRing(bytes.Repeat([]byte{5}, 32))
		del := event.NewDeliverer(db, f.Clock, kr, nil, audit.NewWriter(f.Clock), nil, event.DeliveryOptions{})
		if _, _, err := del.CreateEndpoint(ctx, db, es, event.CreateEndpointInput{URL: "http://127.0.0.1:1/hook", Events: []string{"booking.*"}}); err != nil {
			t.Fatal(err)
		}
		real := f.MustCreate(f.Input(f.Spots[0], "2027-07-10", "2027-07-10"))

		// Switch test mode on: the next booking is a test row.
		row, err := store.EnsureTestModeSettings(ctx, db, es, f.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		row.Enabled = true
		if ok, err := store.UpdateTestModeSettingsVersioned(ctx, db, es, row, row.Version); err != nil || !ok {
			t.Fatalf("enable test mode: %v %v", ok, err)
		}
		test := f.MustCreate(f.Input(f.Spots[1], "2027-07-10", "2027-07-10"))
		if !test.Booking.Test {
			t.Fatal("booking created in test mode not flagged")
		}
		var flagged []store.OutboxEvent
		if err := db.NewSelect().Model(&flagged).Where("test = ?", true).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if len(flagged) == 0 {
			t.Fatal("no outbox event flagged test")
		}
		for _, ev := range flagged {
			if !bytes.Contains(ev.Payload, []byte(test.Booking.ID)) {
				t.Fatalf("flagged event %s does not name the test booking", ev.Type)
			}
		}
		// The purge runs before the dispatcher: the test booking is gone.
		if _, err := db.NewDelete().Model((*store.Booking)(nil)).Where("id = ?", test.Booking.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		sink := &recordingSink{}
		d := event.NewDispatcher(db, f.Clock, []ext.EventSink{sink}, nil, nil)
		if _, err := d.DispatchPending(ctx); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "outbox_events WHERE dispatched_at IS NULL"); n != 0 {
			t.Fatalf("undispatched events: %d", n)
		}
		var deliveries []store.WebhookDelivery
		if err := db.NewSelect().Model(&deliveries).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		for _, dl := range deliveries {
			var ev store.OutboxEvent
			if err := db.NewSelect().Model(&ev).Where("id = ?", dl.EventID).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if ev.Test || bytes.Contains(ev.Payload, []byte(test.Booking.ID)) {
				t.Fatalf("test event %s scheduled to a webhook", ev.Type)
			}
		}
		realDelivered := false
		for _, dl := range deliveries {
			var ev store.OutboxEvent
			_ = db.NewSelect().Model(&ev).Where("id = ?", dl.EventID).Scan(ctx)
			if bytes.Contains(ev.Payload, []byte(real.Booking.ID)) {
				realDelivered = true
			}
		}
		if !realDelivered {
			t.Fatal("the events of the real booking were not scheduled")
		}
		if len(sink.events) < 2 {
			t.Fatalf("sinks received %d events, want every event (test ones included)", len(sink.events))
		}
	})
}
