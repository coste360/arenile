package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/http/middleware"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/internal/store/storetest"
)

// idempotentServer wraps a slow handler that counts its executions.
func idempotentServer(db *store.DB, c clock.Clock, calls *atomic.Int32) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		time.Sleep(30 * time.Millisecond) // keep the claim open while others arrive
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"booking":%d}`, n)
	})
	return middleware.RequestContext(nil)(middleware.Idempotency(middleware.IdempotencyOptions{DB: db, Clock: c})(handler))
}

func parallelPosts(t *testing.T, h http.Handler, n int, key string) map[int]int {
	t.Helper()
	var mu sync.Mutex
	codes := map[int]int{}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := httptest.NewRequest(http.MethodPost, "/api/public/v1/lido/bookings", strings.NewReader(`{"a":1}`))
			r.RemoteAddr = "1.2.3.4:5"
			r.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code == http.StatusCreated && w.Body.String() != `{"booking":1}` {
				t.Errorf("replayed a different response: %s", w.Body.String())
			}
			mu.Lock()
			codes[w.Code]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	return codes
}

// TestConcurrency_IdempotencyKey: 100 parallel POSTs with the same key run
// the handler exactly once; the others replay the stored response or get
// 409 idempotency_in_progress.
func TestConcurrency_IdempotencyKey(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		var calls atomic.Int32
		codes := parallelPosts(t, idempotentServer(db, c, &calls), 100, "key-concurrent-1")
		if got := calls.Load(); got != 1 {
			t.Fatalf("handler executed %d times, want 1 (codes %v)", got, codes)
		}
		if codes[http.StatusCreated]+codes[http.StatusConflict] != 100 || codes[http.StatusCreated] < 1 {
			t.Fatalf("unexpected status codes: %v", codes)
		}
	})
}

// TestConcurrency_IdempotencyKey_ExpiredReclaim: requests racing to reclaim
// an expired key must not delete each other's fresh claim.
func TestConcurrency_IdempotencyKey_ExpiredReclaim(t *testing.T) {
	storetest.EachConcurrent(t, func(t *testing.T, db *store.DB) {
		c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
		status := http.StatusCreated
		old := &store.IdempotencyKey{
			Scope: "anon:1.2.3.4", Key: "key-expired-01", RequestHash: "old", ResponseStatus: &status,
			CreatedAt: c.Now().Add(-48 * time.Hour), ExpiresAt: c.Now().Add(-24 * time.Hour),
		}
		if claimed, _, err := store.ClaimIdempotencyKey(context.Background(), db, old); err != nil || !claimed {
			t.Fatalf("seed expired key: %v %v", claimed, err)
		}
		var calls atomic.Int32
		codes := parallelPosts(t, idempotentServer(db, c, &calls), 50, "key-expired-01")
		if got := calls.Load(); got != 1 {
			t.Fatalf("handler executed %d times after expiry, want 1 (codes %v)", got, codes)
		}
	})
}
