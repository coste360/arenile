#!/usr/bin/env bash
# Prints the build version of Arenile (server, Docker image, desktop packages):
#
#   - HEAD tagged vX.Y.Z and a clean tree  -> the tag (e.g. v1.4.0)
#   - otherwise a semver development build -> X.Y.Z-dev.<commits>+<sha>[.dirty]
#     where X.Y.Z is the release being prepared: the minor after the latest
#     vX.Y.Z tag, or ARENILE_DEV_BASE_VERSION (default below) before the first
#     tag. Pre-release versions sort below the release (internal/desktop
#     CompareVersions), so a dev build is offered the final release.
#   - no git metadata                      -> "dev"
#
# Never 0.0.0: the desktop packages derive their numeric versions from this.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="${ARENILE_DEV_BASE_VERSION:-1.0.0}"
g() { git -C "$root" "$@" 2>/dev/null; }

sha="$(g rev-parse --short HEAD)" || { echo dev; exit 0; }
dirty=""
[ -n "$(g status --porcelain --untracked-files=no)" ] && dirty=".dirty"

if [ -z "$dirty" ] && tag="$(g describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' HEAD)"; then
	echo "$tag"
	exit 0
fi
# Releases only: after v1.4.0-rc.1 the release being prepared is still 1.4.0, not 1.5.0.
if last="$(g describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' HEAD)" &&
	[[ "$last" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
	base="${BASH_REMATCH[1]}.$((BASH_REMATCH[2] + 1)).0"
fi
echo "$base-dev.$(g rev-list --count HEAD)+$sha$dirty"
