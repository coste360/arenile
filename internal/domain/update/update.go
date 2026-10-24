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
//   - a manual check ("Verifica aggiornamenti", CheckNow and Check) runs
//     at once, but at most once every ManualMinInterval (any earlier
//     attempt counts) and never two at a time: a caller that finds a check
//     running waits for its result;
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
	// ManualMinInterval is the shortest time between two network checks
	// started by hand: repeated clicks never hammer the update server.
	ManualMinInterval = time.Minute
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
	// Enabled reports whether the check is on. It is read only when a
	// check is due and by Status (on desktop it reads the settings file).
	Enabled func(ctx context.Context) bool
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
	// Pending: the check is on but its first attempt has not run yet.
	Pending bool
	// LastAttemptAt is the last check attempt, successful or not.
	LastAttemptAt time.Time
	// GOOS and GOARCH of the installation (label of the download).
	GOOS, GOARCH string
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
	// inflight is closed when the running network check ends (nil when
	// none runs): a manual check waits for it instead of starting another.
	inflight chan struct{}
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
		d.Enabled = func(context.Context) bool { return false }
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
	if s.d.Fetcher == nil || !updates.IsRelease(s.d.Current) {
		return nil
	}
	now := s.d.Clock.Now()
	s.mu.Lock()
	due := s.dueLocked(now)
	s.mu.Unlock()
	// The switch is read only when a check is due: no settings I/O every tick.
	if !due || !s.d.Enabled(ctx) {
		return nil
	}
	s.mu.Lock()
	if due = s.inflight == nil && s.dueLocked(now); due {
		s.startLocked(now)
	}
	s.mu.Unlock()
	if due {
		s.fetch(ctx, now)
	}
	return nil
}

// CheckNow runs the check at once ("Verifica aggiornamenti" of the panel)
// and returns the fresh status. Same permissions as Status. With the check
// off or a development version it makes no network call (StateDisabled).
// Within ManualMinInterval of the last attempt it answers from memory; a
// check already running is waited for. A failed check is not an error
// (StateUnavailable).
func (s *Service) CheckNow(ctx context.Context, p *auth.Principal) (Status, error) {
	if err := authorize(p); err != nil {
		return Status{}, err
	}
	return s.Check(ctx), nil
}

// Check is CheckNow for the desktop launcher (menu "Verifica
// aggiornamenti", in-process, no principal: the operator of the machine).
func (s *Service) Check(ctx context.Context) Status {
	if s.d.Fetcher != nil && updates.IsRelease(s.d.Current) && s.d.Enabled(ctx) {
		now := s.d.Clock.Now()
		s.mu.Lock()
		wait := s.inflight
		start := wait == nil && (s.lastAttempt.IsZero() || now.Sub(s.lastAttempt) >= ManualMinInterval)
		if start {
			s.startLocked(now)
		}
		s.mu.Unlock()
		switch {
		case start:
			s.fetch(ctx, now)
		case wait != nil:
			select {
			case <-wait:
			case <-ctx.Done():
			}
		}
	}
	return s.status(ctx)
}

// startLocked records the start of a network check (s.mu held).
func (s *Service) startLocked(now time.Time) {
	s.lastAttempt = now
	s.inflight = make(chan struct{})
}

// fetch runs the network check started by startLocked and records its
// outcome. Network errors are logged at debug level only.
func (s *Service) fetch(ctx context.Context, now time.Time) {
	rel, err := s.d.Fetcher.Fetch(ctx)
	s.mu.Lock()
	done := s.inflight
	s.inflight = nil
	if err != nil {
		s.lastFailed = true
		s.mu.Unlock()
		close(done)
		if ctx.Err() == nil {
			s.d.Log.DebugContext(ctx, "update check failed", slog.String("error", err.Error()))
		}
		return
	}
	newer := updates.CompareVersions(rel.Version, s.d.Current) > 0
	s.release, s.checkedAt, s.lastFailed = &rel, now, false
	notify := newer && s.notified != rel.Version
	if notify {
		s.notified = rel.Version
	}
	s.mu.Unlock()
	close(done)
	if newer {
		s.d.Log.InfoContext(ctx, "a new Arenile version is available", slog.String("current", s.d.Current), slog.String("latest", rel.Version))
	}
	if notify && s.d.OnNewer != nil {
		s.d.OnNewer(rel.Version)
	}
}

func (s *Service) dueLocked(now time.Time) bool {
	switch {
	case s.lastAttempt.IsZero():
		return now.Sub(s.started) >= StartDelay
	case s.lastFailed:
		return now.Sub(s.lastAttempt) >= RetryAfter
	default:
		return now.Sub(s.lastAttempt) >= Interval
	}
}

// Status returns the update notice. Owners and managers may read it (a
// grant with establishment.configure); API keys and other roles get
// auth.ErrForbidden.
func (s *Service) Status(ctx context.Context, p *auth.Principal) (Status, error) {
	if err := authorize(p); err != nil {
		return Status{}, err
	}
	return s.status(ctx), nil
}

func (s *Service) status(ctx context.Context) Status {
	st := Status{
		Current: s.d.Current, State: StateOK, HowTo: s.d.HowTo, CheckEnabled: s.d.Enabled(ctx),
		GOOS: s.d.GOOS, GOARCH: s.d.GOARCH,
	}
	if !st.CheckEnabled || !updates.IsRelease(s.d.Current) {
		st.State = StateDisabled
		return st
	}
	s.mu.Lock()
	rel, checkedAt, failed, attempt := s.release, s.checkedAt, s.lastFailed, s.lastAttempt
	s.mu.Unlock()
	st.LastAttemptAt = attempt
	// Before the first check ends (not run yet, or running). No Fetcher:
	// nothing will ever be checked, so nothing is pending.
	st.Pending = s.d.Fetcher != nil && rel == nil && !failed
	if failed {
		st.State = StateUnavailable
	}
	if rel == nil || s.d.Clock.Now().Sub(checkedAt) > MaxAge {
		return st
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
	return st
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
