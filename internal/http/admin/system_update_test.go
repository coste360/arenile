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

// "Verifica aggiornamenti": the first check runs on demand (before the
// background job's StartDelay); pending is true before it, false after.
func TestCheckSystemUpdate_Scenarios(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		fmt.Fprint(w, `{"version":"1.2.0","published_at":"2026-10-20T08:00:00Z","release_notes":"https://github.com/coste360/arenile/releases/tag/v1.2.0","files":[]}`)
	}))
	defer srv.Close()
	svc := update.NewService(update.Deps{
		Fetcher: updates.Checker{URL: srv.URL, Current: "1.1.0", Client: srv.Client()},
		Current: "1.1.0", HowTo: update.HowDesktop, GOOS: "darwin", GOARCH: "arm64",
		Clock:   clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)),
		Enabled: func(context.Context) bool { return true },
	})
	h := &Handlers{Updates: svc}
	owner := &auth.Principal{Kind: auth.PrincipalUser, UserID: "u", Grants: []auth.Grant{{OrganizationID: "o", Role: auth.RoleOwner}}}
	call := func(method string, p *auth.Principal) (*httptest.ResponseRecorder, api.SystemUpdate) {
		t.Helper()
		r := httptest.NewRequest(method, "/api/admin/v1/system/update/check", nil)
		if p != nil {
			r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		}
		w := httptest.NewRecorder()
		if method == http.MethodGet {
			h.GetSystemUpdate(w, r)
		} else {
			h.CheckSystemUpdate(w, r, api.CheckSystemUpdateParams{})
		}
		var got api.SystemUpdate
		if w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
		}
		return w, got
	}
	if w, got := call(http.MethodGet, owner); w.Code != http.StatusOK || !got.Pending || got.LastAttemptAt != nil ||
		got.Os == nil || *got.Os != "darwin" || got.Arch == nil || *got.Arch != "arm64" {
		t.Fatalf("before: %d %s", w.Code, w.Body)
	}
	w, got := call(http.MethodPost, owner)
	if w.Code != http.StatusOK || got.Pending || got.LastAttemptAt == nil || !got.UpdateAvailable || hits != 1 ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("check: %d %s hits %d", w.Code, w.Body, hits)
	}
	if w, _ := call(http.MethodPost, owner); w.Code != http.StatusOK || hits != 1 {
		t.Fatalf("debounced: %d hits %d", w.Code, hits)
	}
	cashier := &auth.Principal{Kind: auth.PrincipalUser, UserID: "c", Grants: []auth.Grant{{OrganizationID: "o", Role: auth.RoleCashier}}}
	if w, _ := call(http.MethodPost, cashier); w.Code != http.StatusForbidden {
		t.Fatalf("cashier: %d", w.Code)
	}
	if w, _ := call(http.MethodPost, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
	nw := httptest.NewRecorder()
	(&Handlers{}).CheckSystemUpdate(nw, httptest.NewRequest(http.MethodPost, "/api/admin/v1/system/update/check", nil), api.CheckSystemUpdateParams{})
	if nw.Code != http.StatusNotImplemented {
		t.Fatalf("not wired: %d", nw.Code)
	}
}
