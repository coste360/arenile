// Package clock provides an injectable time source so that services can be
// tested without real waits (hold expiry, cut-off times, seasons).
package clock

import (
	"sync"
	"time"
)

// Clock returns the current instant.
type Clock interface {
	Now() time.Time
}

// System is the real wall clock, always in UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fake is a manually controlled clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock set to t.
func NewFake(t time.Time) *Fake { return &Fake{now: t.UTC()} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set moves the clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t.UTC()
}
