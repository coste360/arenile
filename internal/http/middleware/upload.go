package middleware

import (
	"mime"
	"net/http"
	"slices"
	"strings"

	"github.com/coste360/arenile/internal/http/problem"
)

// Upload is an API route that accepts a non-JSON request body (a file) with
// its own size cap, e.g. the CSV import (text/csv, 5 MB).
type Upload struct {
	// Method is the HTTP method of the route (POST).
	Method string
	// Match reports whether the request path is the upload route.
	Match func(path string) bool
	// MediaTypes are the accepted Content-Type media types.
	MediaTypes []string
	// MaxBytes caps the body of the route.
	MaxBytes int64
}

// APIBody is LimitBody(n) + RequireJSON for every API request, except the
// uploads: a matching request must carry one of their media types and is
// capped at their MaxBytes instead.
func APIBody(n int64, uploads ...Upload) Middleware {
	jsonOnly := func(next http.Handler) http.Handler { return LimitBody(n)(RequireJSON(next)) }
	return func(next http.Handler) http.Handler {
		def := jsonOnly(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, u := range uploads {
				if r.Method != u.Method || !u.Match(r.URL.Path) {
					continue
				}
				mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || !slices.Contains(u.MediaTypes, strings.ToLower(mt)) {
					problem.Write(w, r, problem.UnsupportedMediaType,
						problem.Detail("Usa Content-Type: "+strings.Join(u.MediaTypes, " o ")+"."))
					return
				}
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, u.MaxBytes)
				}
				next.ServeHTTP(w, r)
				return
			}
			def.ServeHTTP(w, r)
		})
	}
}
