package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coste360/arenile/internal/domain/configerr"
	"github.com/coste360/arenile/internal/domain/establishment"
	"github.com/coste360/arenile/internal/http/api"
)

func TestParseIfMatch_Scenarios(t *testing.T) {
	s := func(v string) *string { return &v }
	tests := []struct {
		name    string
		in      *string
		want    int64 // 0 = nil
		wantErr bool
	}{
		{"absent", nil, 0, false},
		{"empty", s(""), 0, false},
		{"wildcard", s("*"), 0, false},
		{"bare", s("3"), 3, false},
		{"strong", s(`"12"`), 12, false},
		{"weak (downgraded by a compressing proxy, ADR 0008)", s(`W/"7"`), 7, false},
		{"garbage", s("abc"), 0, true},
		{"zero", s(`"0"`), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIfMatch(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tt.wantErr {
				if !errors.Is(err, configerr.ErrValidation) {
					t.Fatalf("want validation error, got %v", err)
				}
				return
			}
			if (got == nil) != (tt.want == 0) || (got != nil && *got != tt.want) {
				t.Fatalf("got %v, want %d", got, tt.want)
			}
		})
	}
}

func TestRequireIfMatch_Scenarios(t *testing.T) {
	if _, err := RequireIfMatch(nil); !errors.Is(err, configerr.ErrPreconditionRequired) {
		t.Fatalf("missing: err = %v", err)
	}
	empty := ""
	if _, err := RequireIfMatch(&empty); !errors.Is(err, configerr.ErrPreconditionRequired) {
		t.Fatalf("empty: err = %v", err)
	}
	v := `"4"`
	if got, err := RequireIfMatch(&v); err != nil || got.Any || got.Version != 4 {
		t.Fatalf("got %+v, %v", got, err)
	}
	// RFC 9110: "*" is satisfied by any current representation, so it is a
	// precondition, not a missing header (no 428).
	wildcard := "*"
	if got, err := RequireIfMatch(&wildcard); err != nil || !got.Any || got.Version != 0 {
		t.Fatalf("wildcard: got %+v, %v", got, err)
	}
	weak := `W/"4"`
	if got, err := RequireIfMatch(&weak); err != nil || got.Any || got.Version != 4 {
		t.Fatalf("weak: got %+v, %v", got, err)
	}
}

func TestPageParams_Scenarios(t *testing.T) {
	limit := func(n int) *api.Limit { return &n }
	cur := "abc"
	if c, n, err := pageParams(nil, nil); err != nil || c != "" || n != defaultPageSize {
		t.Fatalf("defaults: %q %d %v", c, n, err)
	}
	if c, n, err := pageParams(&cur, limit(10)); err != nil || c != "abc" || n != 10 {
		t.Fatalf("explicit: %q %d %v", c, n, err)
	}
	for _, bad := range []int{0, 201} {
		if _, _, err := pageParams(nil, limit(bad)); !errors.Is(err, configerr.ErrValidation) {
			t.Fatalf("limit %d: %v", bad, err)
		}
	}
	if nextCursor("") != nil || *nextCursor("x") != "x" {
		t.Fatal("nextCursor")
	}
}

func TestWriteVersioned_SetsETag(t *testing.T) {
	w := httptest.NewRecorder()
	writeVersioned(w, http.StatusOK, 5, map[string]string{"ok": "1"})
	if w.Header().Get("ETag") != `"5"` || w.Code != http.StatusOK {
		t.Fatalf("etag %q code %d", w.Header().Get("ETag"), w.Code)
	}
}

func TestNotImplemented_Problem(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/x", nil)
	w := httptest.NewRecorder()
	notImplemented(w, r)
	if w.Code != http.StatusNotImplemented || principalOf(r) != nil {
		t.Fatalf("code %d", w.Code)
	}
}

func TestSettingsToAPI_Defaults(t *testing.T) {
	out := SettingsToAPI(establishment.DefaultSettings())
	if out.Pricing.HalfDayFactor != "0.5" || out.Pricing.Rounding != api.Rounding("none") {
		t.Fatalf("pricing %+v", out.Pricing)
	}
	if out.DefaultPolicy.MaxLeadTimeDays == nil || *out.DefaultPolicy.MaxLeadTimeDays != 120 {
		t.Fatalf("policy %+v", out.DefaultPolicy)
	}
}
