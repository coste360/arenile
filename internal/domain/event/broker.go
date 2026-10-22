package event

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/coste360/arenile/pkg/ext"
)

// Broker is the in-memory real-time bus (ext.EventBus) used by the SSE
// endpoint. Slow subscribers never block publishers: messages that do not
// fit in a subscriber buffer are dropped (clients re-fetch on reconnect).
type Broker struct {
	log     *slog.Logger
	buffer  int
	mu      sync.RWMutex
	subs    map[string]map[*subscriber]struct{}
	dropped atomic.Int64
}

type subscriber struct {
	ch   chan ext.Event
	once sync.Once
}

var _ ext.EventBus = (*Broker)(nil)

// NewBroker returns a broker with a per-subscriber buffer.
func NewBroker(log *slog.Logger, buffer int) *Broker {
	if buffer <= 0 {
		buffer = 64
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Broker{log: log, buffer: buffer, subs: map[string]map[*subscriber]struct{}{}}
}

// Publish delivers ev to the subscribers of its establishment.
func (b *Broker) Publish(_ context.Context, ev ext.Event) error {
	if ev.EstablishmentID == "" {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs[ev.EstablishmentID] {
		select {
		case s.ch <- ev:
		default:
			b.dropped.Add(1)
		}
	}
	return nil
}

// Subscribe registers a subscriber for one establishment.
func (b *Broker) Subscribe(ctx context.Context, establishmentID string) (<-chan ext.Event, func(), error) {
	s := &subscriber{ch: make(chan ext.Event, b.buffer)}
	b.mu.Lock()
	if b.subs[establishmentID] == nil {
		b.subs[establishmentID] = map[*subscriber]struct{}{}
	}
	b.subs[establishmentID][s] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		s.once.Do(func() {
			b.mu.Lock()
			delete(b.subs[establishmentID], s)
			if len(b.subs[establishmentID]) == 0 {
				delete(b.subs, establishmentID)
			}
			close(s.ch)
			b.mu.Unlock()
		})
	}
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return s.ch, cancel, nil
}

// Subscribers returns the number of subscribers of an establishment.
func (b *Broker) Subscribers(establishmentID string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[establishmentID])
}

// Dropped returns the number of messages dropped because of slow subscribers.
func (b *Broker) Dropped() int64 { return b.dropped.Load() }
