#!/usr/bin/env bash
# Regenerates the desktop package icons in packaging/desktop/icons from the
# Arenile brand kit (assets/brand/arenile, ADR 0018):
#
#   arenile/arenile-app-icon-1024.png          full-bleed app icon -> arenile.ico (Windows), arenile.png (Linux, 512 px)
#   arenile/png/arenile-app-icon-macos-1024.png  app icon on the macOS 1024 grid (824 px body, 100 px margin) -> arenile.icns
#   arenile/arenile-app-icon.svg               copied as arenile.svg (Linux hicolor scalable icon, text-free)
#
# The outputs are committed, so packaging needs no image tooling on the CI
# runners. Run this only when the brand kit changes (after
# scripts/brand/build-brand.mjs). Requires ImageMagick 7 (`magick`) and, for
# the .icns, macOS `iconutil`.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dir="$root/packaging/desktop/icons"
brand="$root/assets/brand/arenile"
full="$brand/arenile-app-icon-1024.png"
macos="$brand/png/arenile-app-icon-macos-1024.png"
command -v magick >/dev/null || { echo "gen-icons: ImageMagick 7 (magick) is required" >&2; exit 1; }
for f in "$full" "$macos"; do
	[[ -f "$f" ]] || { echo "gen-icons: missing $f" >&2; exit 1; }
done

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

resize() { # png size out
	magick "$1" -filter Lanczos -resize "${2}x${2}" -strip "PNG32:$3"
}

# Linux (hicolor 512x512, also used by the AppImage as .DirIcon) and the scalable icon.
resize "$full" 512 "$dir/arenile.png"
cp "$brand/arenile-app-icon.svg" "$dir/arenile.svg"

# Windows: multi-resolution .ico (16..256).
sizes=(16 24 32 48 64 128 256)
frames=()
for s in "${sizes[@]}"; do
	resize "$full" "$s" "$work/ico-$s.png"
	frames+=("$work/ico-$s.png")
done
magick "${frames[@]}" "$dir/arenile.ico"

# macOS: .iconset -> .icns.
if command -v iconutil >/dev/null; then
	set_dir="$work/arenile.iconset"
	mkdir -p "$set_dir"
	for s in 16 32 128 256 512; do
		resize "$macos" "$s" "$set_dir/icon_${s}x${s}.png"
		resize "$macos" "$((s * 2))" "$set_dir/icon_${s}x${s}@2x.png"
	done
	iconutil -c icns -o "$dir/arenile.icns" "$set_dir"
else
	echo "gen-icons: iconutil not found (macOS only): arenile.icns left unchanged" >&2
fi

ls -l "$dir"
