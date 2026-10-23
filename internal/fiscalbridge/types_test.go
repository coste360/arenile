package fiscalbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecide_Scenarios(t *testing.T) {
	tests := map[string]struct {
		attempts int
		res      Result
		want     Transition
	}{
		"success":                {1, Result{Outcome: OutcomeSucceeded}, Transition{Status: statusSucceeded}},
		"final failure":          {1, Result{Outcome: OutcomeFailed}, Transition{Status: statusFailed}},
		"retryable first":        {1, Result{Outcome: OutcomeFailed, Retryable: true}, Transition{Status: statusQueued, RetryIn: 15 * time.Second}},
		"retryable third":        {3, Result{Outcome: OutcomeFailed, Retryable: true}, Transition{Status: statusQueued, RetryIn: time.Minute}},
		"retryable exhausted":    {MaxAttempts, Result{Outcome: OutcomeFailed, Retryable: true}, Transition{Status: statusFailed}},
		"retryable success flag": {1, Result{Outcome: OutcomeSucceeded, Retryable: true}, Transition{Status: statusSucceeded}},
	}
	for name, tc := range tests {
		if got := Decide(tc.attempts, tc.res); got != tc.want {
			t.Errorf("%s: Decide = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestBackoff_Bounds(t *testing.T) {
	if Backoff(0) != 15*time.Second || Backoff(100) != 10*time.Minute {
		t.Fatalf("bounds %v %v", Backoff(0), Backoff(100))
	}
	for i := 1; i < 20; i++ {
		if Backoff(i+1) < Backoff(i) {
			t.Fatalf("not monotonic at %d", i)
		}
	}
}

func TestResult_ValidateAndSameAs(t *testing.T) {
	if _, err := (Result{Outcome: "maybe"}).Validate(); err == nil {
		t.Fatal("unknown outcome accepted")
	}
	if _, err := (Result{Outcome: OutcomeSucceeded, Raw: json.RawMessage(`{"a":` + strings.Repeat("1", MaxRawBytes) + `}`)}).Validate(); err == nil {
		t.Fatal("oversized raw accepted")
	}
	if _, err := (Result{Outcome: OutcomeSucceeded, Raw: json.RawMessage(`{`)}).Validate(); err == nil {
		t.Fatal("invalid raw accepted")
	}
	r, err := Result{Outcome: OutcomeFailed, Error: strings.Repeat("è", MaxErrorLen+10)}.Validate()
	if err != nil || len([]rune(r.Error)) != MaxErrorLen {
		t.Fatalf("error not trimmed: %v", err)
	}
	r, _ = Result{Outcome: OutcomeSucceeded, Retryable: true}.Validate()
	if r.Retryable {
		t.Fatal("a success is never retryable")
	}
	a := Result{Outcome: OutcomeSucceeded, Ref: "r", Number: "1", Raw: json.RawMessage(`{"x":1}`)}
	b := Result{Outcome: OutcomeSucceeded, Ref: "r", Number: "1", Raw: json.RawMessage(`{ "x": 1 }`)}
	if !a.SameAs(b) || a.SameAs(Result{Outcome: OutcomeSucceeded, Ref: "r", Number: "2"}) {
		t.Fatal("SameAs")
	}
}

func TestNotifier_BroadcastWakesSubscribers(t *testing.T) {
	n := newNotifier()
	a, b, other := n.subscribe("e1"), n.subscribe("e1"), n.subscribe("e2")
	n.broadcast("e1")
	for _, ch := range []<-chan struct{}{a, b} {
		select {
		case <-ch:
		default:
			t.Fatal("subscriber not woken")
		}
	}
	select {
	case <-other:
		t.Fatal("other establishment woken")
	default:
	}
	if n.subscribe("e1") == a {
		t.Fatal("closed generation reused")
	}
	n.broadcast("none") // no subscriber: no panic
	n.broadcastAll()
	<-other
}

func TestLocalDrivers_Registry(t *testing.T) {
	r := NewLocalDrivers()
	d, err := r.New(NoopCode, nil, nil)
	if err != nil || d.Code() != NoopCode {
		t.Fatalf("noop: %v", err)
	}
	if _, err := r.New("rt-x", nil, nil); !errors.Is(err, ErrUnknownLocalDriver) {
		t.Fatalf("unknown: %v", err)
	}
	if got := r.Codes(); len(got) != 1 || got[0] != NoopCode {
		t.Fatalf("codes %v", got)
	}
	res, _ := d.Issue(context.Background(), DocumentFromRequestKey("k1"))
	again, _ := d.Issue(context.Background(), DocumentFromRequestKey("k1"))
	if res.Ref != again.Ref {
		t.Fatal("noop refs must be stable per idempotency key")
	}
}

func TestIsRetryable_Scenarios(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"plain":     {errors.New("x"), false},
		"marked":    {Retryable(errors.New("carta finita")), true},
		"wrapped":   {fmt.Errorf("rt: %w", Retryable(errors.New("off"))), true},
		"temporary": {tempErr{}, true},
		"timeout":   {context.DeadlineExceeded, true},
		"nil":       {nil, false},
	}
	for name, tc := range tests {
		if got := IsRetryable(tc.err); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
	if Retryable(nil) != nil {
		t.Fatal("Retryable(nil) must be nil")
	}
}

type tempErr struct{}

func (tempErr) Error() string   { return "temp" }
func (tempErr) Temporary() bool { return true }

func TestFileResults_PersistsAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge", "results.json")
	s, err := OpenFileResults(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("issue:k1", Result{Outcome: OutcomeSucceeded, Ref: "r1"}); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenFileResults(path)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := s2.Get("issue:k1"); !ok || r.Ref != "r1" {
		t.Fatalf("not persisted: %+v %v", r, ok)
	}
	m := NewMemoryResults()
	for i := range MaxRememberedResults + 5 {
		_ = m.Put(fmt.Sprint(i), Result{})
	}
	if len(m.m) != MaxRememberedResults {
		t.Fatalf("not pruned: %d", len(m.m))
	}
}
