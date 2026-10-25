package pushrelay_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/event"
	"github.com/coste360/arenile/internal/pushrelay"
)

const testSecret = "s3cr3t-of-the-relay-key-0123456789abcdef"

func mustKey(t *testing.T) pushrelay.Key {
	t.Helper()
	k, err := pushrelay.ParseKey("lido-test." + testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParseKey_Scenarios(t *testing.T) {
	tests := []struct {
		in         string
		ok         bool
		configured bool
	}{
		{"", true, false},
		{"  ", true, false},
		{"lido-test." + testSecret, true, true},
		{"LIDO." + testSecret, false, false},
		{"abc." + testSecret, false, false},
		{"lido-test.short", false, false},
		{"lido-test", false, false},
		{"lido-test." + testSecret + " x", false, false},
	}
	for _, tt := range tests {
		k, err := pushrelay.ParseKey(tt.in)
		if (err == nil) != tt.ok || k.Configured() != tt.configured {
			t.Errorf("ParseKey(%q) = configured %v, err %v", tt.in, k.Configured(), err)
		}
		if err != nil && strings.Contains(err.Error(), testSecret) {
			t.Errorf("error reveals the secret: %v", err)
		}
	}
}

func TestKey_NeverPrintsSecret(t *testing.T) {
	k := mustKey(t)
	var logged strings.Builder
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("relay", slog.Any("key", k))
	for _, s := range []string{k.String(), fmt.Sprintf("%v", k), fmt.Sprintf("%+v", k), fmt.Sprintf("%#v", k), logged.String()} {
		if strings.Contains(s, testSecret) || !strings.Contains(s, "lido-test") {
			t.Errorf("printed key %q", s)
		}
	}
}

// TestSign_MatchesWebhookScheme: the relay verifies signatures with the
// webhook scheme (event.VerifySignature, ADR 0027).
func TestSign_MatchesWebhookScheme(t *testing.T) {
	k := mustKey(t)
	at := time.Date(2027, 7, 1, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"v":1}`)
	if got, want := k.Sign(body, at), event.Sign(testSecret, body, at); got != want {
		t.Fatalf("Sign = %q, event.Sign = %q", got, want)
	}
}

// TestBuildPayload_NoPersonalData: the body has exactly the fields of
// protocol v1 and no free text; the message is a catalogue key.
func TestBuildPayload_NoPersonalData(t *testing.T) {
	b, err := json.Marshal(pushrelay.BuildPayload(pushrelay.Notification{
		ID: "0193a000-0000-7000-8000-000000000001", DevicePushID: "native-token", Platform: "ios",
		Environment: "production", Message: pushrelay.Message{Key: "booking_new_online", Locale: "it"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"device_push_id", "environment", "id", "message", "platform", "ttl_seconds", "v"}; !slices.Equal(keys, want) {
		t.Fatalf("payload fields %v, want %v", keys, want)
	}
	msg := m["message"].(map[string]any)
	if len(msg) != 2 || msg["key"] != "booking_new_online" || msg["locale"] != "it" || m["v"] != float64(1) || m["ttl_seconds"] != float64(3600) {
		t.Fatalf("payload %s", b)
	}
}

// fakeRelay verifies every request like the real relay and answers status.
type fakeRelay struct {
	t      *testing.T
	mu     sync.Mutex
	bodies []pushrelay.Payload
	status int
	header http.Header
}

func (f *fakeRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method != http.MethodPost || r.Header.Get(pushrelay.KeyHeader) != "lido-test" ||
		r.Header.Get("Content-Type") != "application/json" || r.Header.Get("User-Agent") != "arenile/1.2.3" ||
		r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
		f.t.Errorf("request %s %v", r.Method, r.Header)
	}
	if err := event.VerifySignature(testSecret, r.Header.Get(pushrelay.SignatureHeader), body, time.Now(), 5*time.Minute); err != nil {
		f.t.Errorf("signature: %v", err)
	}
	var p pushrelay.Payload
	if err := json.Unmarshal(body, &p); err != nil {
		f.t.Errorf("body: %v", err)
	}
	f.mu.Lock()
	f.bodies = append(f.bodies, p)
	status := f.status
	f.mu.Unlock()
	for k, v := range f.header {
		w.Header()[k] = v
	}
	w.WriteHeader(status)
}

func TestRelayClient_SignatureAndOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     http.Header
		want       pushrelay.Outcome
		retryAfter time.Duration
	}{
		{"accepted", http.StatusAccepted, nil, pushrelay.Delivered, 0},
		{"token gone", http.StatusGone, nil, pushrelay.Gone, 0},
		{"bad request", http.StatusBadRequest, nil, pushrelay.Rejected, 0},
		{"unknown key", http.StatusUnauthorized, nil, pushrelay.Rejected, 0},
		{"rate limited", http.StatusTooManyRequests, http.Header{"Retry-After": {"120"}}, pushrelay.Retry, 2 * time.Minute},
		{"retry after capped", http.StatusServiceUnavailable, http.Header{"Retry-After": {"86400"}}, pushrelay.Retry, time.Hour},
		{"server error", http.StatusInternalServerError, nil, pushrelay.Retry, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRelay{t: t, status: tt.status, header: tt.header}
			srv := httptest.NewServer(f)
			defer srv.Close()
			c := pushrelay.Client{URL: srv.URL, Key: mustKey(t), Version: "v1.2.3"}
			res := c.Send(context.Background(), pushrelay.Notification{
				ID: "d1", DevicePushID: "native-token-abc", Platform: "android", Environment: "production",
				Message: pushrelay.Message{Key: "payment_failed", Locale: "de"},
			})
			if res.Outcome != tt.want || res.Status != tt.status || res.RetryAfter != tt.retryAfter {
				t.Fatalf("result %+v", res)
			}
			if strings.Contains(res.Error, "native-token-abc") || strings.Contains(res.Error, testSecret) {
				t.Fatalf("error leaks data: %q", res.Error)
			}
			if len(f.bodies) != 1 || f.bodies[0].DevicePushID != "native-token-abc" || f.bodies[0].Message.Key != "payment_failed" {
				t.Fatalf("relay got %+v", f.bodies)
			}
		})
	}
	t.Run("redirect is not followed", func(t *testing.T) {
		var hit bool
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hit = true
			w.WriteHeader(http.StatusAccepted)
		}))
		defer other.Close()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
		}))
		defer srv.Close()
		res := pushrelay.Client{URL: srv.URL, Key: mustKey(t), HTTP: srv.Client()}.Send(context.Background(), pushrelay.Notification{DevicePushID: "native-token-abc"})
		if res.Outcome != pushrelay.Rejected || res.Status != http.StatusTemporaryRedirect || hit {
			t.Fatalf("result %+v, redirect target hit %v", res, hit)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := pushrelay.Client{URL: srv.URL, Key: mustKey(t)}.Send(ctx, pushrelay.Notification{DevicePushID: "native-token-abc"})
		if res.Outcome != pushrelay.Retry || !res.Cancelled {
			t.Fatalf("result %+v", res)
		}
	})
	t.Run("network error", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		res := pushrelay.Client{URL: url, Key: mustKey(t)}.Send(context.Background(), pushrelay.Notification{DevicePushID: "native-token-abc"})
		if res.Outcome != pushrelay.Retry || res.Status != 0 || strings.Contains(res.Error, "native-token-abc") {
			t.Fatalf("result %+v", res)
		}
	})
	t.Run("not configured", func(t *testing.T) {
		if res := (pushrelay.Client{URL: pushrelay.DefaultURL}).Send(context.Background(), pushrelay.Notification{}); res.Outcome != pushrelay.Rejected {
			t.Fatalf("result %+v", res)
		}
	})
}
