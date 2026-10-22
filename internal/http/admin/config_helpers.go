package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/http/problem"
)

// Shared helpers of the configuration handlers (M1/M2). Area files
// (seasons.go, maps.go, ...) use them; add area-specific helpers to the area
// file itself, never here, to avoid edit collisions.

// Page size bounds of admin collections (spec 17.1).
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// notImplemented answers 501 not_implemented.
func notImplemented(w http.ResponseWriter, r *http.Request) {
	problem.Write(w, r, problem.NotImplemented)
}

// principalOf returns the authenticated principal (nil when absent: domain
// services answer auth.ErrUnauthenticated).
func principalOf(r *http.Request) *auth.Principal {
	p, _ := auth.PrincipalFrom(r.Context())
	return p
}

// ETag formats a resource version as a strong entity tag: "<version>".
func ETag(version int64) string { return `"` + strconv.FormatInt(version, 10) + `"` }

// writeVersioned writes v as JSON with the ETag header of version.
func writeVersioned(w http.ResponseWriter, status int, version int64, v any) {
	w.Header().Set("ETag", ETag(version))
	writeJSON(w, status, v)
}

// ParseIfMatch parses an If-Match header value into a version. It accepts
// "3", "\"3\"" and the weak form W/"3". A missing/empty header or "*" yields
// nil (no precondition). Malformed values are a validation error.
func ParseIfMatch(h *string) (*int64, error) {
	if h == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*h)
	if s == "" || s == "*" {
		return nil, nil
	}
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 1 {
		return nil, configerr.Invalid("If-Match", "Versione non valida: usa il valore dell'header ETag.")
	}
	return &v, nil
}

// RequireIfMatch is ParseIfMatch for operations where the precondition is
// mandatory (PUT /maps/{id}): a missing header is configerr.ErrPreconditionRequired.
func RequireIfMatch(h *string) (int64, error) {
	v, err := ParseIfMatch(h)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, configerr.ErrPreconditionRequired
	}
	return *v, nil
}

// pageParams validates cursor/limit query parameters.
func pageParams(cursor *api.Cursor, limit *api.Limit) (string, int, error) {
	n := defaultPageSize
	if limit != nil {
		if *limit < 1 || *limit > maxPageSize {
			return "", 0, configerr.Invalid("limit", "Deve essere tra 1 e 200.")
		}
		n = *limit
	}
	c := ""
	if cursor != nil {
		c = *cursor
	}
	return c, n, nil
}

// nextCursor maps an empty cursor to nil (field omitted).
func nextCursor(c string) *string {
	if c == "" {
		return nil
	}
	return &c
}
