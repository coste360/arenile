package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunner_RegisterValidation(t *testing.T) {
	r := NewRunner(nil)
	tests := []struct {
		name string
		job  Job
		ok   bool
	}{
		{"valid", Job{Name: "a", Interval: time.Second, Run: func(context.Context) error { return nil }}, true},
		{"duplicate", Job{Name: "a", Interval: time.Second, Run: func(context.Context) error { return nil }}, false},
		{"no name", Job{Interval: time.Second, Run: func(context.Context) error { return nil }}, false},
		{"no interval", Job{Name: "b", Run: func(context.Context) error { return nil }}, false},
		{"no func", Job{Name: "c", Interval: time.Second}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := r.Register(tt.job); (err == nil) != tt.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
	if len(r.Jobs()) != 1 {
		t.Fatal(r.Jobs())
	}
}

func TestRunner_PeriodicAndShutdown(t *testing.T) {
	r := NewRunner(nil)
	var runs atomic.Int32
	_ = r.Register(Job{Name: "tick", Interval: 10 * time.Millisecond, RunOnStart: true, Run: func(ctx context.Context) error {
		runs.Add(1)
		return nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for runs.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop")
	}
	if runs.Load() < 3 {
		t.Fatalf("runs=%d", runs.Load())
	}
}

func TestRunner_PanicRecoveryAndErrors(t *testing.T) {
	r := NewRunner(nil)
	_ = r.Register(Job{Name: "panic", Interval: time.Hour, Run: func(context.Context) error { panic("boom") }})
	_ = r.Register(Job{Name: "fail", Interval: time.Hour, Run: func(context.Context) error { return errors.New("x") }})
	if err := r.RunNow(context.Background(), "panic"); err == nil {
		t.Fatal("panic must be converted to error")
	}
	if err := r.RunNow(context.Background(), "fail"); err == nil {
		t.Fatal("error must be returned")
	}
	if err := r.RunNow(context.Background(), "missing"); !errors.Is(err, ErrUnknownJob) {
		t.Fatal(err)
	}
}

func TestRunner_NoOverlap(t *testing.T) {
	r := NewRunner(nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	_ = r.Register(Job{Name: "slow", Interval: time.Hour, Run: func(context.Context) error {
		runs.Add(1)
		close(started)
		<-release
		return nil
	}})
	go func() { _ = r.RunNow(context.Background(), "slow") }()
	<-started
	if err := r.RunNow(context.Background(), "slow"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if runs.Load() != 1 {
		t.Fatalf("overlapping run executed: %d", runs.Load())
	}
}

func TestRunner_TimeoutCancelsRun(t *testing.T) {
	r := NewRunner(nil)
	_ = r.Register(Job{Name: "t", Interval: time.Hour, Timeout: 20 * time.Millisecond, Run: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	if err := r.RunNow(context.Background(), "t"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}
