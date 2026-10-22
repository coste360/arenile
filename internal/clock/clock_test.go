package clock

import (
	"sync"
	"testing"
	"time"
)

func TestSystem_NowIsUTC(t *testing.T) {
	before := time.Now()
	now := System{}.Now()
	if now.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", now.Location())
	}
	if now.Before(before.Add(-time.Second)) || now.After(time.Now().Add(time.Second)) {
		t.Fatalf("System.Now() = %v is not the wall clock", now)
	}
}

func TestFake_AdvanceAndSet(t *testing.T) {
	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Skip("tzdata not available")
	}
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, rome)
	f := NewFake(start)
	if !f.Now().Equal(start) || f.Now().Location() != time.UTC {
		t.Fatalf("NewFake: %v", f.Now())
	}
	f.Advance(90 * time.Minute)
	if want := start.Add(90 * time.Minute); !f.Now().Equal(want) {
		t.Fatalf("Advance: got %v want %v", f.Now(), want)
	}
	later := time.Date(2026, 8, 15, 12, 0, 0, 0, rome)
	f.Set(later)
	if !f.Now().Equal(later) || f.Now().Location() != time.UTC {
		t.Fatalf("Set: %v", f.Now())
	}
}

func TestConcurrency_FakeClock(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := NewFake(start)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); f.Advance(time.Second) }()
		go func() { defer wg.Done(); _ = f.Now() }()
	}
	wg.Wait()
	if want := start.Add(50 * time.Second); !f.Now().Equal(want) {
		t.Fatalf("got %v want %v", f.Now(), want)
	}
}

var (
	_ Clock = (*Fake)(nil)
	_ Clock = System{}
)
