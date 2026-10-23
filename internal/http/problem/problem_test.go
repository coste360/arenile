package problem

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/calendar"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/availability"
	"github.com/coste360/arenile/internal/domain/booking"
	"github.com/coste360/arenile/internal/domain/cash"
	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/notify"
	"github.com/coste360/arenile/internal/domain/payment/gateway"
	"github.com/coste360/arenile/internal/domain/policy"
	"github.com/coste360/arenile/internal/domain/pricing"
	"github.com/coste360/arenile/internal/reqctx"
	"github.com/coste360/arenile/internal/store"
	"github.com/coste360/arenile/pkg/ext"
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
		{"checkout not found", fmt.Errorf("x: %w", checkout.ErrSessionNotFound), 404, "checkout_not_found"},
		{"provider unavailable", fmt.Errorf("x: %w", gateway.ErrProviderUnavailable), 503, "payment_provider_unavailable"},
		{"webhook signature", gateway.ErrSignatureInvalid, 400, "webhook_signature_invalid"},
		{"webhook signature ext", ext.ErrInvalidSignature, 400, "webhook_signature_invalid"},
		{"terms outdated", checkout.ErrTermsOutdated, 409, "conflict"},
		{"link not found", notify.ErrLinkNotFound, 404, "not_found"},
		{"validation", configerr.Invalid("code", "obbligatorio"), 400, "validation_failed"},
		{"config conflict", fmt.Errorf("%w: duplicate code", configerr.ErrConflict), 409, "conflict"},
		{"config not found", configerr.ErrNotFound, 404, "not_found"},
		{"version conflict", &configerr.VersionConflictError{Current: 7}, 412, "version_conflict"},
		{"if-match required", configerr.ErrPreconditionRequired, 428, "precondition_required"},
		{"spots allocated", &configerr.SpotsAllocatedError{SpotIDs: []string{"s1"}}, 409, "spot_has_future_allocations"},
		{"no price", &pricing.NoPriceError{}, 422, "no_price"},
		{"policy violation", &policy.ViolationError{Violations: []policy.Violation{{Code: policy.CodeTooManySpots}}}, 422, "policy_violation"},
		{"not implemented", fmt.Errorf("x: %w", configerr.ErrNotImplemented), 501, "not_implemented"},
		{"spot conflict", &availability.ConflictError{Err: availability.ErrSpotUnavailable, Conflicts: []availability.Conflict{{SpotID: "s1", Date: calendar.MustParse("2027-07-01"), Slot: calendar.AM, Reason: availability.ReasonSpotUnavailable}}}, 409, "spot_unavailable"},
		{"pool exhausted", fmt.Errorf("tx: %w", &availability.ConflictError{Err: availability.ErrPoolExhausted}), 409, "pool_exhausted"},
		{"quota exceeded", availability.ErrQuotaExceeded, 409, "quota_exceeded"},
		{"hold expired", availability.ErrHoldExpired, 410, "hold_expired"},
		{"invalid transition", booking.Transition(booking.StatusCancelled, booking.StatusConfirmed), 409, "invalid_transition"},
		{"cash closed", cash.ErrSessionClosed, 409, "cash_session_closed"},
		{"cash open", fmt.Errorf("x: %w", cash.ErrSessionAlreadyOpen), 409, "cash_session_already_open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/x", nil)
			r = r.WithContext(reqctx.With(r.Context(), &reqctx.Info{RequestID: "req-1"}))
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

func TestError_ConfigExtensions(t *testing.T) {
	tests := []struct {
		name string
		err  error
		key  string
		want string
	}{
		{"current version", &configerr.VersionConflictError{Current: 7}, "current_version", "7"},
		{"spot ids", &configerr.SpotsAllocatedError{SpotIDs: []string{"s1", "s2"}}, "spot_ids", `["s1","s2"]`},
		{"violations", &policy.ViolationError{Violations: []policy.Violation{{Code: policy.CodeTooManySpots, Params: map[string]string{"max": "4"}}}}, "violations", `[{"code":"too_many_spots","params":{"max":"4"}}]`},
		{"field errors", configerr.Invalid("ranges[0].from", "data non valida"), "errors", `[{"field":"ranges[0].from","message":"data non valida"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPut, "/api/admin/v1/x", nil)
			w := httptest.NewRecorder()
			Error(w, r, nil, tt.err)
			var body map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if got := string(body[tt.key]); got != tt.want {
				t.Fatalf("%s = %s, want %s", tt.key, got, tt.want)
			}
		})
	}
}
