// Package update backs the "new version available" notice of the panel
// (ADR 0025, GET /api/admin/v1/system/update).
//
// A background job (Jobs) reads the public release description once a day
// through a Fetcher (internal/updates.Checker) and keeps the result in
// memory; requests are answered from that copy and never wait on the
// network. Rules:
//   - no network call at all while Enabled() is false
//     (ARENILE_UPDATE_CHECK=false, or update_check off in the desktop app);
//   - no network call when the running version is not a release ("dev",
//     "-dev." builds of scripts/version.sh): developers are never nagged;
//   - first check StartDelay after start-up, then every Interval with the
//     injected clock; a failed check is retried after RetryAfter;
//   - network errors are logged at debug level and never reach users: the
//     status becomes StateUnavailable (a neutral line in the panel, never
//     an error) and the last successful result is kept for MaxAge.
package update

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/coste360/arenile/internal/clock"
	"github.com/coste360/arenile/internal/domain/auth"
	"github.com/coste360/arenile/internal/jobs"
	"github.com/coste360/arenile/internal/updates"
)

// Scheduling of the background check.
const (
	StartDelay = 30 * time.Second
	Interval   = 24 * time.Hour
	RetryAfter = time.Hour
	// Tick is how often the job looks at the clock; it makes a network
	// call only when a check is due.
	Tick = time.Minute
	// MaxAge is how long a successful result is kept when later checks fail.
	MaxAge = 7 * 24 * time.Hour
)

// State is the outcome of the check (api.SystemUpdate.status).
type State string

// States.
const (
	// StateOK: the last check succeeded, or the first one has not run yet.
	StateOK State = "ok"
	// StateUnavailable: the last check failed (network, timeout, bad
	// response). Never shown as an error.
	StateUnavailable State = "unavailable"
	// StateDisabled: the check is off, or the version is not a release.
	StateDisabled State = "disabled"
)

// JobName is the name of the background job.
const JobName = "updates.check"

// HowTo tells how this installation is updated (api.SystemUpdate.how_to_update).
type HowTo string

// Update methods.
const (
	HowDesktop HowTo = "desktop"
	HowDocker  HowTo = "docker"
	HowBinary  HowTo = "binary"
)

// Fetcher reads the latest release (internal/updates.Checker).
type Fetcher interface {
	Fetch(ctx context.Context) (updates.Release, error)
}

// Deps are the dependencies of Service.
type Deps struct {
	Fetcher Fetcher
	// Current is the running version.
	Current string
	HowTo   HowTo
	// GOOS and GOARCH pick the package for download_url.
	GOOS, GOARCH string
	Clock        clock.Clock
	// Enabled reports whether the check is on; read before every check.
	Enabled func() bool
	// OnNewer, optional, is called once per newer version found (desktop
	// native notification).
	OnNewer func(latest string)
	Log     *slog.Logger
}

// Status mirrors api.SystemUpdate.
type Status struct {
	Current         string
	State           State
	Latest          string
	UpdateAvailable bool
	PublishedAt     time.Time
	ReleaseNotesURL string
	DownloadURL     string
	HowTo           HowTo
	CheckEnabled    bool
	CheckedAt       time.Time
}

// Service caches the latest release.
type Service struct {
	d       Deps
	started time.Time

	mu          sync.Mutex
	release     *updates.Release
	checkedAt   time.Time
	lastAttempt time.Time
	lastFailed  bool
	notified    string
}

// NewService returns a Service. A nil Enabled means disabled.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Enabled == nil {
		d.Enabled = func() bool { return false }
	}
	if d.HowTo == "" {
		d.HowTo = HowBinary
	}
	return &Service{d: d, started: d.Clock.Now()}
}

// Jobs returns the background check.
func (s *Service) Jobs() []jobs.Job {
	return []jobs.Job{{Name: JobName, Interval: Tick, RunOnStart: true, Timeout: 30 * time.Second, Run: s.Refresh}}
}

// Refresh checks for a new version when a check is due. It never returns
// network errors (logged at debug): the job must not report them.
func (s *Service) Refresh(ctx context.Context) error {
	if s.d.Fetcher == nil || !updates.IsRelease(s.d.Current) || !s.d.Enabled() {
		return nil
	}
	now := s.d.Clock.Now()
	s.mu.Lock()
	due := false
	switch {
	case s.lastAttempt.IsZero():
		due = now.Sub(s.started) >= StartDelay
	case s.lastFailed:
		due = now.Sub(s.lastAttempt) >= RetryAfter
	default:
		due = now.Sub(s.lastAttempt) >= Interval
	}
	if due {
		s.lastAttempt = now
	}
	s.mu.Unlock()
	if !due {
		return nil
	}

	rel, err := s.d.Fetcher.Fetch(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastFailed = true
		s.mu.Unlock()
		if ctx.Err() == nil {
			s.d.Log.DebugContext(ctx, "update check failed", slog.String("error", err.Error()))
		}
		return nil
	}
	newer := updates.CompareVersions(rel.Version, s.d.Current) > 0
	s.mu.Lock()
	s.release, s.checkedAt, s.lastFailed = &rel, now, false
	notify := newer && s.notified != rel.Version
	if notify {
		s.notified = rel.Version
	}
	s.mu.Unlock()
	if newer {
		s.d.Log.InfoContext(ctx, "a new Arenile version is available", slog.String("current", s.d.Current), slog.String("latest", rel.Version))
	}
	if notify && s.d.OnNewer != nil {
		s.d.OnNewer(rel.Version)
	}
	return nil
}

// Status returns the update notice. Owners and managers may read it (a
// grant with establishment.configure); API keys and other roles get
// auth.ErrForbidden.
func (s *Service) Status(_ context.Context, p *auth.Principal) (Status, error) {
	if err := authorize(p); err != nil {
		return Status{}, err
	}
	st := Status{Current: s.d.Current, State: StateOK, HowTo: s.d.HowTo, CheckEnabled: s.d.Enabled()}
	if !st.CheckEnabled || !updates.IsRelease(s.d.Current) {
		st.State = StateDisabled
		return st, nil
	}
	s.mu.Lock()
	rel, checkedAt, failed := s.release, s.checkedAt, s.lastFailed
	s.mu.Unlock()
	if failed {
		st.State = StateUnavailable
	}
	if rel == nil || s.d.Clock.Now().Sub(checkedAt) > MaxAge {
		return st, nil
	}
	st.Latest, st.PublishedAt, st.ReleaseNotesURL, st.CheckedAt = rel.Version, rel.PublishedAt, rel.ReleaseNotes, checkedAt
	st.UpdateAvailable = updates.CompareVersions(rel.Version, s.d.Current) > 0
	switch s.d.HowTo {
	case HowDesktop, HowBinary:
		edition := "desktop"
		if s.d.HowTo == HowBinary {
			edition = "server"
		}
		st.DownloadURL = updates.DownloadPage
		if f, ok := updates.MatchFile(rel.Files, edition, s.d.GOOS, s.d.GOARCH); ok {
			st.DownloadURL = f.URL
		}
	case HowDocker:
	}
	return st, nil
}

func authorize(p *auth.Principal) error {
	if p == nil {
		return auth.ErrUnauthenticated
	}
	if p.Kind != auth.PrincipalUser || p.TOTPEnrolmentRequired {
		return auth.ErrForbidden
	}
	for _, g := range p.Grants {
		if auth.Can(g.Role, auth.PermEstablishmentConfigure) {
			return nil
		}
	}
	return auth.ErrForbidden
}
