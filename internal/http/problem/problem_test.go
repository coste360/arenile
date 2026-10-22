package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/http/httpctx"
	"github.com/coste360/arenile/internal/store"
)

func TestError_Mapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		typ    string
	}{
		{"unauthenticated", auth.ErrUnauthenticated, 401, "unauthorized"},
		{"credentials", auth.ErrInvalidCredentials, 401, "invalid_credentials"},
		{"totp required", auth.ErrTOTPRequired, 401, "totp_required"},
		{"forbidden wrapped", fmt.Errorf("x: %w", auth.ErrForbidden), 403, "forbidden"},
		{"not found", store.ErrNotFound, 404, "not_found"},
		{"conflict", store.ErrConflict, 409, "conflict"},
		{"rate limited", &auth.RateLimitError{RetryAfter: 1500 * time.Millisecond}, 429, "rate_limited"},
		{"unknown", errors.New("db exploded: secret detail"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/x", nil)
			r = r.WithContext(httpctx.With(r.Context(), &httpctx.Info{RequestID: "req-1"}))
			w := httptest.NewRecorder()
			Error(w, r, nil, tt.err)
			if w.Code != tt.status {
				t.Fatalf("status %d", w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != ContentType {
				t.Fatalf("content type %q", ct)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["type"] != tt.typ || body["status"].(float64) != float64(tt.status) || body["title"] == "" ||
				body["instance"] != "/api/admin/v1/x" || body["request_id"] != "req-1" {
				t.Fatalf("body %v", body)
			}
			if tt.status == 500 && w.Body.String() != "" && contains(w.Body.String(), "secret detail") {
				t.Fatal("internal errors must not leak details")
			}
			if tt.status == 429 && (w.Header().Get("Retry-After") != "2" || body["retry_after"].(float64) != 2) {
				t.Fatalf("retry-after %q %v", w.Header().Get("Retry-After"), body["retry_after"])
			}
		})
	}
}

func TestCatalog_MatchesOpenAPIEnum(t *testing.T) {
	for typ, m := range catalog {
		if !typ.Valid() {
			t.Fatalf("type %q is not declared in the ProblemType enum of api/openapi.yaml", typ)
		}
		if m.status < 400 || m.title == "" {
			t.Fatalf("type %q has incomplete metadata", typ)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
