#!/usr/bin/env bash
# Shared helpers of the desktop packaging scripts (sourced, not executed).
#
# Inputs (environment):
#   VERSION        release tag (e.g. v1.4.0); default: scripts/version.sh (the tag,
#                  or a dev build such as 0.6.0-dev.187+835d645, else "dev").
#                  The desktop packages use the same tag as the server (spec 29.1).
#   DESKTOP_TAGS   Go build tags of the launcher: "" = headless, CGO off;
#                  "desktop" = tray build, CGO on (native runner of each OS).
#   DIST_DIR       output directory; default: dist/desktop (git-ignored).
#   SKIP_WEB_BUILD 1 = the SPAs are already in internal/http/static (CI copies
#                  the web artifact there); otherwise `make web-build` runs first.
#
# Outputs follow the pattern Arenile_<version>_<os>_<arch>[...][-unsigned].<ext>:
# a package without a valid publisher signature always carries "-unsigned".

# shellcheck disable=SC2034 # the variables below are used by the sourcing scripts
set -euo pipefail

DESKTOP_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PKG_DIR="$DESKTOP_ROOT/packaging/desktop"
DIST_DIR="${DIST_DIR:-$DESKTOP_ROOT/dist/desktop}"
DESKTOP_TAGS="${DESKTOP_TAGS:-}"
APP_NAME="Arenile"
BUNDLE_ID="it.coste360.arenile"

if [ -z "${VERSION:-}" ]; then
	VERSION="$("$DESKTOP_ROOT/scripts/version.sh")"
fi
# File-name safe form of the version (PR refs such as "12/merge").
VERSION_SAFE="$(printf '%s' "$VERSION" | tr '/ ' '--')"

log() { printf 'desktop: %s\n' "$*" >&2; }
die() {
	printf 'desktop: error: %s\n' "$*" >&2
	exit 1
}

# numeric_version prints MAJOR.MINOR.PATCH from a version such as v1.2.3-rc.1
# or 0.6.0-dev.187+835d645 (0.0.0 only for a bare "dev" without git metadata).
# Windows file versions and macOS CFBundleShortVersionString accept digits only.
numeric_version() {
	local v="${VERSION#v}"
	if [[ "$v" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
		printf '%s.%s.%s' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
	else
		printf '0.0.0'
	fi
}

# build_number prints a monotonically increasing integer for macOS
# CFBundleVersion: the number of commits of HEAD (1 without git metadata).
build_number() {
	git -C "$DESKTOP_ROOT" rev-list --count HEAD 2>/dev/null || printf 1
}

# go_ldflags: -s -w strip, empty build id (reproducible-ish), version injection.
go_ldflags() {
	printf '%s' "-s -w -buildid= -X main.version=$VERSION${1:+ $1}"
}

# cgo_flag GOOS prints 1 when the launcher is built with tags on macOS or
# Windows (tray and webview need CGO there). On Linux the tray build is
# CGO-free (fyne.io/systray over D-Bus; no webview, see
# internal/desktop/remote/window_webview.go), so the AppImage is a static
# binary that runs on any glibc or musl distribution.
cgo_flag() {
	if [ -n "$DESKTOP_TAGS" ] && [ "${1:-}" != linux ]; then printf 1; else printf 0; fi
}

# ensure_web embeds the SPAs (the bundle must include the admin panel).
ensure_web() {
	if [ "${SKIP_WEB_BUILD:-0}" = 1 ]; then
		[ -f "$DESKTOP_ROOT/internal/http/static/admin/index.html" ] ||
			die "SKIP_WEB_BUILD=1 but internal/http/static/admin/index.html is missing"
		return
	fi
	log "building the SPAs (make web-build)"
	make -C "$DESKTOP_ROOT" web-build >&2
}

# go_build_launcher OUT GOOS GOARCH [extra ldflags]: builds cmd/arenile-desktop.
go_build_launcher() {
	local out="$1" goos="$2" goarch="$3" extra="${4:-}"
	local tags=()
	[ -n "$DESKTOP_TAGS" ] && tags=(-tags "$DESKTOP_TAGS")
	log "go build $goos/$goarch tags=[${DESKTOP_TAGS}] cgo=$(cgo_flag "$goos") -> ${out#"$DESKTOP_ROOT"/}"
	(
		cd "$DESKTOP_ROOT"
		GOOS="$goos" GOARCH="$goarch" CGO_ENABLED="$(cgo_flag "$goos")" GOFLAGS="${GOFLAGS:-} -buildvcs=false" \
			go build -trimpath ${tags[@]+"${tags[@]}"} -ldflags "$(go_ldflags "$extra")" -o "$out" ./cmd/arenile-desktop
	)
}

# sha256_sidecar FILE: writes FILE.sha256 in `sha256sum` format.
sha256_sidecar() {
	local f="$1" dir base sum
	dir="$(dirname "$f")"
	base="$(basename "$f")"
	if command -v sha256sum >/dev/null; then
		sum="$(cd "$dir" && sha256sum "$base")"
	else
		sum="$(cd "$dir" && shasum -a 256 "$base")"
	fi
	printf '%s\n' "$sum" >"$f.sha256"
	log "sha256 $sum"
}

mkdir -p "$DIST_DIR"
