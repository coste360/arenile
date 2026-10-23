package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// Cookie names.
const (
	SessionCookie = "arenile_session"
	CSRFCookie    = "arenile_csrf"
	CSRFHeader    = "X-CSRF-Token"
)

// Authenticator resolves credentials to principals.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, token string) (*auth.Principal, error)
	AuthenticateAPIKey(ctx context.Context, key string) (*auth.Principal, error)
}

func infoHTTPS(r *http.Request) bool { return reqctx.From(r.Context()).HTTPS }

// CookieSecure decides the Secure attribute of a cookie set in answer to
// r: the configured default (config.SecureCookies: false only for plain
// HTTP development and the desktop loopback profile) OR the request came
// over HTTPS (TLS on this listener or a trusted X-Forwarded-Proto). The
// desktop LAN listener serves the same handler over TLS while the loopback
// listener is plain HTTP, so a static flag would send the session cookie
// in clear on the lido Wi-Fi.
func CookieSecure(r *http.Request, configured bool) bool { return configured || infoHTTPS(r) }

// Authenticate puts the principal (from a Bearer API key or the session
// cookie) in the context. An invalid Bearer key is rejected; an invalid or
// expired cookie is ignored (the request continues unauthenticated).
func Authenticate(a Authenticator, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := reqctx.From(r.Context())
			ctx := r.Context()
			if h := r.Header.Get("Authorization"); h != "" {
				scheme, key, ok := strings.Cut(h, " ")
				if !ok || !strings.EqualFold(scheme, "Bearer") {
					problem.Write(w, r, problem.Unauthorized, problem.Detail("Usa Authorization: Bearer <chiave API>."))
					return
				}
				p, err := a.AuthenticateAPIKey(ctx, strings.TrimSpace(key))
				if err != nil {
					if !errors.Is(err, auth.ErrUnauthenticated) {
						problem.Error(w, r, log, err)
						return
					}
					problem.Write(w, r, problem.Unauthorized)
					return
				}
				info.AuthMethod = reqctx.AuthBearer
				next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(ctx, p)))
				return
			}
			if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
				p, err := a.AuthenticateSession(ctx, c.Value)
				switch {
				case err == nil:
					info.AuthMethod = reqctx.AuthCookie
					info.SessionToken = c.Value
					ctx = auth.WithPrincipal(ctx, p)
				case !errors.Is(err, auth.ErrUnauthenticated):
					problem.Error(w, r, log, err)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth rejects unauthenticated requests except for the given exact
// paths, and restricts owners who must still enrol TOTP to the paths under
// totpEnrolmentPrefix (the auth endpoints).
func RequireAuth(publicPaths []string, totpEnrolmentPrefix string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if slices.Contains(publicPaths, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			p, ok := auth.PrincipalFrom(r.Context())
			if !ok {
				problem.Write(w, r, problem.Unauthorized)
				return
			}
			if p.TOTPEnrolmentRequired && !strings.HasPrefix(r.URL.Path, totpEnrolmentPrefix) {
				problem.Write(w, r, problem.TOTPEnrolmentRequired)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NewCSRFToken returns a random token for the double-submit cookie.
func NewCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// SetCSRFCookie sets the (JavaScript-readable) CSRF cookie.
func SetCSRFCookie(w http.ResponseWriter, token string, secure bool, maxAge time.Duration) {
	// Double-submit cookie: must be readable by JavaScript, Secure follows config.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly=false is required for double-submit CSRF
		Name: CSRFCookie, Value: token, Path: "/", Secure: secure, HttpOnly: false,
		SameSite: http.SameSiteLaxMode, MaxAge: int(maxAge.Seconds()),
	})
}

// CSRF implements the double-submit token for cookie-authenticated
// state-changing requests: the X-CSRF-Token header must equal the
// arenile_csrf cookie. The cookie is issued on any response when missing.
// Requests authenticated with a Bearer API key are exempt.
func CSRF(secure bool, cookieMaxAge time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CSRFCookie)
			hasCookie := err == nil && len(cookie.Value) >= 32
			if !hasCookie {
				SetCSRFCookie(w, NewCSRFToken(), CookieSecure(r, secure), cookieMaxAge)
			}
			if isUnsafe(r.Method) && reqctx.From(r.Context()).AuthMethod == reqctx.AuthCookie {
				header := r.Header.Get(CSRFHeader)
				if !hasCookie || header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
					problem.Write(w, r, problem.CSRFFailed)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
