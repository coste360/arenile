// Package middleware contains the HTTP middlewares of Arenile: request
// context (id, real IP, scheme), access log, panic recovery, security
// headers, authentication, CSRF, rate limiting and idempotency.
package middleware

import (
	"bufio"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/coste360/arenile/internal/http/problem"
	"github.com/coste360/arenile/internal/reqctx"
)

// Middleware is a net/http middleware.
type Middleware = func(http.Handler) http.Handler

// OnPrefix applies mws only to requests whose path starts with prefix.
func OnPrefix(prefix string, mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		wrapped := next
		for i := len(mws) - 1; i >= 0; i-- {
			wrapped = mws[i](wrapped)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, prefix) {
				wrapped.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestContext assigns the request id (reusing a sane incoming
// X-Request-ID), resolves the client IP honouring X-Forwarded-For only from
// trusted proxies, and detects HTTPS (TLS or trusted X-Forwarded-Proto).
func RequestContext(trusted []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if !validRequestID(id) {
				id = uuid.Must(uuid.NewV7()).String()
			}
			remote := remoteAddr(r.RemoteAddr)
			info := &reqctx.Info{RequestID: id, ClientIP: remote.String(), HTTPS: r.TLS != nil}
			if remote.IsValid() && isTrusted(remote, trusted) {
				info.ClientIP = clientFromXFF(r.Header.Values("X-Forwarded-For"), trusted, remote).String()
				if p := strings.ToLower(strings.TrimSpace(firstCSV(r.Header.Get("X-Forwarded-Proto")))); p == "https" {
					info.HTTPS = true
				}
			}
			if !remote.IsValid() {
				info.ClientIP = "unknown"
			}
			w.Header().Set("X-Request-ID", id)
			next.ServeHTTP(w, r.WithContext(reqctx.With(r.Context(), info)))
		})
	}
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
		if !ok {
			return false
		}
	}
	return true
}

func remoteAddr(s string) netip.Addr {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap()
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		host = s
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// clientFromXFF walks X-Forwarded-For from right to left and returns the
// first address that is not a trusted proxy.
func clientFromXFF(values []string, trusted []netip.Prefix, remote netip.Addr) netip.Addr {
	var hops []string
	for _, v := range values {
		hops = append(hops, strings.Split(v, ",")...)
	}
	client := remote
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		client = a
		if !isTrusted(a, trusted) {
			break
		}
	}
	return client
}

func firstCSV(s string) string {
	if i := strings.IndexByte(s, ','); i >= 0 {
		return s[:i]
	}
	return s
}

// statusRecorder captures status and size while preserving Flush and
// http.ResponseController support.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog logs one line per request without personal data: the route
// pattern is logged instead of the raw path (which may contain tokens), and
// neither query strings nor client IPs are logged.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			route := ""
			if rc := chi.RouteContext(r.Context()); rc != nil {
				route = rc.RoutePattern()
				// Requests rejected by a middleware never reach routing:
				// resolve the pattern without exposing the raw path.
				if route == "" && rc.Routes != nil {
					route = rc.Routes.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
				}
			}
			if route == "" {
				route = "unmatched"
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			switch {
			case route == "/healthz" || route == "/readyz":
				level = slog.LevelDebug
			case status >= 500:
				level = slog.LevelError
			}
			log.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("bytes", rec.bytes),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", reqctx.From(r.Context()).RequestID),
			)
		})
	}
}

// Recoverer converts panics into a 500 problem response.
func Recoverer(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "panic in handler", slog.Any("panic", rec),
					slog.String("request_id", reqctx.From(r.Context()).RequestID), slog.String("stack", string(debug.Stack())))
				problem.Write(w, r, problem.InternalError)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// LimitBody caps request bodies.
func LimitBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSON rejects state-changing requests with a body that is not JSON.
// Besides API hygiene, this blocks cross-site HTML form submissions, which
// cannot send application/json without a CORS preflight.
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafe(r.Method) && (r.ContentLength != 0 || r.Header.Get("Content-Type") != "") {
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || (mt != "application/json" && !strings.HasSuffix(mt, "+json")) {
				problem.Write(w, r, problem.UnsupportedMediaType, problem.Detail("Usa Content-Type: application/json."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isUnsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	}
	return true
}
