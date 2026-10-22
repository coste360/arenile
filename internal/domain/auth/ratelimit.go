package auth

import (
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
)

// RateLimiter is an in-memory fixed-window counter per key. It is used for
// login throttling and by the HTTP layer for public endpoints. Memory is
// bounded by periodic eviction of expired windows.
type RateLimiter struct {
	clock  clock.Clock
	limit  int
	window time.Duration

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	start time.Time
	count int
}

// NewRateLimiter allows limit events per window per key.
func NewRateLimiter(c clock.Clock, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{clock: c, limit: limit, window: window, buckets: map[string]*bucket{}}
}

// Allow records one event for key and reports whether it is within the
// limit; when not, it returns how long to wait (the event is not counted).
func (l *RateLimiter) Allow(key string) (bool, time.Duration) {
	return l.do(key, true, true)
}

// Peek reports whether key is currently within the limit without recording.
func (l *RateLimiter) Peek(key string) (bool, time.Duration) {
	return l.do(key, true, false)
}

// Hit records an event without checking (e.g. a failed login).
func (l *RateLimiter) Hit(key string) { l.do(key, false, true) }

// Reset forgets key (e.g. after a successful login).
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

func (l *RateLimiter) do(key string, check, record bool) (bool, time.Duration) {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok || now.Sub(b.start) >= l.window {
		b = &bucket{start: now}
		l.buckets[key] = b
	}
	if check && b.count >= l.limit {
		return false, b.start.Add(l.window).Sub(now)
	}
	if record {
		b.count++
	}
	return true, 0
}

func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.start) >= l.window {
			delete(l.buckets, k)
		}
	}
}

// LoginThrottle limits login attempts per IP and failed attempts per account.
type LoginThrottle struct {
	perIP      *RateLimiter
	perAccount *RateLimiter
}

// NewLoginThrottle uses sensible defaults: 30 attempts per IP and 8 failed
// attempts per account every 15 minutes.
func NewLoginThrottle(c clock.Clock) *LoginThrottle {
	return &LoginThrottle{
		perIP:      NewRateLimiter(c, 30, 15*time.Minute),
		perAccount: NewRateLimiter(c, 8, 15*time.Minute),
	}
}

// Check records an attempt from ip and checks both limits.
func (t *LoginThrottle) Check(ip, account string) (bool, time.Duration) {
	if ok, retry := t.perAccount.Peek(account); !ok {
		return false, retry
	}
	return t.perIP.Allow(ip)
}

// Failure records a failed attempt for account.
func (t *LoginThrottle) Failure(account string) { t.perAccount.Hit(account) }

// Success clears the failure counter of account.
func (t *LoginThrottle) Success(account string) { t.perAccount.Reset(account) }
