package problem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestError_ClientCancelled: a request cancelled by the client is answered
// 499 and logged at debug level only, never as "request failed".
func TestError_ClientCancelled(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		cancel     bool
		wantStatus int
	}{
		{"wrapped context.Canceled", fmt.Errorf("store: list: %w", context.Canceled), false, StatusClientClosedRequest},
		{"driver error on a cancelled request", errors.New("sql: connection closed"), true, StatusClientClosedRequest},
		{"server deadline stays a 500", fmt.Errorf("x: %w", context.DeadlineExceeded), false, http.StatusInternalServerError},
		{"other errors stay 500", errors.New("boom"), false, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/x", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			Error(rec, req, log, tt.err)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", rec.Code, tt.wantStatus)
			}
			logged := strings.Contains(buf.String(), "request failed")
			if wantLog := tt.wantStatus == http.StatusInternalServerError; logged != wantLog {
				t.Fatalf("error logged = %v, want %v (%s)", logged, wantLog, buf.String())
			}
		})
	}
}
