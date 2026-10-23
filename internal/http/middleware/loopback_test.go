package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopbackOnly_Scenarios(t *testing.T) {
	h := LoopbackOnly()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	tests := []struct {
		name, remote, host string
		xff                string
		want               int
	}{
		{"ipv4 loopback", "127.0.0.1:5000", "127.0.0.1:18190", "", 204},
		{"ipv6 loopback", "[::1]:5000", "[::1]:18190", "", 204},
		{"localhost host", "127.0.0.1:5000", "localhost:18190", "", 204},
		{"lan peer", "192.168.1.20:5000", "127.0.0.1:18190", "", 403},
		{"forwarded header ignored", "192.168.1.20:5000", "127.0.0.1", "127.0.0.1", 403},
		{"dns rebinding host", "127.0.0.1:5000", "evil.example:18190", "", 403},
		{"lan host name", "127.0.0.1:5000", "192.168.1.2:8443", "", 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/desktop/v1/first-run", nil)
			r.RemoteAddr, r.Host = tt.remote, tt.host
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d", w.Code, tt.want)
			}
		})
	}
}
