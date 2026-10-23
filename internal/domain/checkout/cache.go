package checkout

import (
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
)

// priceCache keeps the "from" prices of the public page for a short time
// (spec 7.8: availability reads are frequent, prices change rarely; the
// availability counts themselves are never cached). Bounded: when full it
// drops the expired entries, then everything.
type priceCache struct {
	mu      sync.Mutex
	clock   clock.Clock
	ttl     time.Duration
	entries map[string]priceEntry
}

type priceEntry struct {
	value   *priceTable
	expires time.Time
}

const maxPriceCacheEntries = 512

func newPriceCache(c clock.Clock, ttl time.Duration) *priceCache {
	return &priceCache{clock: c, ttl: ttl, entries: map[string]priceEntry{}}
}

func (c *priceCache) get(key string) (*priceTable, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.clock.Now().Before(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *priceCache) put(key string, v *priceTable) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	if len(c.entries) >= maxPriceCacheEntries {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= maxPriceCacheEntries {
			c.entries = map[string]priceEntry{}
		}
	}
	c.entries[key] = priceEntry{value: v, expires: now.Add(c.ttl)}
}
