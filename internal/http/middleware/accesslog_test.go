package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coste360/arenile/internal/http/problem"
)

// TestAccessLog_ClientCancelledIsNotAnError: a 499 (client went away) is
// logged at debug level, a 500 at error level.
func TestAccessLog_ClientCancelledIsNotAnError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel string
	}{
		{"cancelled", context.Canceled, "level=DEBUG"},
		{"server error", context.DeadlineExceeded, "level=ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			h := AccessLog(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				problem.Error(w, r, nil, tt.err)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/v1/x", nil))
			var line string
			for _, l := range strings.Split(buf.String(), "\n") {
				if strings.Contains(l, `msg="http request"`) {
					line = l
				}
			}
			if !strings.Contains(line, tt.wantLevel) {
				t.Fatalf("access log %q, want %s", line, tt.wantLevel)
			}
		})
	}
}
