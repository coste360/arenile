package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// TestIdempotency_NeverStoresNoStoreResponses: a response marked NoReplay
// (a secret shown once) is never written to idempotency_keys, even when the
// client sends an Idempotency-Key.
func TestIdempotency_NeverStoresNoStoreResponses(t *testing.T) {
	db := storetest.SQLite(t)
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	var calls atomic.Int32
	h := middleware.RequestContext(nil)(middleware.Idempotency(middleware.IdempotencyOptions{DB: db, Clock: c})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "private, no-store")
			middleware.NoReplay(w)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"secret":"ar_live_x_%d"}`, n)
		})))
	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/admin/v1/establishments/e/api-keys", strings.NewReader(`{"name":"x"}`))
		r.Header.Set("Idempotency-Key", "key-secret-01")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := post(); w.Code != 201 || !strings.Contains(w.Body.String(), "ar_live_x_1") {
		t.Fatalf("first: %d %s", w.Code, w.Body.String())
	}
	var stored int
	if err := db.NewSelect().Model((*store.IdempotencyKey)(nil)).ColumnExpr("count(*)").Scan(context.Background(), &stored); err != nil || stored != 0 {
		t.Fatalf("stored rows = %d (%v): a secret was persisted", stored, err)
	}
	if w := post(); w.Code != 201 || w.Header().Get("Idempotent-Replayed") != "" || calls.Load() != 2 {
		t.Fatalf("retry must execute again, not replay: %d %v", w.Code, w.Header())
	}
}

// TestIdempotency_NoStoreHeaderStillReplays: SecurityHeaders marks every
// /api/ response no-store, so the header alone must not disable the replay
// (admin bookings and payments, the offline queue, the public double tap).
// Wired as in the server: SecurityHeaders, then Idempotency.
func TestIdempotency_NoStoreHeaderStillReplays(t *testing.T) {
	db := storetest.SQLite(t)
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	var calls atomic.Int32
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if strings.HasSuffix(r.URL.Path, "/api-keys") {
			middleware.NoReplay(w)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"n":%d}`, n)
	})
	h := middleware.RequestContext(nil)(middleware.SecurityHeaders(middleware.SecurityOptions{})(
		middleware.Idempotency(middleware.IdempotencyOptions{DB: db, Clock: c})(inner)))
	post := func(path, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"x":1}`))
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/admin/v1/establishments/e/payments", "/api/public/v1/lido/bookings"} {
		key := "key-" + strings.ReplaceAll(path, "/", "-")
		before := calls.Load()
		first, second := post(path, key), post(path, key)
		if first.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: SecurityHeaders not in the chain: %v", path, first.Header())
		}
		if second.Header().Get("Idempotent-Replayed") != "true" || second.Body.String() != first.Body.String() || calls.Load() != before+1 {
			t.Fatalf("%s: retry must replay: %v %s (calls %d)", path, second.Header(), second.Body.String(), calls.Load()-before)
		}
	}
	before := calls.Load()
	_ = post("/api/admin/v1/establishments/e/api-keys", "key-secret-03")
	if again := post("/api/admin/v1/establishments/e/api-keys", "key-secret-03"); again.Header().Get("Idempotent-Replayed") != "" || calls.Load() != before+2 {
		t.Fatalf("a secret shown once was replayed: %v (calls %d)", again.Header(), calls.Load()-before)
	}
}
