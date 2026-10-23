#!/usr/bin/env bash
# Packages the desktop edition for macOS: universal (arm64 + x86_64) binary
# with lipo, Arenile.app, .dmg (hdiutil), optional Developer ID signature,
# notarization and stapling. Runs on macOS only (local Mac or macos-latest).
#
#   scripts/desktop/package-macos.sh
#
# Environment (besides common.sh):
#   MACOS_ARCHS                  default "arm64 amd64" (universal); one value builds a
#                                thin package: "arm64" -> _macos_arm64.dmg,
#                                "amd64" -> _macos_x86_64.dmg (smaller downloads)
#   MACOS_MIN_VERSION            LSMinimumSystemVersion / deployment target (default 13.0,
#                                the minimum of the Go toolchain in go.mod)
#   Signing (all optional; without them the build is ad-hoc signed and named -unsigned):
#   APPLE_SIGNING_IDENTITY       e.g. "Developer ID Application: Coste360 SRL (TEAMID)";
#                                found in the keychain when it is empty and a P12 is given
#   APPLE_CERTIFICATE_P12_BASE64 Developer ID Application certificate + key (.p12, base64),
#   APPLE_CERTIFICATE_PASSWORD   imported into a temporary keychain (CI)
#   Notarization (App Store Connect API key, optional; needs a signature):
#   APPLE_API_KEY_ID, APPLE_API_ISSUER_ID, APPLE_API_KEY_P8_BASE64
#   NOTARY_TIMEOUT               wait for each notarization (default 90m; the first
#                                submissions of a new team can take longer than 30m)
#
# Output in $DIST_DIR: Arenile_<version>_macos_<universal|arm64|x86_64>[-unsigned|-unnotarized|
# -notarization-pending].dmg (+ .sha256) and the bundle itself in $DIST_DIR/macos/Arenile.app.
# When Apple has not finished within NOTARY_TIMEOUT the signed dmg is still written, named
# -notarization-pending, with a .notarization.txt file next to it holding the submission ids
# and how to check and staple them later (docs/desktop-build.md); the script then exits 0 so the
# caller can keep the artifact and fail afterwards (desktop.yml does).
# shellcheck source=scripts/desktop/common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
NOTARY_TIMEOUT="${NOTARY_TIMEOUT:-90m}"

[ "$(uname -s)" = Darwin ] || die "package-macos.sh runs on macOS only (hdiutil, lipo, codesign)"

MACOS_ARCHS="${MACOS_ARCHS:-arm64 amd64}"
MACOS_MIN_VERSION="${MACOS_MIN_VERSION:-13.0}"
export MACOSX_DEPLOYMENT_TARGET="$MACOS_MIN_VERSION"

work="$(mktemp -d)"
keychain=""
cleanup() {
	if [ -n "$keychain" ]; then security delete-keychain "$keychain" >/dev/null 2>&1 || true; fi
	rm -rf "$work"
}
trap cleanup EXIT

# --- 1. binaries ------------------------------------------------------------
ensure_web
thin=()
for arch in $MACOS_ARCHS; do
	out="$work/arenile-desktop-$arch"
	clang_arch="$arch"
	[ "$arch" = amd64 ] && clang_arch=x86_64
	# With CGO (tray build) the host clang cross-compiles the other slice.
	CC="clang -arch $clang_arch" CGO_CFLAGS="-mmacosx-version-min=$MACOS_MIN_VERSION" \
		CGO_LDFLAGS="-mmacosx-version-min=$MACOS_MIN_VERSION" \
		go_build_launcher "$out" darwin "$arch"
	thin+=("$out")
done
bin="$work/arenile-desktop"
if [ "${#thin[@]}" -gt 1 ]; then
	lipo -create -output "$bin" "${thin[@]}"
	arch_label=universal
else
	cp "${thin[0]}" "$bin"
	arch_label="$MACOS_ARCHS"
	# Apple's name for the Intel slice, as in the Linux AppImage (x86_64).
	[ "$arch_label" = amd64 ] && arch_label=x86_64
fi
log "binary: $(lipo -archs "$bin")"

# --- 2. bundle --------------------------------------------------------------
app_root="$DIST_DIR/macos"
app="$app_root/$APP_NAME.app"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin" "$app/Contents/MacOS/arenile-desktop"
chmod 0755 "$app/Contents/MacOS/arenile-desktop"
cp "$PKG_DIR/icons/arenile.icns" "$app/Contents/Resources/arenile.icns"
cp "$DESKTOP_ROOT/LICENSE" "$app/Contents/Resources/LICENSE.txt"
printf 'APPL????' >"$app/Contents/PkgInfo"
short="$(numeric_version)"
sed -e "s|@BUNDLE_ID@|$BUNDLE_ID|g" \
	-e "s|@SHORT_VERSION@|$short|g" \
	-e "s|@BUNDLE_VERSION@|$(build_number)|g" \
	-e "s|@VERSION@|$VERSION|g" \
	-e "s|@MIN_MACOS@|$MACOS_MIN_VERSION|g" \
	"$PKG_DIR/macos/Info.plist.in" >"$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" >&2

# --- 3. signing ---------------------------------------------------------------
identity="${APPLE_SIGNING_IDENTITY:-}"
if [ -n "${APPLE_CERTIFICATE_P12_BASE64:-}" ]; then
	keychain="$work/signing.keychain-db"
	kc_pass="$(openssl rand -hex 24)"
	printf '%s' "$APPLE_CERTIFICATE_P12_BASE64" | base64 --decode >"$work/cert.p12"
	security create-keychain -p "$kc_pass" "$keychain"
	security set-keychain-settings -lut 3600 "$keychain"
	security unlock-keychain -p "$kc_pass" "$keychain"
	security import "$work/cert.p12" -k "$keychain" -P "${APPLE_CERTIFICATE_PASSWORD:-}" \
		-T /usr/bin/codesign -T /usr/bin/security >/dev/null
	security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$kc_pass" "$keychain" >/dev/null
	# Prepend the temporary keychain to the search list (keep the existing ones).
	# shellcheck disable=SC2046
	security list-keychains -d user -s "$keychain" $(security list-keychains -d user | tr -d '"')
	rm -f "$work/cert.p12"
	if [ -z "$identity" ]; then
		identity="$(security find-identity -v -p codesigning "$keychain" |
			sed -n 's/.*"\(Developer ID Application: [^"]*\)".*/\1/p' | head -n1)"
	fi
	[ -n "$identity" ] || die "no 'Developer ID Application' identity in the imported certificate"
fi

sign() { # path
	if [ -n "$identity" ]; then
		codesign --force --timestamp --options runtime \
			--entitlements "$PKG_DIR/macos/entitlements.plist" --sign "$identity" "$1"
	else
		codesign --force --sign - "$1"
	fi
}

# Submission ids of notarizations Apple had not finished within NOTARY_TIMEOUT
# ("<file> <id>" lines), written next to the dmg at the end.
pending=()

# notarize PATH [WAIT]: submits PATH (zip or dmg) and, unless WAIT is "nowait",
# waits up to NOTARY_TIMEOUT. Returns 0 when Accepted, 2 when still in progress
# (the id is recorded in $pending); a rejection prints the log and fails.
notarize() {
	local auth out status id wait="${2:-wait}"
	auth=(--key "$work/AuthKey.p8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
	# Submit first and wait separately: the submission id is then always known,
	# also when Apple takes longer than the wait (first submissions of a team
	# can take hours) and notarytool reports the timeout on stderr.
	out="$(xcrun notarytool submit "$1" "${auth[@]}" --output-format json 2>&1)" || true
	id="$(printf '%s' "$out" | sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p' | head -n1)"
	[ -n "$id" ] || die "notarization upload of $(basename "$1") failed: $out"
	if [ "$wait" = nowait ]; then
		log "notarization of $(basename "$1") submitted: $id (not waited for)"
	else
		log "notarization of $(basename "$1") submitted: $id (waiting up to $NOTARY_TIMEOUT)"
		xcrun notarytool wait "$id" "${auth[@]}" --timeout "$NOTARY_TIMEOUT" >&2 || true
	fi
	out="$(xcrun notarytool info "$id" "${auth[@]}" --output-format json 2>&1)" || true
	status="$(printf '%s' "$out" | sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p' | head -n1)"
	log "notarization of $(basename "$1"): ${status:-unknown} ($id)"
	case "$status" in
	Accepted) return 0 ;;
	"In Progress" | "")
		# Unknown status (network hiccup on `info`) is treated as pending: the
		# id is kept and can be checked later, the signed artifact is not lost.
		pending+=("$(basename "$1") $id")
		return 2
		;;
	*)
		xcrun notarytool log "$id" "${auth[@]}" >&2 || true
		die "notarization $id: $status"
		;;
	esac
}

can_notarize=0
if [ -n "$identity" ] && [ -n "${APPLE_API_KEY_ID:-}" ] && [ -n "${APPLE_API_ISSUER_ID:-}" ] &&
	[ -n "${APPLE_API_KEY_P8_BASE64:-}" ]; then
	printf '%s' "$APPLE_API_KEY_P8_BASE64" | base64 --decode >"$work/AuthKey.p8"
	can_notarize=1
fi

if [ -n "$identity" ]; then
	log "signing with Developer ID (hardened runtime)"
	label=""
	[ "$can_notarize" = 1 ] || label="-unnotarized"
else
	log "no Developer ID: ad-hoc signature, package labelled -unsigned"
	label="-unsigned"
fi
sign "$app"
codesign --verify --deep --strict --verbose=2 "$app" >&2

# Notarize and staple the .app too, so it opens offline once copied out of the dmg.
app_notarized=0
if [ "$can_notarize" = 1 ]; then
	ditto -c -k --keepParent "$app" "$work/Arenile.app.zip"
	if notarize "$work/Arenile.app.zip"; then
		app_notarized=1
		xcrun stapler staple "$app" >&2
		xcrun stapler validate "$app" >&2
		spctl --assess --type execute -vvv "$app" >&2
	else
		log "the .app notarization is still pending: the dmg is built and signed anyway"
	fi
fi

# --- 4. dmg -------------------------------------------------------------------
# Built under a temporary name: the final label is known only after notarization
# (the signature and the notarization ticket do not depend on the file name).
stage="$work/dmg"
mkdir -p "$stage"
ditto "$app" "$stage/$APP_NAME.app"
ln -s /Applications "$stage/Applications"
tmp_dmg="$work/${APP_NAME}_${VERSION_SAFE}_macos_${arch_label}.dmg"
hdiutil create -volname "$APP_NAME" -srcfolder "$stage" -fs HFS+ -format UDZO \
	-imagekey zlib-level=9 -ov "$tmp_dmg" >&2
hdiutil verify "$tmp_dmg" >/dev/null
if [ -n "$identity" ]; then
	codesign --force --timestamp --sign "$identity" "$tmp_dmg"
fi
if [ "$can_notarize" = 1 ]; then
	# When the .app is still pending, the dmg is submitted without waiting
	# (another NOTARY_TIMEOUT would only exceed the job timeout).
	wait_dmg=wait
	[ "$app_notarized" = 1 ] || wait_dmg=nowait
	if notarize "$tmp_dmg" "$wait_dmg"; then
		xcrun stapler staple "$tmp_dmg" >&2
		xcrun stapler validate "$tmp_dmg" >&2
		spctl --assess --type open --context context:primary-signature -vvv "$tmp_dmg" >&2
	fi
	[ "${#pending[@]}" -eq 0 ] || label="-notarization-pending"
fi

# Remove only earlier outputs of this architecture: the universal and the thin
# packages can sit side by side in $DIST_DIR.
prefix="$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_macos_${arch_label}"
rm -f "$prefix".dmg "$prefix"-*.dmg "$prefix"*.dmg.sha256 "$prefix"*.notarization.txt
dmg="$prefix$label.dmg"
mv "$tmp_dmg" "$dmg"
sha256_sidecar "$dmg"

if [ "${#pending[@]}" -gt 0 ]; then
	note="$dmg.notarization.txt"
	{
		echo "Notarization of $(basename "$dmg") still in progress after $NOTARY_TIMEOUT."
		echo "Submissions (file id):"
		printf '  %s\n' "${pending[@]}"
		echo
		echo "Check (App Store Connect API key, see docs/desktop-build.md):"
		echo "  xcrun notarytool info <id> --key AuthKey_<KEY_ID>.p8 --key-id <KEY_ID> --issuer <ISSUER_ID>"
		echo "  xcrun notarytool history --key AuthKey_<KEY_ID>.p8 --key-id <KEY_ID> --issuer <ISSUER_ID>"
		echo "When the dmg submission is Accepted (stapling needs no signing key):"
		echo "  xcrun stapler staple <dmg> && xcrun stapler validate <dmg>"
		echo "  then drop -notarization-pending from the name and regenerate the .sha256."
	} >"$note"
	log "notarization pending, see $(basename "$note")"
fi
printf '%s\n' "$dmg"
