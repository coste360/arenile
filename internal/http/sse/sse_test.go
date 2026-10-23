package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/pkg/ext"
)

const estID = "est-1"

// message is one parsed SSE frame (comments are collected separately).
type message struct {
	ID, Event, Data string
}

// client reads frames from a running stream.
type client struct {
	t        *testing.T
	sc       *bufio.Scanner
	frames   chan message
	comments chan string
	closed   chan struct{}
}

func newServer(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = s.Serve(w, r, estID)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func connect(t *testing.T, srv *httptest.Server, lastEventID string) *client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	c := &client{t: t, sc: bufio.NewScanner(resp.Body), frames: make(chan message, 64), comments: make(chan string, 64), closed: make(chan struct{})}
	go c.read()
	return c
}

func (c *client) read() {
	defer close(c.closed)
	var m message
	for c.sc.Scan() {
		line := c.sc.Text()
		switch {
		case line == "":
			if m != (message{}) {
				c.frames <- m
			}
			m = message{}
		case strings.HasPrefix(line, ":"):
			c.comments <- strings.TrimSpace(strings.TrimPrefix(line, ":"))
		case strings.HasPrefix(line, "id: "):
			m.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			m.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			m.Data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func (c *client) next() message {
	c.t.Helper()
	select {
	case m := <-c.frames:
		return m
	case <-time.After(3 * time.Second):
		c.t.Fatal("no message")
	}
	return message{}
}

func (c *client) comment(want string) {
	c.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-c.comments:
			if got == want {
				return
			}
		case <-deadline:
			c.t.Fatalf("comment %q not received", want)
		}
	}
}

func (c *client) noMessage(d time.Duration) {
	c.t.Helper()
	select {
	case m := <-c.frames:
		c.t.Fatalf("unexpected message %+v", m)
	case <-time.After(d):
	}
}

func waitSubscribed(t *testing.T, b *event.Broker, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for b.Subscribers(estID) < n {
		if time.Now().After(deadline) {
			t.Fatal("subscriber not registered")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func ev(i int, typ string, data string) ext.Event {
	return ext.Event{
		ID: fmt.Sprintf("evt_%03d", i), Type: typ, EstablishmentID: estID,
		OccurredAt: time.Date(2027, 7, 10, 9, 0, i, 0, time.UTC), Data: json.RawMessage(data), Version: 1,
	}
}

// TestServe_TopicsAndNoPersonalData checks the wire format of spec 21.1:
// the SSE event name is the topic, data is a StreamMessage with only ids,
// dates and spots even when the payload carries personal data.
func TestServe_TopicsAndNoPersonalData(t *testing.T) {
	b := event.NewBroker(nil, 16)
	srv := newServer(t, &Server{Bus: b, Heartbeat: time.Hour})
	c := connect(t, srv, "")
	c.comment("connected")
	waitSubscribed(t, b, 1)

	cases := []struct {
		name      string
		ev        ext.Event
		wantTopic string
	}{
		{"booking", ev(1, "booking.confirmed", `{"booking_id":"bk1","spot_ids":["s1"],"dates":["2027-07-10"],"first_name":"Mario","email":"mario@example.com","phone":"+390000000000","notes":"VIP"}`), event.TopicBooking},
		{"allocation", ev(2, event.AllocationAssigned, `{"reason":"assigned","spot_ids":["s2"],"dates":["2027-07-11"]}`), event.TopicAllocation},
		{"hold", ev(3, event.HoldCreated, `{"hold_id":"h1","spot_ids":["s3"]}`), event.TopicHold},
		{"counter sale", ev(4, "payment.succeeded", `{"payment_id":"p1","amount":500}`), event.TopicCash},
		{"booking payment", ev(5, "payment.succeeded", `{"payment_id":"p2","booking_id":"bk1","amount":500}`), event.TopicBooking},
		{"customer", ev(6, event.CustomerUpdated, `{"customer_id":"c1","last_name":"Rossi"}`), event.TopicCustomer},
		{"cash", ev(7, event.CashSessionOpened, `{"cash_session_id":"cs1","register":"main","status":"open"}`), event.TopicCash},
		{"other", ev(8, "establishment.updated", `{"name":"Lido"}`), event.TopicOther},
	}
	for _, tc := range cases {
		if err := b.Publish(context.Background(), tc.ev); err != nil {
			t.Fatal(err)
		}
		m := c.next()
		if m.ID != tc.ev.ID || m.Event != tc.wantTopic {
			t.Fatalf("%s: frame %+v, want id %s topic %s", tc.name, m, tc.ev.ID, tc.wantTopic)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(m.Data), &got); err != nil {
			t.Fatalf("%s: data %q: %v", tc.name, m.Data, err)
		}
		allowed := map[string]bool{"id": true, "topic": true, "type": true, "booking_id": true, "establishment_id": true, "occurred_at": true, "spot_ids": true, "dates": true}
		for k := range got {
			if !allowed[k] {
				t.Fatalf("%s: field %q leaked in stream data %s", tc.name, k, m.Data)
			}
		}
		for _, pii := range []string{"Mario", "mario@example.com", "+390000000000", "VIP", "Rossi", "Lido"} {
			if strings.Contains(m.Data, pii) {
				t.Fatalf("%s: personal data %q in %s", tc.name, pii, m.Data)
			}
		}
		if got["type"] != tc.ev.Type || got["establishment_id"] != estID {
			t.Fatalf("%s: data %s", tc.name, m.Data)
		}
	}
}

// TestServe_Heartbeat sends keep-alive comments.
func TestServe_Heartbeat(t *testing.T) {
	b := event.NewBroker(nil, 16)
	srv := newServer(t, &Server{Bus: b, Heartbeat: 20 * time.Millisecond})
	c := connect(t, srv, "")
	c.comment("connected")
	c.comment("ping")
	c.comment("ping")
}

// TestServe_ResumeFromLastEventID replays the missed events once, then
// streams live without duplicates.
func TestServe_ResumeFromLastEventID(t *testing.T) {
	broker := event.NewBroker(nil, 16)
	bus := NewReplayBus(broker, 8)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if err := bus.Publish(ctx, ev(i, "booking.created", `{"booking_id":"bk"}`)); err != nil {
			t.Fatal(err)
		}
	}
	srv := newServer(t, &Server{Bus: bus, Heartbeat: time.Hour})
	c := connect(t, srv, "evt_001")
	if m := c.next(); m.ID != "evt_002" {
		t.Fatalf("first replayed %+v", m)
	}
	if m := c.next(); m.ID != "evt_003" {
		t.Fatalf("second replayed %+v", m)
	}
	waitSubscribed(t, broker, 1)
	if err := bus.Publish(ctx, ev(4, "booking.cancelled", `{"booking_id":"bk"}`)); err != nil {
		t.Fatal(err)
	}
	if m := c.next(); m.ID != "evt_004" || m.Event != event.TopicBooking {
		t.Fatalf("live %+v", m)
	}
	c.noMessage(50 * time.Millisecond)
}

// TestServe_ResumeUnknownIDStartsLive tells the client the resume is not
// possible (it refetches) and continues live.
func TestServe_ResumeUnknownIDStartsLive(t *testing.T) {
	broker := event.NewBroker(nil, 16)
	bus := NewReplayBus(broker, 2)
	ctx := context.Background()
	for i := 1; i <= 3; i++ { // evt_001 falls out of the ring
		_ = bus.Publish(ctx, ev(i, "booking.created", `{}`))
	}
	srv := newServer(t, &Server{Bus: bus, Heartbeat: time.Hour})
	c := connect(t, srv, "evt_001")
	c.comment("resume unavailable")
	waitSubscribed(t, broker, 1)
	_ = bus.Publish(ctx, ev(9, "booking.created", `{}`))
	if m := c.next(); m.ID != "evt_009" {
		t.Fatalf("live %+v", m)
	}
}

// chanBus is an ext.EventBus whose subscriber channel is controlled by the
// test.
type chanBus struct{ ch chan ext.Event }

func (b *chanBus) Publish(context.Context, ext.Event) error { return nil }
func (b *chanBus) Subscribe(context.Context, string) (<-chan ext.Event, func(), error) {
	return b.ch, func() {}, nil
}

// TestServe_DisconnectsLaggingClient ends the stream of a subscriber whose
// buffer filled up (the broker may have dropped messages): the browser
// reconnects and resumes instead of silently missing updates.
func TestServe_DisconnectsLaggingClient(t *testing.T) {
	bus := &chanBus{ch: make(chan ext.Event, 4)}
	for i := 1; i <= 4; i++ {
		bus.ch <- ev(i, "booking.created", `{}`)
	}
	s := &Server{Bus: bus, Heartbeat: time.Hour}
	srv := newServer(t, s)
	c := connect(t, srv, "")
	select {
	case <-c.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("lagging client not disconnected")
	}
	if s.Disconnected() != 1 {
		t.Fatalf("disconnected = %d", s.Disconnected())
	}
}

// TestServe_EndsWithContext stops when the client goes away and releases
// the subscription; a normal close is not counted as a drop.
func TestServe_EndsWithContext(t *testing.T) {
	b := event.NewBroker(nil, 16)
	s := &Server{Bus: b, Heartbeat: time.Hour}
	srv := newServer(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	waitSubscribed(t, b, 1)
	cancel()
	_ = resp.Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for b.Subscribers(estID) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("subscription not released")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.Disconnected() != 0 {
		t.Fatalf("normal close counted as drop: %d", s.Disconnected())
	}
}

// TestReplayBus_Since covers the ring buffer: order, wrap-around, unknown
// ids and isolation between establishments.
func TestReplayBus_Since(t *testing.T) {
	ctx := context.Background()
	bus := NewReplayBus(event.NewBroker(nil, 4), 3)
	for i := 1; i <= 5; i++ {
		_ = bus.Publish(ctx, ev(i, "booking.created", `{}`))
	}
	other := ev(6, "booking.created", `{}`)
	other.EstablishmentID = "est-2"
	_ = bus.Publish(ctx, other)

	ids := func(evs []ext.Event) string {
		var out []string
		for _, e := range evs {
			out = append(out, e.ID)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name, est, last string
		want            string
		ok              bool
	}{
		{"oldest kept", estID, "evt_003", "evt_004,evt_005", true},
		{"latest", estID, "evt_005", "", true},
		{"evicted", estID, "evt_002", "", false},
		{"empty id", estID, "", "", false},
		{"other establishment id", estID, "evt_006", "", false},
		{"unknown establishment", "est-x", "evt_003", "", false},
		{"second establishment", "est-2", "evt_006", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := bus.Since(tc.est, tc.last)
			if ok != tc.ok || ids(got) != tc.want {
				t.Fatalf("Since(%s, %s) = %q, %v; want %q, %v", tc.est, tc.last, ids(got), ok, tc.want, tc.ok)
			}
		})
	}
}

// TestConcurrency_ReplayBusPublishSince publishes and reads the history
// from many goroutines (run with -race).
func TestConcurrency_ReplayBusPublishSince(t *testing.T) {
	ctx := context.Background()
	bus := NewReplayBus(event.NewBroker(nil, 4), 32)
	done := make(chan struct{})
	for g := 0; g < 100; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			_ = bus.Publish(ctx, ev(g, "booking.created", `{}`))
			_, _ = bus.Since(estID, fmt.Sprintf("evt_%03d", g))
		}(g)
	}
	for g := 0; g < 100; g++ {
		<-done
	}
	got, ok := bus.Since(estID, "missing")
	if ok || got != nil {
		t.Fatal("unknown id resolved")
	}
}
