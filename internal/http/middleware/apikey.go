package middleware

import (
	"net/http"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/problem"
)

// DefaultAPIKeyRequestsPerMinute is the default budget of one API key on
// the admin API (spec 17, 22): generous for an integration that syncs
// bookings, low enough that a runaway script cannot monopolise a small
// self-hosted server.
const DefaultAPIKeyRequestsPerMinute = 300

// APIKeyRateLimit limits the requests of each API key (fixed window per key
// id, in memory, per instance). It must run after Authenticate; requests
// authenticated with a session cookie or not authenticated pass through
// (login has its own throttle, the public API PublicRateLimit). Over budget
// the answer is 429 rate_limited with Retry-After.
func APIKeyRateLimit(l *auth.RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p, ok := auth.PrincipalFrom(r.Context()); ok && p.Kind == auth.PrincipalAPIKey {
				if ok, retry := l.Allow(p.APIKeyID); !ok {
					problem.Write(w, r, problem.RateLimited, problem.RetryAfter(retry))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
