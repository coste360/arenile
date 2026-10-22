package event

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coste360/arenile/pkg/ext"
)

func TestSign_VerifyRoundtrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"id":"evt_1","type":"booking.confirmed"}`)
	h := Sign("whsec_x", body, now)
	if h[:2] != "t=" {
		t.Fatalf("header %q", h)
	}
	tests := []struct {
		name    string
		secret  string
		header  string
		body    []byte
		now     time.Time
		wantErr bool
	}{
		{"valid", "whsec_x", h, body, now, false},
		{"valid within tolerance", "whsec_x", h, body, now.Add(4 * time.Minute), false},
		{"wrong secret", "whsec_y", h, body, now, true},
		{"tampered body", "whsec_x", h, []byte(`{"id":"evt_2"}`), now, true},
		{"too old", "whsec_x", h, body, now.Add(6 * time.Minute), true},
		{"from the future", "whsec_x", h, body, now.Add(-6 * time.Minute), true},
		{"missing v1", "whsec_x", "t=1800000000", body, now, true},
		{"garbage", "whsec_x", "nonsense", body, now, true},
		{"rotation: second v1 matches", "whsec_x", h + ",v1=deadbeef", body, now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySignature(tt.secret, tt.header, tt.body, tt.now, 5*time.Minute)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrBadSignature) {
				t.Fatalf("error must wrap ErrBadSignature: %v", err)
			}
		})
	}
}

func TestSign_KnownVector(t *testing.T) {
	// HMAC-SHA256("secret", "1700000000.{}")
	got := Sign("secret", []byte("{}"), time.Unix(1700000000, 0))
	want := "t=1700000000,v1=" + mac("secret", "1700000000", []byte("{}"))
	if got != want || len(got) != len("t=1700000000,v1=")+64 {
		t.Fatalf("got %s", got)
	}
}

func TestBackoff_Schedule(t *testing.T) {
	tests := []struct {
		n    int
		want time.Duration
	}{
		{0, 30 * time.Second},
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{5, 8 * time.Minute},
		{10, 256 * time.Minute},
		{11, 6 * time.Hour},
		{40, 6 * time.Hour},
	}
	for _, tt := range tests {
		if got := Backoff(tt.n); got != tt.want {
			t.Fatalf("Backoff(%d)=%s want %s", tt.n, got, tt.want)
		}
	}
	// The cumulative schedule must exceed 24h within a bounded number of attempts.
	var total time.Duration
	n := 0
	for total < 24*time.Hour {
		n++
		total += Backoff(n)
	}
	if n > 16 {
		t.Fatalf("too many attempts before 24h: %d", n)
	}
}

func TestBroker_PublishSubscribe(t *testing.T) {
	b := NewBroker(nil, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chA, cancelA, _ := b.Subscribe(ctx, "est-a")
	chB, cancelB, _ := b.Subscribe(ctx, "est-b")
	defer cancelB()
	ev := ext.Event{ID: "evt_1", Type: "booking.created", EstablishmentID: "est-a"}
	_ = b.Publish(ctx, ev)
	select {
	case got := <-chA:
		if got.ID != "evt_1" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber A did not receive")
	}
	select {
	case got := <-chB:
		t.Fatalf("B must not receive events of A: %v", got)
	default:
	}
	// Slow subscriber: buffer 2, extra messages dropped without blocking.
	for range 5 {
		_ = b.Publish(ctx, ev)
	}
	if b.Dropped() != 3 {
		t.Fatalf("dropped=%d", b.Dropped())
	}
	cancelA()
	cancelA() // idempotent
	if b.Subscribers("est-a") != 0 {
		t.Fatal("unsubscribed")
	}
	_ = b.Publish(ctx, ev) // must not panic on closed channel
}

func TestBroker_ContextCancelUnsubscribes(t *testing.T) {
	b := NewBroker(nil, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ch, _, _ := b.Subscribe(ctx, "e")
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after context cancel")
	}
}

func TestToStreamMessage_Hints(t *testing.T) {
	ev := ext.Event{
		ID: "evt_1", Type: "booking.confirmed", EstablishmentID: "e", OccurredAt: time.Unix(0, 0),
		Data: json.RawMessage(`{"spot_ids":["s1"],"dates":["2027-07-01"],"customer":{"email":"x@y.it"}}`),
	}
	m := ToStreamMessage(ev)
	if len(m.SpotIDs) != 1 || len(m.Dates) != 1 || m.OccurredAt != "1970-01-01T00:00:00Z" {
		t.Fatalf("%+v", m)
	}
	b, _ := json.Marshal(m)
	if string(b) == "" || json.Valid(b) == false {
		t.Fatal("invalid json")
	}
	for _, forbidden := range []string{"email", "customer"} {
		if contains(string(b), forbidden) {
			t.Fatalf("stream message must not leak payload: %s", b)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
