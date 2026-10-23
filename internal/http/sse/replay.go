package sse

import (
	"context"
	"sync"

	"github.com/coste360/arenile/pkg/ext"
)

// DefaultReplaySize is the number of recent events kept per establishment
// by ReplayBus.
const DefaultReplaySize = 512

// Replayer returns the events of an establishment published after the event
// lastEventID. ok is false when lastEventID is no longer (or was never) in
// the history: the client may have missed events and must refetch.
type Replayer interface {
	Since(establishmentID, lastEventID string) (events []ext.Event, ok bool)
}

// ReplayBus decorates an ext.EventBus with a bounded in-memory history per
// establishment, so that a reconnecting client sending Last-Event-ID gets the
// messages it missed (decorator pattern: publishers and subscribers keep
// using the plain ext.EventBus interface).
//
// The history lives in the process, like the default broker: with several
// instances behind a load balancer (ext.EventBus on LISTEN/NOTIFY), a resume
// that lands on another instance simply reports ok=false and the client
// refetches, which is always correct.
type ReplayBus struct {
	inner ext.EventBus
	size  int

	mu   sync.Mutex
	ring map[string]*ring
}

type ring struct {
	buf  []ext.Event
	next int // index of the next write
	full bool
}

var (
	_ ext.EventBus = (*ReplayBus)(nil)
	_ Replayer     = (*ReplayBus)(nil)
)

// NewReplayBus wraps inner keeping the last size events per establishment
// (DefaultReplaySize when size <= 0).
func NewReplayBus(inner ext.EventBus, size int) *ReplayBus {
	if size <= 0 {
		size = DefaultReplaySize
	}
	return &ReplayBus{inner: inner, size: size, ring: map[string]*ring{}}
}

// Publish records ev in the history of its establishment and forwards it.
// Recording happens before forwarding, so a subscriber that receives ev
// live and then asks for a replay sees it at most once (Server deduplicates
// by id).
func (b *ReplayBus) Publish(ctx context.Context, ev ext.Event) error {
	if ev.EstablishmentID != "" && ev.ID != "" {
		b.mu.Lock()
		r := b.ring[ev.EstablishmentID]
		if r == nil {
			r = &ring{buf: make([]ext.Event, b.size)}
			b.ring[ev.EstablishmentID] = r
		}
		r.buf[r.next] = ev
		r.next = (r.next + 1) % len(r.buf)
		if r.next == 0 {
			r.full = true
		}
		b.mu.Unlock()
	}
	return b.inner.Publish(ctx, ev)
}

// Subscribe delegates to the wrapped bus.
func (b *ReplayBus) Subscribe(ctx context.Context, establishmentID string) (<-chan ext.Event, func(), error) {
	return b.inner.Subscribe(ctx, establishmentID)
}

// Since implements Replayer.
func (b *ReplayBus) Since(establishmentID, lastEventID string) ([]ext.Event, bool) {
	if lastEventID == "" {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.ring[establishmentID]
	if r == nil {
		return nil, false
	}
	ordered := r.ordered()
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].ID == lastEventID {
			out := make([]ext.Event, len(ordered)-i-1)
			copy(out, ordered[i+1:])
			return out, true
		}
	}
	return nil, false
}

// ordered returns the history oldest first (caller holds the lock).
func (r *ring) ordered() []ext.Event {
	if !r.full {
		return r.buf[:r.next]
	}
	out := make([]ext.Event, 0, len(r.buf))
	out = append(out, r.buf[r.next:]...)
	return append(out, r.buf[:r.next]...)
}
