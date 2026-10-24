package updates

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sampleLatest = `{
  "version": "1.2.0",
  "tag": "v1.2.0",
  "published_at": "2026-10-20T08:00:00Z",
  "release_notes": "https://github.com/coste360/arenile/releases/tag/v1.2.0",
  "checksums": "https://arenile.app/download/1.2.0/SHA256SUMS",
  "files": [
    {"name": "Arenile-1.2.0-universal.dmg", "edition": "desktop", "os": "macos", "arch": "universal", "size": 10, "sha256": "aa", "url": "https://arenile.app/download/1.2.0/Arenile-1.2.0-universal.dmg"},
    {"name": "Arenile-1.2.0-arm64_setup.exe", "edition": "desktop", "os": "windows", "arch": "arm64", "size": 10, "sha256": "bb", "url": "https://arenile.app/download/1.2.0/Arenile-1.2.0-arm64_setup.exe"},
    {"name": "Arenile-1.2.0_setup.exe", "edition": "desktop", "os": "windows", "arch": "x64", "size": 10, "sha256": "cc", "url": "https://arenile.app/download/1.2.0/Arenile-1.2.0_setup.exe"},
    {"name": "Arenile-1.2.0.AppImage", "edition": "desktop", "os": "linux", "arch": "x86_64", "size": 10, "sha256": "dd", "url": "https://arenile.app/download/1.2.0/Arenile-1.2.0.AppImage"},
    {"name": "arenile_1.2.0_linux_arm64.tar.gz", "edition": "server", "os": "linux", "arch": "arm64", "size": 10, "sha256": "ee", "url": "https://arenile.app/download/1.2.0/arenile_1.2.0_linux_arm64.tar.gz"},
    {"name": "evil", "edition": "desktop", "os": "linux", "arch": "arm64", "size": 1, "sha256": "ff", "url": "javascript:alert(1)"}
  ]
}`

func TestCompareVersions_Scenarios(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.3.9", 1},
		{"v1.10.0", "1.9.0", 1},
		{"1.4", "1.4.0", 0},
		{"1.4.0-rc.1", "1.4.0", -1},
		{"v1.4.0", "1.4.0-rc.1", 1},
		{"1.4.0-rc.1", "1.4.0-rc.2", 0},
		{"0.6.0-dev.187+835d645", "v0.6.0", -1},
		{"0.6.0-dev.187+835d645.dirty", "0.5.9", 1},
		{"0.6.0+build.7", "0.6.0", 0},
		{"1.3.0", "v1.4.0", -1},
		{"1.0.0", "dev", 1},
		{"dev", "dev", 0},
		{"x.y", "1.0", -1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsRelease_Scenarios(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"v1.4.0", true},
		{"1.4.0", true},
		{"1.4.0-rc.1", true},
		{"dev", false},
		{"", false},
		{"1.1.0-dev.12+abc", false},
		{"1.1.0-dev.12+abc.dirty", false},
	}
	for _, tt := range tests {
		if got := IsRelease(tt.v); got != tt.want {
			t.Errorf("IsRelease(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

func TestUserAgent_Scenarios(t *testing.T) {
	tests := []struct {
		version, edition, goos, goarch, want string
	}{
		{"v1.0.1", "desktop", "windows", "arm64", "arenile/1.0.1 (desktop; windows/arm64)"},
		{"1.0.1", "server", "linux", "amd64", "arenile/1.0.1 (server; linux/amd64)"},
		{"", "", "", "", "arenile/dev (server; unknown/unknown)"},
		{"1.0\r\nX-Evil: 1", "desktop", "linux", "amd64", "arenile/1.0X-Evil1 (desktop; linux/amd64)"},
	}
	for _, tt := range tests {
		if got := UserAgent(tt.version, tt.edition, tt.goos, tt.goarch); got != tt.want {
			t.Errorf("UserAgent(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestParse_LatestJSON(t *testing.T) {
	rel, err := Parse(strings.NewReader(sampleLatest))
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "1.2.0" || rel.Tag != "v1.2.0" || !rel.PublishedAt.Equal(time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("release %+v", rel)
	}
	if rel.ReleaseNotes != "https://github.com/coste360/arenile/releases/tag/v1.2.0" || len(rel.Files) != 5 {
		t.Fatalf("notes %q, %d files (the javascript: URL must be dropped)", rel.ReleaseNotes, len(rel.Files))
	}
}

func TestParse_Invalid(t *testing.T) {
	for _, body := range []string{`{}`, `{"version":"dev"}`, `not json`, `{"version":"1.0","release_notes":"javascript:x"}`} {
		rel, err := Parse(strings.NewReader(body))
		if body == `{"version":"1.0","release_notes":"javascript:x"}` {
			if err != nil || rel.ReleaseNotes != "" {
				t.Errorf("%s: %+v %v", body, rel, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidRelease) {
			t.Errorf("%s: err = %v", body, err)
		}
	}
}

// Old desktop builds read only {"version"}: a minimal file stays valid.
func TestParse_VersionOnly(t *testing.T) {
	rel, err := Parse(strings.NewReader(`{"version":"1.5.0"}`))
	if err != nil || rel.Version != "1.5.0" || rel.Files != nil {
		t.Fatalf("%+v %v", rel, err)
	}
}

func TestMatchFile_Scenarios(t *testing.T) {
	rel, err := Parse(strings.NewReader(sampleLatest))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		edition, goos, goarch, want string
	}{
		{"desktop", "darwin", "arm64", "Arenile-1.2.0-universal.dmg"},
		{"desktop", "darwin", "amd64", "Arenile-1.2.0-universal.dmg"},
		{"desktop", "windows", "arm64", "Arenile-1.2.0-arm64_setup.exe"},
		{"desktop", "windows", "amd64", "Arenile-1.2.0_setup.exe"},
		{"desktop", "linux", "amd64", "Arenile-1.2.0.AppImage"},
		{"desktop", "linux", "arm64", ""}, // only the javascript: entry, dropped
		{"server", "linux", "arm64", "arenile_1.2.0_linux_arm64.tar.gz"},
		{"server", "linux", "amd64", ""},
	}
	for _, tt := range tests {
		f, ok := MatchFile(rel.Files, tt.edition, tt.goos, tt.goarch)
		if f.Name != tt.want || ok != (tt.want != "") {
			t.Errorf("MatchFile(%s %s/%s) = %q %v, want %q", tt.edition, tt.goos, tt.goarch, f.Name, ok, tt.want)
		}
	}
}

func TestChecker_RequestCarriesOnlyTheUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Cookie") != "" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request %s %s %v", r.Method, r.URL, r.Header)
		}
		if got := r.UserAgent(); got != "arenile/1.0.1 (desktop; windows/arm64)" {
			t.Errorf("User-Agent = %q", got)
		}
		fmt.Fprint(w, sampleLatest)
	}))
	defer srv.Close()
	c := Checker{URL: srv.URL, Current: "v1.0.1", Edition: "desktop", GOOS: "windows", GOARCH: "arm64", Client: srv.Client()}
	latest, newer, err := c.Check(context.Background())
	if err != nil || latest != "1.2.0" || !newer {
		t.Fatalf("Check = %q %v %v", latest, newer, err)
	}
}

func TestChecker_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := (Checker{URL: srv.URL, Client: srv.Client()}).Fetch(context.Background()); err == nil {
		t.Fatal("want error on 404")
	}
}
