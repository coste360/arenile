#!/usr/bin/env bash
# Packages the desktop edition for Windows (amd64 or arm64): arenile-desktop.exe
# with icon/version/manifest resources, optional Authenticode signature, Inno
# Setup installer. Runs in Git Bash on windows-latest (x64) or windows-11-arm
# (arm64, Snapdragon PCs), or locally on Windows; the `binary` stage also
# cross-builds from macOS/Linux when no build tags are set.
#
#   scripts/desktop/package-windows.sh [all|binary|sign FILE...|installer|checksums]
#
# The stages exist so CI can interleave the Azure Trusted Signing action
# between them (binary -> sign exe -> installer -> sign installer -> checksums);
# `all` (default) runs them in order with the PFX signer.
#
# Environment (besides common.sh):
#   WINDOWS_GOARCH                 amd64 (default) or arm64. The arm64 window build needs
#                                  CGO with an aarch64 Windows C/C++ toolchain: llvm-mingw
#                                  (CC=aarch64-w64-mingw32-clang CXX=...-clang++, see
#                                  desktop.yml); the x64 one uses the runner's MinGW gcc.
#   ISCC, SIGNTOOL, WINDRES        tool paths (auto-detected)
#   Signing (optional; without it the installer is named -unsigned):
#   WINDOWS_CERTIFICATE_PFX_BASE64 Authenticode certificate + key (.pfx, base64)
#   WINDOWS_CERTIFICATE_PASSWORD   its password
#   WINDOWS_TIMESTAMP_URL          RFC 3161 server (default http://timestamp.digicert.com)
#   WINDOWS_SIGNING=external       files are signed outside this script (Azure Trusted
#                                  Signing action in CI): drop the -unsigned label
# shellcheck source=scripts/desktop/common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# Git Bash rewrites arguments that start with "/" into paths: ISCC and
# signtool use /switches, so disable the conversion for native tools.
export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'

GOARCH_WIN="${WINDOWS_GOARCH:-amd64}"
case "$GOARCH_WIN" in
amd64) WINDRES_CANDIDATES="windres x86_64-w64-mingw32-windres" WINDRES_TARGET="--target=pe-x86-64" ;;
# llvm-mingw's triple-prefixed llvm-windres defaults to the arm64 COFF machine.
arm64) WINDRES_CANDIDATES="aarch64-w64-mingw32-windres" WINDRES_TARGET="" ;;
*) die "WINDOWS_GOARCH must be amd64 or arm64 (got $GOARCH_WIN)" ;;
esac
EXE="$DIST_DIR/bin/windows_${GOARCH_WIN}/arenile-desktop.exe"
SYSO="$DESKTOP_ROOT/cmd/arenile-desktop/zz_arenile_windows_${GOARCH_WIN}.syso"
TIMESTAMP_URL="${WINDOWS_TIMESTAMP_URL:-http://timestamp.digicert.com}"
work="$(mktemp -d)"
trap 'rm -rf "$work" "$SYSO"' EXIT

winpath() { # native Windows path when running under Git Bash / MSYS
	if command -v cygpath >/dev/null; then cygpath -w "$1"; else printf '%s' "$1"; fi
}
mixpath() { # C:/forward/slashes for windres
	if command -v cygpath >/dev/null; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

signing_mode() {
	if [ -n "${WINDOWS_CERTIFICATE_PFX_BASE64:-}" ]; then
		printf pfx
	elif [ "${WINDOWS_SIGNING:-}" = external ]; then
		printf external
	else
		printf none
	fi
}

find_signtool() {
	if [ -n "${SIGNTOOL:-}" ]; then printf '%s' "$SIGNTOOL"; return; fi
	if command -v signtool.exe >/dev/null; then command -v signtool.exe; return; fi
	local f
	# shellcheck disable=SC2012 # fixed SDK layout, version-sorted
	f="$(ls -1d "/c/Program Files (x86)/Windows Kits/10/bin/"*/x64/signtool.exe 2>/dev/null | sort -V | tail -n1 || true)"
	[ -n "$f" ] || die "signtool.exe not found (Windows SDK); set SIGNTOOL"
	printf '%s' "$f"
}

find_iscc() {
	if [ -n "${ISCC:-}" ]; then printf '%s' "$ISCC"; return; fi
	local c
	for c in "/c/Program Files (x86)/Inno Setup 6/ISCC.exe" "/c/Program Files/Inno Setup 6/ISCC.exe"; do
		[ -x "$c" ] && { printf '%s' "$c"; return; }
	done
	command -v iscc >/dev/null && { command -v iscc; return; }
	command -v ISCC.exe >/dev/null && { command -v ISCC.exe; return; }
	die "Inno Setup 6 (ISCC.exe) not found: install it (choco install innosetup) or set ISCC"
}

pfx_file() {
	local f="$work/codesign.pfx"
	[ -f "$f" ] || printf '%s' "$WINDOWS_CERTIFICATE_PFX_BASE64" | base64 --decode >"$f"
	printf '%s' "$f"
}

stage_binary() {
	ensure_web
	local v windres rc
	v="$(numeric_version)"
	windres="${WINDRES:-}"
	if [ -z "$windres" ]; then
		for c in $WINDRES_CANDIDATES; do
			command -v "$c" >/dev/null && { windres="$c"; break; }
		done
	fi
	if [ -n "$windres" ]; then
		rc="$work/arenile.rc"
		sed -e "s|@ICON@|$(mixpath "$PKG_DIR/icons/arenile.ico")|" \
			-e "s|@MANIFEST@|$(mixpath "$PKG_DIR/windows/arenile.exe.manifest")|" \
			-e "s|@V_COMMA@|${v//./,}|g" -e "s|@V_DOT@|$v|g" -e "s|@VERSION@|$VERSION|g" \
			"$PKG_DIR/windows/arenile.rc.in" >"$rc"
		# shellcheck disable=SC2086 # empty target on arm64
		"$windres" $WINDRES_TARGET -O coff -i "$rc" -o "$SYSO"
		log "resources compiled with $windres"
	else
		log "WARNING: windres (MinGW) not found: the .exe has no icon nor version info"
	fi
	local extra=""
	# The window/tray build is a GUI app: no console window behind it.
	[ -n "$DESKTOP_TAGS" ] && extra="-H windowsgui"
	mkdir -p "$(dirname "$EXE")"
	go_build_launcher "$EXE" windows "$GOARCH_WIN" "$extra"
	rm -f "$SYSO"
}

stage_sign() { # files...
	[ "$(signing_mode)" = pfx ] || { log "sign: no PFX configured, skipped"; return; }
	local st f
	st="$(find_signtool)"
	for f in "$@"; do
		"$st" sign /f "$(winpath "$(pfx_file)")" /p "${WINDOWS_CERTIFICATE_PASSWORD:-}" \
			/fd SHA256 /tr "$TIMESTAMP_URL" /td SHA256 /d "Arenile" /du "https://coste360.it" "$(winpath "$f")"
		"$st" verify /pa /v "$(winpath "$f")" >/dev/null
		log "signed $(basename "$f")"
	done
}

installer_path() {
	local label="-unsigned"
	[ "$(signing_mode)" = none ] || label=""
	printf '%s' "$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_windows_${GOARCH_WIN}_setup${label}.exe"
}

stage_installer() {
	[ -f "$EXE" ] || die "missing $EXE: run the binary stage first"
	local iscc out name args=()
	iscc="$(find_iscc)"
	out="$(installer_path)"
	name="$(basename "$out" .exe)"
	rm -f "$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_windows_${GOARCH_WIN}_setup"*.exe*
	args=(
		"/DAppVersion=$(numeric_version)" "/DAppVersionTag=$VERSION"
		"/DRepoRoot=$(winpath "$DESKTOP_ROOT")" "/DSourceExe=$(winpath "$EXE")"
		"/DOutputDir=$(winpath "$DIST_DIR")" "/DOutputName=$name" "/DArch=$GOARCH_WIN"
	)
	if [ "$(signing_mode)" = pfx ]; then
		# Inno signs the embedded uninstaller and the setup with this tool.
		local st
		st="$(winpath "$(find_signtool)")"
		args+=("/DSign=arenile"
			"/Sarenile=\$q$st\$q sign /f \$q$(winpath "$(pfx_file)")\$q /p ${WINDOWS_CERTIFICATE_PASSWORD:-} /fd SHA256 /tr $TIMESTAMP_URL /td SHA256 /d \$qArenile\$q \$f")
	fi
	"$iscc" /Q "${args[@]}" "$(winpath "$PKG_DIR/windows/arenile.iss")"
	[ -f "$out" ] || die "ISCC did not produce $out"
	log "installer: $out"
}

stage_checksums() {
	local out
	out="$(installer_path)"
	[ -f "$out" ] || die "missing $out"
	sha256_sidecar "$out"
	printf '%s\n' "$out"
}

case "${1:-all}" in
binary) stage_binary ;;
sign)
	shift
	stage_sign "$@"
	;;
installer) stage_installer ;;
checksums) stage_checksums ;;
all)
	stage_binary
	stage_sign "$EXE"
	stage_installer
	stage_checksums
	;;
*) die "unknown stage $1 (all|binary|sign|installer|checksums)" ;;
esac
