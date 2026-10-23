package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
)

func TestAPIKeyRateLimit_PerKey(t *testing.T) {
	c := clock.NewFake(time.Date(2027, 7, 1, 9, 0, 0, 0, time.UTC))
	h := APIKeyRateLimit(auth.NewRateLimiter(c, 2, time.Minute))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	call := func(p *auth.Principal) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/establishments", nil)
		if p != nil {
			r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	keyA := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "a"}
	keyB := &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "b"}
	user := &auth.Principal{Kind: auth.PrincipalUser, UserID: "u"}
	for i := 0; i < 2; i++ {
		if w := call(keyA); w.Code != http.StatusNoContent {
			t.Fatalf("call %d: %d", i+1, w.Code)
		}
	}
	w := call(keyA)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("over budget: %d %v", w.Code, w.Header())
	}
	if w := call(keyB); w.Code != http.StatusNoContent {
		t.Fatalf("other key shares the budget: %d", w.Code)
	}
	for i := 0; i < 5; i++ {
		if w := call(user); w.Code != http.StatusNoContent {
			t.Fatalf("session user limited: %d", w.Code)
		}
		if w := call(nil); w.Code != http.StatusNoContent {
			t.Fatalf("anonymous limited: %d", w.Code)
		}
	}
	c.Advance(time.Minute)
	if w := call(keyA); w.Code != http.StatusNoContent {
		t.Fatalf("new window: %d", w.Code)
	}
}
