package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/store/storetest"
)

func TestIdempotency_ReplayMismatchAndRetry(t *testing.T) {
	db := storetest.SQLite(t)
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	var calls atomic.Int32
	var fail atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"booking":%d}`, n)
	})
	h := middleware.RequestContext(nil)(middleware.Idempotency(middleware.IdempotencyOptions{DB: db, Clock: c, Skip: []string{"/api/admin/v1/auth/"}})(handler))
	postAs := func(path, key, body, checkout string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.RemoteAddr = "1.2.3.4:5"
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		if checkout != "" {
			r.Header.Set("X-Arenile-Checkout", checkout)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	post := func(path, key, body string) *httptest.ResponseRecorder { return postAs(path, key, body, "") }

	// The checkout token is part of the request identity: the same key
	// from the same IP with another (or no) token is not a replay of the
	// first response (which may carry a payment redirect).
	withToken := postAs("/api/public/v1/lido/bookings/b1/pay", "key-00000009", `{}`, strings.Repeat("A", 43))
	if withToken.Code != 201 {
		t.Fatalf("pay: %d", withToken.Code)
	}
	if w := postAs("/api/public/v1/lido/bookings/b1/pay", "key-00000009", `{}`, strings.Repeat("B", 43)); w.Code != 422 {
		t.Fatalf("same key, other token must not replay: %d %s", w.Code, w.Body.String())
	}
	if w := post("/api/public/v1/lido/bookings/b1/pay", "key-00000009", `{}`); w.Code != 422 {
		t.Fatalf("same key, no token must not replay: %d", w.Code)
	}
	calls.Store(0)

	first := post("/api/public/v1/lido/bookings", "key-00000001", `{"a":1}`)
	if first.Code != 201 || first.Body.String() != `{"booking":1}` {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	replay := post("/api/public/v1/lido/bookings", "key-00000001", `{"a":1}`)
	if replay.Code != 201 || replay.Body.String() != `{"booking":1}` || replay.Header().Get("Idempotent-Replayed") != "true" ||
		replay.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("replay: %d %s %v", replay.Code, replay.Body.String(), replay.Header())
	}
	if calls.Load() != 1 {
		t.Fatal("replay must not execute the handler")
	}
	mismatch := post("/api/public/v1/lido/bookings", "key-00000001", `{"a":2}`)
	if mismatch.Code != 422 || !strings.Contains(mismatch.Body.String(), "idempotency_key_reused") {
		t.Fatalf("mismatch: %d %s", mismatch.Code, mismatch.Body.String())
	}
	if w := post("/api/public/v1/lido/bookings", "short", `{}`); w.Code != 400 {
		t.Fatalf("short key: %d", w.Code)
	}
	if post("/api/public/v1/lido/bookings", "", `{}`).Code != 201 || calls.Load() != 2 {
		t.Fatal("requests without key are not deduplicated")
	}

	// Server errors are not stored: the client may retry with the same key.
	fail.Store(true)
	if w := post("/api/public/v1/lido/bookings", "key-00000002", `{}`); w.Code != 500 {
		t.Fatal(w.Code)
	}
	fail.Store(false)
	if w := post("/api/public/v1/lido/bookings", "key-00000002", `{}`); w.Code != 201 {
		t.Fatalf("retry after 5xx: %d", w.Code)
	}

	// Skipped paths (auth) are never stored.
	before := calls.Load()
	post("/api/admin/v1/auth/login", "key-00000003", `{}`)
	post("/api/admin/v1/auth/login", "key-00000003", `{}`)
	if calls.Load() != before+2 {
		t.Fatal("auth endpoints must not be replayed")
	}

	// Expired records are forgotten.
	c.Advance(25 * time.Hour)
	if w := post("/api/public/v1/lido/bookings", "key-00000001", `{"a":2}`); w.Code != 201 || w.Header().Get("Idempotent-Replayed") != "" {
		t.Fatalf("expired key: %d", w.Code)
	}
}
