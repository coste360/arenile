package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/desktop/control"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/domain/update"
)

// desktopSettings is a desktop control with a given update_check setting.
type desktopSettings struct {
	fakeDesktop
	updateCheck bool
}

func (d desktopSettings) Settings(context.Context) (control.Settings, error) {
	return control.Settings{UpdateCheck: d.updateCheck}, nil
}

// Invariant (ADR 0025): no network call when ARENILE_UPDATE_CHECK=false on
// the server edition, or update_check is off in the desktop app; with the
// check on, one GET whose User-Agent names version, edition and OS/arch.
func TestUpdates_WiringAndNoNetworkWhenDisabled(t *testing.T) {
	tests := []struct {
		name      string
		check     bool
		install   string
		desktop   control.Control
		wantHits  int32
		wantHowTo update.HowTo
		wantUA    string
	}{
		{"server off", false, "", nil, 0, update.HowBinary, ""},
		{"server on", true, "", nil, 1, update.HowBinary, "(server; "},
		{"docker on", true, "docker", nil, 1, update.HowDocker, "(server; "},
		{"desktop setting off", true, "", desktopSettings{updateCheck: false}, 0, update.HowDesktop, ""},
		{"desktop setting on", true, "", desktopSettings{updateCheck: true}, 1, update.HowDesktop, "(desktop; "},
		{"desktop setting on, env off", false, "", desktopSettings{updateCheck: true}, 0, update.HowDesktop, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if !strings.HasPrefix(r.UserAgent(), "arenile/1.0.0 "+tt.wantUA) || r.Header.Get("Cookie") != "" {
					t.Errorf("User-Agent %q", r.UserAgent())
				}
				fmt.Fprint(w, `{"version":"1.1.0"}`)
			}))
			defer srv.Close()
			cfg := devConfig(t)
			cfg.UpdateCheck, cfg.UpdateURL = tt.check, srv.URL
			if tt.install != "" {
				cfg.Install = tt.install
			}
			c := clock.NewFake(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
			opts := []Option{
				WithVersion("v1.0.0"), WithClock(c), WithoutSignalHandling(),
				WithLogger(slog.New(slog.DiscardHandler)), WithUpdateHTTPClient(srv.Client()),
			}
			if tt.desktop != nil {
				opts = append(opts, WithDesktop(tt.desktop))
			}
			a, err := New(cfg, opts...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.Close() })
			for range 3 {
				c.Advance(update.Interval)
				if err := a.Jobs().RunNow(context.Background(), update.JobName); err != nil {
					t.Fatal(err)
				}
			}
			if got := hits.Load(); got != tt.wantHits*3 {
				t.Fatalf("network calls = %d, want %d", got, tt.wantHits*3)
			}
			owner := &auth.Principal{Kind: auth.PrincipalUser, UserID: "u", Grants: []auth.Grant{{OrganizationID: "o", Role: auth.RoleOwner}}}
			st, err := a.Updates.Status(context.Background(), owner)
			if err != nil || st.HowTo != tt.wantHowTo || st.UpdateAvailable != (tt.wantHits > 0) || st.Current != "v1.0.0" {
				t.Fatalf("status %+v %v", st, err)
			}
		})
	}
}
