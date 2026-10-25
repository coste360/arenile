package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
)

// RateLimitByIPPrefix limits requests per client network: the IPv4 address,
// or the /64 of an IPv6 one (a single client can rotate through a whole
// /64), plus a global budget shared by every client. Used by the
// unauthenticated demo pairing (ADR 0027).
func RateLimitByIPPrefix(perClient, global *auth.RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := IPPrefixKey(reqctx.From(r.Context()).ClientIP)
			for _, l := range []struct {
				lim *auth.RateLimiter
				key string
			}{{perClient, key}, {global, "*"}} {
				if ok, retry := l.lim.Allow(l.key); !ok {
					problem.Write(w, r, problem.RateLimited, problem.RetryAfter(retry))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPPrefixKey is the rate limit key of a client IP: the address for IPv4
// (and anything unparsable, as is), the /64 network for IPv6.
func IPPrefixKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || a.Is4() || a.Is4In6() {
		return ip
	}
	p, err := a.Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

// PublicRateLimit limits public API calls per client IP with the two budgets
// of spec 17.2: creations are the requests that create something new from
// nothing, POST .../bookings (a hold), POST .../manage-links (an email) and
// POST .../waitlist (a customer, an entry and an email);
// every other call counts as a read, including the session-token or
// magic-link steps of an existing booking (customer details, pay, modify,
// cancel), so one checkout never spends more than one creation and
// several customers behind the same Wi-Fi can still book. POST .../quote
// writes nothing (debounced price summary, spec 12.3).
func PublicRateLimit(reads, creations *auth.RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := reqctx.From(r.Context()).ClientIP
			l := reads
			if isPublicCreation(r) {
				l = creations
			}
			if ok, retry := l.Allow(ip); !ok {
				problem.Write(w, r, problem.RateLimited, problem.RetryAfter(retry))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IdempotencyOptions configure Idempotency.
type IdempotencyOptions struct {
	DB    bun.IDB
	Clock clock.Clock
	Log   *slog.Logger
	// TTL of stored responses (default 24h).
	TTL time.Duration
	// Skip excludes paths (prefix match), e.g. authentication endpoints
	// whose responses set cookies that must not be replayed.
	Skip []string
	// MaxBody is the largest response stored for replay (default 1 MiB).
	MaxBody int
}

const maxIdempotencyKeyLen = 128

// Idempotency honours the Idempotency-Key header on POST requests: the first
// request with a key is executed and its response stored; repeats with the
// same request return the stored response, repeats with a different request
// get 422 idempotency_key_reused, concurrent repeats get 409.
func Idempotency(o IdempotencyOptions) Middleware {
	if o.TTL <= 0 {
		o.TTL = 24 * time.Hour
	}
	if o.MaxBody <= 0 {
		o.MaxBody = 1 << 20
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if r.Method != http.MethodPost || key == "" || skipped(r.URL.Path, o.Skip) {
				next.ServeHTTP(w, r)
				return
			}
			if len(key) < 8 || len(key) > maxIdempotencyKeyLen {
				problem.Write(w, r, problem.BadRequest, problem.Detail("Idempotency-Key deve avere tra 8 e 128 caratteri."))
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				problem.Write(w, r, problem.BadRequest, problem.Detail("Corpo della richiesta non leggibile o troppo grande."))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			h := sha256.New()
			h.Write([]byte(r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery + "\n"))
			// The checkout token authenticates public calls: a replay with
			// another (or no) token is another request, never the stored
			// response of the first one.
			h.Write([]byte(r.Header.Get("X-Arenile-Checkout") + "\n"))
			h.Write(body)
			hash := hex.EncodeToString(h.Sum(nil))
			scope := idempotencyScope(r)
			ctx := r.Context()

			now := o.Clock.Now()
			row := &store.IdempotencyKey{Scope: scope, Key: key, RequestHash: hash, CreatedAt: now, ExpiresAt: now.Add(o.TTL)}
			claimed, existing, err := store.ClaimIdempotencyKey(ctx, o.DB, row)
			if err == nil && !claimed && !existing.ExpiresAt.After(now) {
				// Expired record: forget it and claim again.
				if err = store.ReleaseExpiredIdempotencyKey(ctx, o.DB, scope, key, now); err == nil {
					claimed, existing, err = store.ClaimIdempotencyKey(ctx, o.DB, row)
				}
			}
			if err != nil {
				problem.Error(w, r, o.Log, err)
				return
			}
			if !claimed {
				switch {
				case existing.RequestHash != hash:
					problem.Write(w, r, problem.IdempotencyKeyReused)
				case existing.ResponseStatus == nil:
					problem.Write(w, r, problem.IdempotencyInProgress)
				default:
					if existing.ResponseContentType != nil {
						w.Header().Set("Content-Type", *existing.ResponseContentType)
					}
					w.Header().Set("Idempotent-Replayed", "true")
					w.WriteHeader(*existing.ResponseStatus)
					_, _ = w.Write(existing.ResponseBody)
				}
				return
			}

			rec := &captureWriter{ResponseWriter: w, max: o.MaxBody}
			completed := false
			defer func() {
				if completed {
					return
				}
				// Handler panicked or failed: allow a retry with the same key.
				if err := store.ReleaseIdempotencyKey(context.WithoutCancel(ctx), o.DB, scope, key); err != nil {
					o.Log.WarnContext(ctx, "release idempotency key", slog.String("error", err.Error()))
				}
			}()
			next.ServeHTTP(rec, r)
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			// A response marked NoReplay carries a credential shown once (API
			// key, webhook secret, initial password, pairing code, magic link):
			// never persist it for replay. The key is released, so a retry
			// executes the request again. Cache-Control: no-store alone is not
			// the signal: SecurityHeaders sets it on every /api/ response.
			if status >= 500 || rec.overflow || rec.noReplay {
				return
			}
			if err := store.CompleteIdempotencyKey(context.WithoutCancel(ctx), o.DB, scope, key, status, rec.Header().Get("Content-Type"), rec.buf.Bytes()); err != nil {
				o.Log.WarnContext(ctx, "store idempotent response", slog.String("error", err.Error()))
				return
			}
			completed = true
		})
	}
}

// NoReplay marks the response being written as one that must never be
// stored by Idempotency (a secret shown once). Handlers call it before
// writing; it finds the idempotency recorder through Unwrap chains and is a
// no-op when the request carries no Idempotency-Key.
func NoReplay(w http.ResponseWriter) {
	for w != nil {
		if c, ok := w.(*captureWriter); ok {
			c.noReplay = true
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

func skipped(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func idempotencyScope(r *http.Request) string {
	if p, ok := auth.PrincipalFrom(r.Context()); ok {
		return p.ID()
	}
	return "anon:" + reqctx.From(r.Context()).ClientIP
}

type captureWriter struct {
	http.ResponseWriter
	status   int
	buf      bytes.Buffer
	max      int
	overflow bool
	noReplay bool
}

func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if !c.overflow {
		if c.buf.Len()+len(b) > c.max {
			c.overflow = true
			c.buf.Reset()
		} else {
			c.buf.Write(b)
		}
	}
	return c.ResponseWriter.Write(b)
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// isPublicCreation: POST of a new booking session, of a new manage link or
// of a waitlist request (POST .../waitlist creates a customer, an entry and
// an email; the accept/decline steps of an existing request under
// .../waitlist/{token}/... are reads).
func isPublicCreation(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	p := strings.TrimRight(r.URL.Path, "/")
	return strings.HasSuffix(p, "/bookings") || strings.HasSuffix(p, "/manage-links") || strings.HasSuffix(p, "/waitlist")
}
