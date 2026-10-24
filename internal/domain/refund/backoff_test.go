package refund

import (
	"testing"
	"time"

	"github.com/coste360/arenile/internal/store"
)

func TestBackoff_Steps(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want time.Duration
	}{
		{0, time.Minute},
		{1, time.Minute},
		{2, 5 * time.Minute},
		{3, 30 * time.Minute},
		{4, 2 * time.Hour},
		{5, 6 * time.Hour},
		{6, 12 * time.Hour},
		{40, 12 * time.Hour},
	} {
		if got := Backoff(tc.n); got != tc.want {
			t.Errorf("Backoff(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestCycle_RetryStartsNewCycle(t *testing.T) {
	s := NewService(Deps{MaxAttempts: 3})
	for _, tc := range []struct {
		n         int
		cycle     int
		exhausted bool
	}{
		{1, 1, false}, {2, 2, false}, {3, 3, true}, {4, 1, false}, {6, 3, true},
	} {
		if got := s.cycleAttempt(tc.n); got != tc.cycle {
			t.Errorf("cycleAttempt(%d) = %d, want %d", tc.n, got, tc.cycle)
		}
		if got := s.exhausted(tc.n); got != tc.exhausted {
			t.Errorf("exhausted(%d) = %v, want %v", tc.n, got, tc.exhausted)
		}
	}
}

func TestIntentKey_DerivedAfterRefusal(t *testing.T) {
	rr := &store.RefundRequest{IdempotencyKey: IdempotencyKey("b", "p")}
	for n, want := range map[int]string{0: "refund:b:p", 1: "refund:b:p", 2: "refund:b:p:a2", 11: "refund:b:p:a11"} {
		if got := intentKey(rr, n); got != want {
			t.Errorf("intentKey(%d) = %q, want %q", n, got, want)
		}
	}
}
