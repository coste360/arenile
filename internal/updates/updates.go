// Package updates reads the public description of the latest Arenile
// release (ADR 0025) and compares versions. It is shared by the server (the
// update notice of the panel, internal/domain/update) and the desktop
// launcher, and depends only on the standard library.
//
// The request is a plain GET of a public JSON file: no cookie, no install
// identifier, no data of the lido. The User-Agent carries only the running
// version, the edition and the OS/architecture, e.g.
// "arenile/1.0.1 (desktop; windows/arm64)", which Coste360 uses for
// aggregate counts. It is not telemetry (ARENILE_TELEMETRY stays false).
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultURL is the public file describing the latest release, written by
// the build pipeline (arenile-build scripts/bundle.sh). Override it with
// ARENILE_UPDATE_URL (server and desktop).
const DefaultURL = "https://updates.coste360.it/arenile/latest.json"

// DownloadPage is the public download page, the fallback download link.
const DownloadPage = "https://arenile.app/download/"

// maxBody bounds the response read from the update URL.
const maxBody = 256 << 10

// ErrInvalidRelease is returned when the file cannot be used.
var ErrInvalidRelease = errors.New("updates: invalid release description")

// File is one published package (files[] of latest.json).
type File struct {
	Name    string `json:"name"`
	Edition string `json:"edition"` // "desktop", "server" or "other"
	OS      string `json:"os"`      // "macos", "windows", "linux"
	Arch    string `json:"arch"`    // desktop: "arm64", "x86_64", "x64", "universal"; server: "amd64", "arm64"
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
}

// Release is the latest published release. Old desktop builds read only
// {"version"}: that field keeps its meaning.
type Release struct {
	Version     string
	Tag         string
	PublishedAt time.Time // zero when missing or malformed
	// ReleaseNotes is the URL of the release notes ("" when not http(s)).
	ReleaseNotes string
	// Files holds only the entries with an http(s) URL.
	Files []File
}

// Checker fetches the latest release.
type Checker struct {
	URL     string
	Current string
	// Edition, GOOS and GOARCH only go into the User-Agent.
	Edition string
	GOOS    string
	GOARCH  string
	Client  *http.Client
}

// UserAgent is the only information the request carries:
// "arenile/<version> (<edition>; <goos>/<goarch>)".
func UserAgent(version, edition, goos, goarch string) string {
	clean := func(s, def string) string {
		s = strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '+', r == '_':
				return r
			}
			return -1
		}, s)
		if s == "" {
			return def
		}
		return s
	}
	return fmt.Sprintf("arenile/%s (%s; %s/%s)",
		clean(strings.TrimPrefix(strings.TrimSpace(version), "v"), "dev"),
		clean(edition, "server"), clean(goos, "unknown"), clean(goarch, "unknown"))
}

// Fetch reads and validates the release description.
func (c Checker) Fetch(ctx context.Context) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("updates: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent(c.Current, c.Edition, c.GOOS, c.GOARCH))
	req.Header.Set("Accept", "application/json")
	hc := c.Client
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updates: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("updates: HTTP %d", res.StatusCode)
	}
	return Parse(io.LimitReader(res.Body, maxBody))
}

// Parse decodes a latest.json document (pure apart from reading r).
func Parse(r io.Reader) (Release, error) {
	var body struct {
		Version      string `json:"version"`
		Tag          string `json:"tag"`
		PublishedAt  string `json:"published_at"`
		ReleaseNotes string `json:"release_notes"`
		Files        []File `json:"files"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("%w: %w", ErrInvalidRelease, err)
	}
	v := strings.TrimSpace(body.Version)
	if _, ok := versionParts(v); !ok {
		return Release{}, fmt.Errorf("%w: version %q", ErrInvalidRelease, body.Version)
	}
	rel := Release{Version: v, Tag: body.Tag}
	if t, err := time.Parse(time.RFC3339, body.PublishedAt); err == nil {
		rel.PublishedAt = t.UTC()
	}
	if isWebURL(body.ReleaseNotes) {
		rel.ReleaseNotes = body.ReleaseNotes
	}
	for _, f := range body.Files {
		if isWebURL(f.URL) {
			rel.Files = append(rel.Files, f)
		}
	}
	return rel, nil
}

// IsRelease reports whether v is a release build: not "dev" nor a
// development build of scripts/version.sh ("1.1.0-dev.12+abc"). Only
// releases check for updates, so developers are never nagged.
func IsRelease(v string) bool {
	if strings.Contains(v, "-dev.") {
		return false
	}
	_, ok := versionParts(v)
	return ok
}

// MatchFile returns the package of files for edition ("desktop" or
// "server") on goos/goarch (Go names), following the naming of the build
// pipeline: macOS desktop prefers the native build, then "universal".
func MatchFile(files []File, edition, goos, goarch string) (File, bool) {
	osName := map[string]string{"darwin": "macos"}[goos]
	if osName == "" {
		osName = goos
	}
	var arches []string
	switch {
	case edition != "desktop":
		arches = []string{goarch}
	case goarch == "amd64" && goos == "windows":
		arches = []string{"x64", "x86_64", "amd64"}
	case goarch == "amd64":
		arches = []string{"x86_64", "amd64", "x64", "universal"}
	default:
		arches = []string{goarch, "universal"}
	}
	for _, a := range arches {
		for _, f := range files {
			if f.Edition == edition && f.OS == osName && f.Arch == a {
				return f, true
			}
		}
	}
	return File{}, false
}

func isWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
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
