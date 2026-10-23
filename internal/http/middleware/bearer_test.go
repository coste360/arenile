package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/device"
)

// stubCreds authenticates one session cookie, one API key and one device
// token (M6/M7 bearer strategy, spec 29.2).
type stubCreds struct{}

var (
	stubDeviceToken = "ar_dev_abcdefgh_" + strings.Repeat("b", 52)
	stubAPIKey      = "ar_live_abcdefgh_" + strings.Repeat("c", 40)
)

func (stubCreds) AuthenticateSession(_ context.Context, token string) (*auth.Principal, error) {
	if token == "good-session" {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: "cookie-user"}, nil
	}
	return nil, auth.ErrUnauthenticated
}

func (stubCreds) AuthenticateAPIKey(_ context.Context, key string) (*auth.Principal, error) {
	if key == stubAPIKey {
		return &auth.Principal{Kind: auth.PrincipalAPIKey, APIKeyID: "key"}, nil
	}
	return nil, auth.ErrUnauthenticated
}

func (stubCreds) AuthenticateDevice(_ context.Context, token string) (*auth.Principal, error) {
	if token == stubDeviceToken {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: "device-user", DeviceID: "dev-1"}, nil
	}
	return nil, auth.ErrUnauthenticated
}

// TestAuthenticate_BearerAndCookie runs the /api/admin chain (Authenticate,
// RequireAuth, CSRF) with device.BearerAuthenticator: device tokens and API
// keys authenticate through Authorization and are exempt from CSRF; cookie
// sessions need the double-submit token on unsafe methods; a bearer wins
// over a cookie sent along.
func TestAuthenticate_BearerAndCookie(t *testing.T) {
	a := device.BearerAuthenticator{Auth: stubCreds{}, Devices: stubCreds{}}
	chain := OnPrefix("/",
		Authenticate(a, nil),
		RequireAuth(nil, "/auth/"),
		CSRF(false, time.Hour),
	)
	var who string
	h := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFrom(r.Context())
		who = p.UserID + "|" + p.DeviceID + "|" + p.APIKeyID
		w.WriteHeader(http.StatusNoContent)
	}))
	const csrf = "0123456789abcdef0123456789abcdef-csrf"
	tests := []struct {
		name    string
		method  string
		authz   string
		cookie  string
		csrf    string
		want    int
		wantWho string
	}{
		{"device token GET", http.MethodGet, "Bearer " + stubDeviceToken, "", "", 204, "device-user|dev-1|"},
		{"device token POST without csrf", http.MethodPost, "Bearer " + stubDeviceToken, "", "", 204, "device-user|dev-1|"},
		{"device token lowercase scheme", http.MethodDelete, "bearer " + stubDeviceToken, "", "", 204, "device-user|dev-1|"},
		{"api key POST without csrf", http.MethodPost, "Bearer " + stubAPIKey, "", "", 204, "||key"},
		{"unknown device token", http.MethodGet, "Bearer ar_dev_abcdefgh_" + strings.Repeat("z", 52), "", "", 401, ""},
		{"device token prefix routed to devices only", http.MethodGet, "Bearer ar_dev_" + stubAPIKey, "", "", 401, ""},
		{"basic scheme", http.MethodGet, "Basic Zm9vOmJhcg==", "", "", 401, ""},
		{"bearer wins over cookie", http.MethodPost, "Bearer " + stubDeviceToken, "good-session", "", 204, "device-user|dev-1|"},
		{"invalid bearer with valid cookie", http.MethodGet, "Bearer nope", "good-session", "", 401, ""},
		{"cookie GET", http.MethodGet, "", "good-session", "", 204, "cookie-user||"},
		{"cookie POST without csrf", http.MethodPost, "", "good-session", "", 403, ""},
		{"cookie POST wrong csrf", http.MethodPost, "", "good-session", "x" + csrf[1:], 403, ""},
		{"cookie POST with csrf", http.MethodPost, "", "good-session", csrf, 204, "cookie-user||"},
		{"no credentials", http.MethodGet, "", "", "", 401, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			who = ""
			r := httptest.NewRequest(tt.method, "/api/admin/v1/x", nil)
			if tt.authz != "" {
				r.Header.Set("Authorization", tt.authz)
			}
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: SessionCookie, Value: tt.cookie})
				r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: csrf})
			}
			if tt.csrf != "" {
				r.Header.Set(CSRFHeader, tt.csrf)
			}
			w := httptest.NewRecorder()
			RequestContext(nil)(h).ServeHTTP(w, r)
			if w.Code != tt.want || who != tt.wantWho {
				t.Fatalf("status %d who %q, want %d %q (%s)", w.Code, who, tt.want, tt.wantWho, w.Body)
			}
		})
	}
}

// TestAuthenticate_DeviceOnly: /api/device accepts device tokens only.
func TestAuthenticate_DeviceOnly(t *testing.T) {
	h := OnPrefix("/", Authenticate(device.DeviceOnly{Devices: stubCreds{}}, nil), RequireAuth([]string{"/api/device/v1/pair"}, ""))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	tests := []struct {
		name, path, authz, cookie string
		want                      int
	}{
		{"device token", "/api/device/v1/self", "Bearer " + stubDeviceToken, "", 204},
		{"api key refused", "/api/device/v1/self", "Bearer " + stubAPIKey, "", 401},
		{"cookie ignored", "/api/device/v1/self", "", "good-session", 401},
		{"pair is public", "/api/device/v1/pair", "", "", 204},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodPost, tt.path, nil)
		if tt.authz != "" {
			r.Header.Set("Authorization", tt.authz)
		}
		if tt.cookie != "" {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: tt.cookie})
		}
		w := httptest.NewRecorder()
		RequestContext(nil)(h).ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, w.Code, tt.want)
		}
	}
}
