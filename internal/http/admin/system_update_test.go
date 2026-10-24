package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/update"
	"github.com/coste360/arenile/internal/http/api"
	"github.com/coste360/arenile/internal/updates"
)

func TestGetSystemUpdate_Scenarios(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version":"1.2.0","published_at":"2026-10-20T08:00:00Z","release_notes":"https://github.com/coste360/arenile/releases/tag/v1.2.0","files":[]}`)
	}))
	defer srv.Close()
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	svc := update.NewService(update.Deps{
		Fetcher: updates.Checker{URL: srv.URL, Current: "1.1.0", Client: srv.Client()},
		Current: "1.1.0", HowTo: update.HowDesktop, GOOS: "linux", GOARCH: "amd64", Clock: c,
		Enabled: func(context.Context) bool { return true },
	})
	c.Advance(update.StartDelay)
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{Updates: svc}
	role := func(r auth.Role) *auth.Principal {
		return &auth.Principal{Kind: auth.PrincipalUser, UserID: "u", Grants: []auth.Grant{{OrganizationID: "o", Role: r}}}
	}
	tests := []struct {
		name string
		p    *auth.Principal
		code int
	}{
		{"owner", role(auth.RoleOwner), http.StatusOK},
		{"manager", role(auth.RoleManager), http.StatusOK},
		{"cashier", role(auth.RoleCashier), http.StatusForbidden},
		{"anonymous", nil, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system/update", nil)
			if tt.p != nil {
				r = r.WithContext(auth.WithPrincipal(r.Context(), tt.p))
			}
			w := httptest.NewRecorder()
			h.GetSystemUpdate(w, r)
			if w.Code != tt.code {
				t.Fatalf("status %d, want %d: %s", w.Code, tt.code, w.Body)
			}
			if tt.code != http.StatusOK {
				return
			}
			var got api.SystemUpdate
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !got.UpdateAvailable || got.Latest == nil || *got.Latest != "1.2.0" || got.Current != "1.1.0" ||
				got.HowToUpdate != api.SystemUpdateHowToUpdateDesktop || !got.CheckEnabled ||
				got.DownloadUrl == nil || *got.DownloadUrl != updates.DownloadPage || got.CheckedAt == nil || got.ReleaseNotesUrl == nil {
				t.Fatalf("body %s", w.Body)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing Cache-Control: no-store")
			}
		})
	}
}

func TestGetSystemUpdate_NotWired(t *testing.T) {
	w := httptest.NewRecorder()
	(&Handlers{}).GetSystemUpdate(w, httptest.NewRequest(http.MethodGet, "/api/admin/v1/system/update", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status %d", w.Code)
	}
}
