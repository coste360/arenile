#!/usr/bin/env bash
# Builds the desktop launcher (cmd/arenile-desktop) with the SPAs embedded.
#
#   scripts/desktop/build.sh [GOOS [GOARCH]]     default: the host platform
#
# Output: $DIST_DIR/bin/<goos>_<goarch>/arenile-desktop[.exe]. CGO is enabled
# only when DESKTOP_TAGS is set (tray build), so a tagged build must run on
# the native runner of the target OS. See scripts/desktop/common.sh.
# shellcheck source=scripts/desktop/common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

goos="${1:-$(go env GOOS)}"
goarch="${2:-$(go env GOARCH)}"
ext=""
[ "$goos" = windows ] && ext=".exe"

ensure_web
out="$DIST_DIR/bin/${goos}_${goarch}/arenile-desktop$ext"
mkdir -p "$(dirname "$out")"
go_build_launcher "$out" "$goos" "$goarch"
printf '%s\n' "$out"
