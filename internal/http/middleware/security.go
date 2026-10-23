package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// EmbedOriginsFunc returns the origins allowed to frame the public page of
// an establishment (allowed_embed_origins).
type EmbedOriginsFunc func(ctx context.Context, slug string) ([]string, error)

// SecurityOptions configure SecurityHeaders.
type SecurityOptions struct {
	EmbedOrigins EmbedOriginsFunc
	// AdminFrameSources are extra origins the admin panel may frame (UI plugins).
	AdminFrameSources []string
	Log               *slog.Logger
}

const adminCSPBase = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; manifest-src 'self'; worker-src 'self'; " +
	"base-uri 'self'; form-action 'self'; object-src 'none'; frame-ancestors 'none'"

const bookingCSPBase = "default-src 'self'; script-src 'self' https://js.stripe.com; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self' https://api.stripe.com; " +
	"frame-src https://js.stripe.com https://hooks.stripe.com; base-uri 'self'; form-action 'self' https:; object-src 'none'"

const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// managePathRe matches the customer manage area of the public SPA
// (/b/{slug}/m and /b/{slug}/m/{token}).
var managePathRe = regexp.MustCompile(`^/b/[^/]+/m(/|$)`)

var originRe = regexp.MustCompile(`^https?://(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`)

// SanitizeOrigins keeps only syntactically safe origins for CSP sources.
func SanitizeOrigins(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		o = strings.ToLower(strings.TrimRight(strings.TrimSpace(o), "/"))
		if originRe.MatchString(o) {
			out = append(out, o)
		}
	}
	return out
}

// SecurityHeaders sets the headers of spec 22: a strict CSP on the admin
// panel, per-establishment frame-ancestors on the public page, nosniff,
// Referrer-Policy and HSTS over HTTPS.
func SecurityHeaders(opts SecurityOptions) Middleware {
	adminCSP := adminCSPBase
	if fs := SanitizeOrigins(opts.AdminFrameSources); len(fs) > 0 {
		adminCSP += "; frame-src 'self' " + strings.Join(fs, " ")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if infoHTTPS(r) {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			p := r.URL.Path
			switch {
			case strings.HasPrefix(p, "/api/"):
				h.Set("Content-Security-Policy", apiCSP)
				h.Set("Cache-Control", "no-store")
			case p == "/admin" || strings.HasPrefix(p, "/admin/"):
				h.Set("Content-Security-Policy", adminCSP)
				h.Set("X-Frame-Options", "DENY")
				h.Set("Cross-Origin-Opener-Policy", "same-origin")
				h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			case strings.HasPrefix(p, "/b/"):
				ancestors := "'self'"
				if slug := slugFromPath(p); slug != "" && opts.EmbedOrigins != nil {
					origins, err := opts.EmbedOrigins(r.Context(), slug)
					if err != nil && opts.Log != nil {
						opts.Log.WarnContext(r.Context(), "embed origins lookup failed", slog.String("error", err.Error()))
					}
					if o := SanitizeOrigins(origins); len(o) > 0 {
						ancestors += " " + strings.Join(o, " ")
					}
				}
				h.Set("Content-Security-Policy", bookingCSPBase+"; frame-ancestors "+ancestors)
				if managePathRe.MatchString(p) {
					// The magic link token is in the path: never in a Referer,
					// never in a shared cache.
					h.Set("Referrer-Policy", "no-referrer")
					h.Set("Cache-Control", "no-store")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func slugFromPath(p string) string {
	rest := strings.TrimPrefix(p, "/b/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.ToLower(rest)
}
