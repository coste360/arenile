package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultUpdateURL is where the update check reads the latest desktop
// version (JSON {"version": "1.4.0"}). The request carries no identifier,
// no cookie and no data of the lido: only the User-Agent with the version.
// It is called ONLY when the operator enables update_check (off by
// default, spec 29.1 and ARENILE_TELEMETRY=false).
const DefaultUpdateURL = "https://download.coste360.it/arenile/desktop/latest.json"

// UpdateCheckInterval spaces the checks while the app stays open.
const UpdateCheckInterval = 24 * time.Hour

// UpdateChecker looks for a newer desktop release.
type UpdateChecker struct {
	URL     string
	Current string
	Client  *http.Client
	Log     *slog.Logger
}

// Check returns the latest version and whether it is newer than Current.
func (u UpdateChecker) Check(ctx context.Context) (latest string, newer bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.URL, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "arenile-desktop/"+u.Current)
	req.Header.Set("Accept", "application/json")
	c := u.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("desktop: update check: HTTP %d", res.StatusCode)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&body); err != nil {
		return "", false, fmt.Errorf("desktop: update check: %w", err)
	}
	if body.Version == "" {
		return "", false, errors.New("desktop: update check: empty version")
	}
	return body.Version, CompareVersions(body.Version, u.Current) > 0, nil
}

// CompareVersions compares two dotted versions ("v1.4.0", "1.10") on their
// numeric parts (pure): -1, 0 or 1. With equal numeric parts a pre-release
// ("1.4.0-rc.1", the development builds "0.6.0-dev.187+835d645" of
// scripts/version.sh) is lower than the release, as in semver; two
// pre-releases compare equal and build metadata ("+...") is ignored. A
// non-numeric version (e.g. "dev") is lower than any release.
func CompareVersions(a, b string) int {
	pa, oka := versionParts(a)
	pb, okb := versionParts(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch pra, prb := isPrerelease(a), isPrerelease(b); {
	case pra && !prb:
		return -1
	case !pra && prb:
		return 1
	}
	return 0
}

// isPrerelease reports whether v carries a semver pre-release suffix
// ("-..." before any "+build" metadata).
func isPrerelease(v string) bool {
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	return strings.Contains(v, "-")
}

func versionParts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// RunUpdateChecks checks at start-up and every UpdateCheckInterval while
// enabled() is true; notify is called once per newer version found. It
// makes no network call while enabled() is false.
func RunUpdateChecks(ctx context.Context, u UpdateChecker, enabled func() bool, notify func(latest string), tick <-chan time.Time) {
	if tick == nil {
		t := time.NewTicker(UpdateCheckInterval)
		defer t.Stop()
		tick = t.C
	}
	notified := ""
	check := func() {
		if !enabled() {
			return
		}
		latest, newer, err := u.Check(ctx)
		if err != nil {
			if u.Log != nil {
				u.Log.Info("desktop update check failed", slog.String("error", err.Error()))
			}
			return
		}
		if newer && latest != notified {
			notified = latest
			notify(latest)
		}
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			check()
		}
	}
}
