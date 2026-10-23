#!/usr/bin/env bash
# Packages the desktop edition for macOS: universal (arm64 + x86_64) binary
# with lipo, Arenile.app, .dmg (hdiutil), optional Developer ID signature,
# notarization and stapling. Runs on macOS only (local Mac or macos-latest).
#
#   scripts/desktop/package-macos.sh
#
# Environment (besides common.sh):
#   MACOS_ARCHS                  default "arm64 amd64" (one value = thin binary)
#   MACOS_MIN_VERSION            LSMinimumSystemVersion / deployment target (default 13.0,
#                                the minimum of the Go toolchain in go.mod)
#   Signing (all optional; without them the build is ad-hoc signed and named -unsigned):
#   APPLE_SIGNING_IDENTITY       e.g. "Developer ID Application: Coste360 SRL (TEAMID)";
#                                found in the keychain when it is empty and a P12 is given
#   APPLE_CERTIFICATE_P12_BASE64 Developer ID Application certificate + key (.p12, base64),
#   APPLE_CERTIFICATE_PASSWORD   imported into a temporary keychain (CI)
#   Notarization (App Store Connect API key, optional; needs a signature):
#   APPLE_API_KEY_ID, APPLE_API_ISSUER_ID, APPLE_API_KEY_P8_BASE64
#
# Output in $DIST_DIR: Arenile_<version>_macos_universal[-unsigned|-unnotarized].dmg
# (+ .sha256) and the bundle itself in $DIST_DIR/macos/Arenile.app.
# shellcheck source=scripts/desktop/common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

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

notarize() { # path (zip or dmg)
	local out status id
	out="$(xcrun notarytool submit "$1" --key "$work/AuthKey.p8" --key-id "$APPLE_API_KEY_ID" \
		--issuer "$APPLE_API_ISSUER_ID" --wait --timeout 30m --output-format json)" || true
	status="$(printf '%s' "$out" | sed -n 's/.*"status" *: *"\([^"]*\)".*/\1/p')"
	id="$(printf '%s' "$out" | sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p')"
	log "notarization of $(basename "$1"): ${status:-unknown} (${id:-no id})"
	if [ "$status" != Accepted ]; then
		[ -n "$id" ] && xcrun notarytool log "$id" --key "$work/AuthKey.p8" \
			--key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID" >&2 || true
		die "notarization failed"
	fi
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
if [ "$can_notarize" = 1 ]; then
	ditto -c -k --keepParent "$app" "$work/app.zip"
	notarize "$work/app.zip"
	xcrun stapler staple "$app" >&2
	xcrun stapler validate "$app" >&2
	spctl --assess --type execute -vvv "$app" >&2
fi

# --- 4. dmg -------------------------------------------------------------------
dmg="$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_macos_${arch_label}${label}.dmg"
rm -f "$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_macos_"*.dmg "$DIST_DIR/${APP_NAME}_${VERSION_SAFE}_macos_"*.dmg.sha256
stage="$work/dmg"
mkdir -p "$stage"
ditto "$app" "$stage/$APP_NAME.app"
ln -s /Applications "$stage/Applications"
hdiutil create -volname "$APP_NAME" -srcfolder "$stage" -fs HFS+ -format UDZO \
	-imagekey zlib-level=9 -ov "$dmg" >&2
if [ -n "$identity" ]; then
	codesign --force --timestamp --sign "$identity" "$dmg"
fi
if [ "$can_notarize" = 1 ]; then
	notarize "$dmg"
	xcrun stapler staple "$dmg" >&2
	xcrun stapler validate "$dmg" >&2
	spctl --assess --type open --context context:primary-signature -vvv "$dmg" >&2
fi
hdiutil verify "$dmg" >/dev/null
sha256_sidecar "$dmg"
printf '%s\n' "$dmg"
