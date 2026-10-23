#!/usr/bin/env bash
# Packages the desktop edition for Linux as an AppImage (appimagetool).
# Runs on Linux (ubuntu runner or a local machine / container).
#
#   scripts/desktop/package-linux.sh
#
# Environment (besides common.sh):
#   LINUX_GOARCH       amd64 (default on x86_64 hosts) or arm64; the Linux
#                      launcher is CGO-free (tray over D-Bus, no webview) and
#                      cross-compiles, but appimagetool needs a host of the
#                      target architecture
#   APPIMAGETOOL       path to an appimagetool binary; otherwise the pinned release
#                      (APPIMAGETOOL_VERSION) is downloaded into $DIST_DIR/.tools and
#                      verified against the sha256 below; the AppImage runtime is
#                      pinned the same way (APPIMAGE_RUNTIME_VERSION), so the build
#                      never depends on a moving "continuous" download.
#
# Output in $DIST_DIR: Arenile_<version>_linux_<x86_64|aarch64>.AppImage (+ .sha256).
# AppImages carry no publisher signature on Linux; integrity is the checksum
# published by the Coste360 portal.
# shellcheck source=scripts/desktop/common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

[ "$(uname -s)" = Linux ] || die "package-linux.sh runs on Linux only (appimagetool)"
for t in curl file sha256sum; do command -v "$t" >/dev/null || die "$t is required (appimagetool needs file)"; done

host_arch="$(uname -m)"
case "${LINUX_GOARCH:-$(go env GOARCH)}" in
amd64) goarch=amd64 appimage_arch=x86_64 ;;
arm64) goarch=arm64 appimage_arch=aarch64 ;;
*) die "unsupported LINUX_GOARCH ${LINUX_GOARCH:-}" ;;
esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# --- 1. binary --------------------------------------------------------------
ensure_web
appdir="$work/Arenile.AppDir"
mkdir -p "$appdir/usr/bin" "$appdir/usr/share/applications" "$appdir/usr/share/metainfo" \
	"$appdir/usr/share/icons/hicolor/512x512/apps" "$appdir/usr/share/icons/hicolor/scalable/apps" \
	"$appdir/usr/share/doc/arenile"
go_build_launcher "$appdir/usr/bin/arenile-desktop" linux "$goarch"

# --- 2. AppDir ----------------------------------------------------------------
install -m 0755 "$PKG_DIR/linux/AppRun" "$appdir/AppRun"
install -m 0644 "$PKG_DIR/linux/arenile.desktop" "$appdir/arenile.desktop"
install -m 0644 "$PKG_DIR/linux/arenile.desktop" "$appdir/usr/share/applications/arenile.desktop"
install -m 0644 "$PKG_DIR/linux/it.coste360.arenile.metainfo.xml" "$appdir/usr/share/metainfo/it.coste360.arenile.metainfo.xml"
install -m 0644 "$PKG_DIR/icons/arenile.png" "$appdir/arenile.png"
install -m 0644 "$PKG_DIR/icons/arenile.png" "$appdir/usr/share/icons/hicolor/512x512/apps/arenile.png"
install -m 0644 "$PKG_DIR/icons/arenile.svg" "$appdir/usr/share/icons/hicolor/scalable/apps/arenile.svg"
install -m 0644 "$DESKTOP_ROOT/LICENSE" "$appdir/usr/share/doc/arenile/LICENSE"
ln -s arenile.png "$appdir/.DirIcon"

# --- 3. appimagetool + runtime (pinned, verified) -------------------------------
# Bump together: tag and sha256 of each asset (GitHub shows the digests).
APPIMAGETOOL_VERSION="${APPIMAGETOOL_VERSION:-1.9.1}"
APPIMAGE_RUNTIME_VERSION="${APPIMAGE_RUNTIME_VERSION:-20251108}"
pinned_sha() { # name
	case "$1" in
	appimagetool-1.9.1-x86_64) echo ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0 ;;
	appimagetool-1.9.1-aarch64) echo f0837e7448a0c1e4e650a93bb3e85802546e60654ef287576f46c71c126a9158 ;;
	runtime-20251108-x86_64) echo 2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d ;;
	runtime-20251108-aarch64) echo 00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444 ;;
	*) echo "" ;;
	esac
}
fetch() { # url dest sha256
	[ -f "$2" ] && return
	log "downloading $1"
	mkdir -p "$(dirname "$2")"
	curl -fsSL --retry 3 -o "$2.tmp" "$1"
	[ -n "$3" ] || die "no pinned sha256 for $(basename "$2"): set it in pinned_sha"
	printf '%s  %s\n' "$3" "$2.tmp" | sha256sum -c - >&2 || die "checksum mismatch for $1"
	chmod +x "$2.tmp"
	mv "$2.tmp" "$2"
}
tools="$DIST_DIR/.tools"
tool="${APPIMAGETOOL:-}"
if [ -z "$tool" ]; then
	tool="$tools/appimagetool-$APPIMAGETOOL_VERSION-$host_arch.AppImage"
	fetch "https://github.com/AppImage/appimagetool/releases/download/$APPIMAGETOOL_VERSION/appimagetool-$host_arch.AppImage" \
		"$tool" "$(pinned_sha "appimagetool-$APPIMAGETOOL_VERSION-$host_arch")"
fi
runtime="$tools/runtime-$APPIMAGE_RUNTIME_VERSION-$appimage_arch"
fetch "https://github.com/AppImage/type2-runtime/releases/download/$APPIMAGE_RUNTIME_VERSION/runtime-$appimage_arch" \
	"$runtime" "$(pinned_sha "runtime-$APPIMAGE_RUNTIME_VERSION-$appimage_arch")"

# Reproducible-ish squashfs timestamps: the commit time.
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$DESKTOP_ROOT" log -1 --format=%ct 2>/dev/null || date +%s)}"
export SOURCE_DATE_EPOCH

out="$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_linux_${appimage_arch}.AppImage"
rm -f "$out" "$out.sha256"
# No FUSE on CI runners and containers: let the tool extract itself.
APPIMAGE_EXTRACT_AND_RUN=1 ARCH="$appimage_arch" VERSION="$VERSION" \
	"$tool" --no-appstream --runtime-file "$runtime" "$appdir" "$out" >&2
chmod 0755 "$out"
sha256_sidecar "$out"
printf '%s\n' "$out"
