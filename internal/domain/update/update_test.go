package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/updates"
)

const latestJSON = `{
  "version": "1.2.0", "tag": "v1.2.0", "published_at": "2026-10-20T08:00:00Z",
  "release_notes": "https://github.com/coste360/arenile/releases/tag/v1.2.0",
  "files": [
    {"name": "Arenile-1.2.0_setup.exe", "edition": "desktop", "os": "windows", "arch": "x64", "url": "https://arenile.app/download/1.2.0/Arenile-1.2.0_setup.exe"},
    {"name": "arenile_1.2.0_linux_amd64.tar.gz", "edition": "server", "os": "linux", "arch": "amd64", "url": "https://arenile.app/download/1.2.0/arenile_1.2.0_linux_amd64.tar.gz"}
  ]
}`

var owner = &auth.Principal{Kind: auth.PrincipalUser, UserID: "u", Grants: []auth.Grant{{OrganizationID: "o", Role: auth.RoleOwner}}}

type fakeServer struct {
	*httptest.Server
	hits   atomic.Int32
	status atomic.Int32
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.status.Store(http.StatusOK)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.hits.Add(1)
		if s := int(f.status.Load()); s != http.StatusOK {
			w.WriteHeader(s)
			return
		}
		fmt.Fprint(w, latestJSON)
	}))
	t.Cleanup(f.Close)
	return f
}

func newService(f *fakeServer, c *clock.Fake, current string, how HowTo, enabled *atomic.Bool, onNewer func(string)) *Service {
	return NewService(Deps{
		Fetcher: updates.Checker{URL: f.URL, Current: current, Edition: "server", GOOS: "linux", GOARCH: "amd64", Client: f.Client()},
		Current: current, HowTo: how, GOOS: "windows", GOARCH: "amd64", Clock: c,
		Enabled: func(context.Context) bool { return enabled.Load() }, OnNewer: onNewer,
	})
}

func TestRefresh_Schedule(t *testing.T) {
	f := newFakeServer(t)
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	var on atomic.Bool
	on.Store(true)
	var notified []string
	s := newService(f, c, "v1.1.0", HowDesktop, &on, func(v string) { notified = append(notified, v) })
	ctx := context.Background()

	steps := []struct {
		advance  time.Duration
		wantHits int32
	}{
		{0, 0},                      // start-up: wait StartDelay
		{StartDelay, 1},             // first check
		{time.Hour, 1},              // not due
		{Interval - time.Hour, 2},   // daily check
		{Interval - time.Minute, 2}, // not yet
		{time.Minute, 3},            // due again
	}
	for i, st := range steps {
		c.Advance(st.advance)
		if err := s.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
		if got := f.hits.Load(); got != st.wantHits {
			t.Fatalf("step %d: hits = %d, want %d", i, got, st.wantHits)
		}
	}
	if len(notified) != 1 || notified[0] != "1.2.0" {
		t.Fatalf("notified %v, want once 1.2.0", notified)
	}
}

func TestRefresh_FailureIsSilentAndRetried(t *testing.T) {
	f := newFakeServer(t)
	f.status.Store(http.StatusBadGateway)
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	var on atomic.Bool
	on.Store(true)
	s := newService(f, c, "1.1.0", HowDocker, &on, nil)
	c.Advance(StartDelay)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("network errors must not surface: %v", err)
	}
	st, err := s.Status(context.Background(), owner)
	if err != nil || st.UpdateAvailable || st.Latest != "" || !st.CheckEnabled {
		t.Fatalf("status after failure %+v %v", st, err)
	}
	c.Advance(RetryAfter - time.Minute)
	_ = s.Refresh(context.Background())
	if f.hits.Load() != 1 {
		t.Fatalf("retried too early: %d", f.hits.Load())
	}
	f.status.Store(http.StatusOK)
	c.Advance(time.Minute)
	_ = s.Refresh(context.Background())
	st, _ = s.Status(context.Background(), owner)
	if f.hits.Load() != 2 || !st.UpdateAvailable || st.Latest != "1.2.0" || st.DownloadURL != "" || st.HowTo != HowDocker {
		t.Fatalf("after retry: hits %d, status %+v", f.hits.Load(), st)
	}
}

// Invariant: no network call when the update check is disabled
// (ARENILE_UPDATE_CHECK=false or update_check off) or the version is a
// development build.
func TestRefresh_NoNetworkWhenDisabledOrDev(t *testing.T) {
	tests := []struct {
		name    string
		current string
		enabled bool
	}{
		{"disabled", "1.1.0", false},
		{"dev", "dev", true},
		{"dev build", "1.2.0-dev.12+abc", true},
		{"dirty dev build", "1.2.0-dev.12+abc.dirty", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
			var on atomic.Bool
			on.Store(tt.enabled)
			s := newService(f, c, tt.current, HowBinary, &on, nil)
			for range 5 {
				c.Advance(Interval)
				_ = s.Refresh(context.Background())
			}
			if f.hits.Load() != 0 {
				t.Fatalf("network calls: %d", f.hits.Load())
			}
			st, err := s.Status(context.Background(), owner)
			if err != nil || st.UpdateAvailable || st.CheckEnabled != tt.enabled {
				t.Fatalf("status %+v %v", st, err)
			}
		})
	}
}

func TestStatus_DownloadURL(t *testing.T) {
	tests := []struct {
		how  HowTo
		goos string
		want string
	}{
		{HowDesktop, "windows", "https://arenile.app/download/1.2.0/Arenile-1.2.0_setup.exe"},
		{HowDesktop, "darwin", updates.DownloadPage},
		{HowBinary, "linux", "https://arenile.app/download/1.2.0/arenile_1.2.0_linux_amd64.tar.gz"},
		{HowDocker, "linux", ""},
	}
	for _, tt := range tests {
		f := newFakeServer(t)
		c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
		s := NewService(Deps{
			Fetcher: updates.Checker{URL: f.URL, Current: "1.1.0", Client: f.Client()},
			Current: "1.1.0", HowTo: tt.how, GOOS: tt.goos, GOARCH: "amd64", Clock: c,
			Enabled: func(context.Context) bool { return true },
		})
		c.Advance(StartDelay)
		_ = s.Refresh(context.Background())
		st, err := s.Status(context.Background(), owner)
		if err != nil || st.DownloadURL != tt.want || st.ReleaseNotesURL == "" || st.CheckedAt.IsZero() || st.PublishedAt.IsZero() {
			t.Errorf("%s/%s: %+v %v", tt.how, tt.goos, st, err)
		}
	}
}

func TestStatus_NotNewer(t *testing.T) {
	f := newFakeServer(t)
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	var on atomic.Bool
	on.Store(true)
	s := newService(f, c, "v1.2.0", HowDesktop, &on, func(string) { t.Error("must not notify") })
	c.Advance(StartDelay)
	_ = s.Refresh(context.Background())
	st, _ := s.Status(context.Background(), owner)
	if st.UpdateAvailable || st.Latest != "1.2.0" {
		t.Fatalf("%+v", st)
	}
	// Turning the check off hides what was cached.
	on.Store(false)
	st, _ = s.Status(context.Background(), owner)
	if st.Latest != "" || st.CheckEnabled {
		t.Fatalf("disabled: %+v", st)
	}
}

func TestStatus_Permissions(t *testing.T) {
	user := func(roles ...auth.Role) *auth.Principal {
		p := &auth.Principal{Kind: auth.PrincipalUser, UserID: "u"}
		for _, r := range roles {
			p.Grants = append(p.Grants, auth.Grant{OrganizationID: "o", EstablishmentID: "e", Role: r})
		}
		return p
	}
	totp := user(auth.RoleOwner)
	totp.TOTPEnrolmentRequired = true
	tests := []struct {
		name string
		p    *auth.Principal
		want error
	}{
		{"owner", user(auth.RoleOwner), nil},
		{"manager", user(auth.RoleManager), nil},
		{"cashier and manager", user(auth.RoleCashier, auth.RoleManager), nil},
		{"cashier", user(auth.RoleCashier), auth.ErrForbidden},
		{"viewer", user(auth.RoleViewer), auth.ErrForbidden},
		{"api key", &auth.Principal{Kind: auth.PrincipalAPIKey, Scopes: []auth.Permission{auth.PermEstablishmentConfigure}}, auth.ErrForbidden},
		{"totp enrolment pending", totp, auth.ErrForbidden},
		{"anonymous", nil, auth.ErrUnauthenticated},
	}
	s := NewService(Deps{Current: "1.0.0"})
	for _, tt := range tests {
		if _, err := s.Status(context.Background(), tt.p); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}

// The switch (a settings file read on desktop) is read only when a check
// is due, not at every tick of the job.
func TestRefresh_ReadsSwitchOnlyWhenDue(t *testing.T) {
	f := newFakeServer(t)
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	var reads atomic.Int32
	s := NewService(Deps{
		Fetcher: updates.Checker{URL: f.URL, Current: "1.1.0", Client: f.Client()},
		Current: "1.1.0", Clock: c,
		Enabled: func(context.Context) bool { reads.Add(1); return true },
	})
	for range 10 { // ticks before StartDelay and between daily checks
		c.Advance(Tick / 4)
		_ = s.Refresh(context.Background())
	}
	if reads.Load() != 1 || f.hits.Load() != 1 {
		t.Fatalf("reads=%d hits=%d, want 1 and 1", reads.Load(), f.hits.Load())
	}
	for range 30 {
		c.Advance(Tick)
		_ = s.Refresh(context.Background())
	}
	if reads.Load() != 1 {
		t.Fatalf("switch read %d times between checks", reads.Load())
	}
}

// A failed check is never an error: status "unavailable", and the last
// successful result is kept for MaxAge, then dropped.
func TestStatus_States(t *testing.T) {
	f := newFakeServer(t)
	c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	var on atomic.Bool
	on.Store(true)
	s := newService(f, c, "1.1.0", HowDesktop, &on, nil)
	ctx := context.Background()
	state := func() Status {
		t.Helper()
		st, err := s.Status(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	if st := state(); st.State != StateOK || st.Latest != "" {
		t.Fatalf("before the first check: %+v", st)
	}
	c.Advance(StartDelay)
	_ = s.Refresh(ctx)
	if st := state(); st.State != StateOK || !st.UpdateAvailable {
		t.Fatalf("after success: %+v", st)
	}
	f.status.Store(http.StatusServiceUnavailable)
	c.Advance(Interval)
	_ = s.Refresh(ctx)
	if st := state(); st.State != StateUnavailable || !st.UpdateAvailable || st.Latest != "1.2.0" {
		t.Fatalf("failure within MaxAge keeps the result: %+v", st)
	}
	c.Advance(MaxAge)
	_ = s.Refresh(ctx)
	if st := state(); st.State != StateUnavailable || st.UpdateAvailable || st.Latest != "" {
		t.Fatalf("failure after MaxAge drops the result: %+v", st)
	}
	on.Store(false)
	if st := state(); st.State != StateDisabled {
		t.Fatalf("disabled: %+v", st)
	}
	dev := NewService(Deps{Current: "dev", Enabled: func(context.Context) bool { return true }})
	if st, _ := dev.Status(ctx, owner); st.State != StateDisabled || !st.CheckEnabled {
		t.Fatalf("dev build: %+v", st)
	}
}
