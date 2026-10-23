#!/usr/bin/env bash
# Regenerates the desktop package icons from the brand SVGs in
# packaging/desktop/icons (the same mark as web/admin/public/icon.svg).
#
#   arenile.svg        full-bleed mark -> arenile.ico (Windows), arenile.png (Linux, 512 px)
#   arenile-macos.svg  mark on the macOS 1024 grid (824 px body, 100 px margin) -> arenile.icns
#
# The outputs are committed, so packaging needs no image tooling on the CI
# runners. Run this only when the brand mark changes. Requires ImageMagick 7
# (`magick`) and, for the .icns, macOS `iconutil`.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dir="$root/packaging/desktop/icons"
command -v magick >/dev/null || { echo "gen-icons: ImageMagick 7 (magick) is required" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

render() { # svg size out
	magick -background none -density 1200 "$1" -resize "${2}x${2}" -strip "PNG32:$3"
}

# Linux (hicolor 512x512, also used by the AppImage as .DirIcon).
render "$dir/arenile.svg" 512 "$dir/arenile.png"

# Windows: multi-resolution .ico (16..256).
for s in 16 24 32 48 64 128 256; do render "$dir/arenile.svg" "$s" "$work/ico-$s.png"; done
magick "$work"/ico-16.png "$work"/ico-24.png "$work"/ico-32.png "$work"/ico-48.png \
	"$work"/ico-64.png "$work"/ico-128.png "$work"/ico-256.png "$dir/arenile.ico"

# macOS: .iconset -> .icns.
if command -v iconutil >/dev/null; then
	set_dir="$work/arenile.iconset"
	mkdir -p "$set_dir"
	for s in 16 32 128 256 512; do
		render "$dir/arenile-macos.svg" "$s" "$set_dir/icon_${s}x${s}.png"
		render "$dir/arenile-macos.svg" "$((s * 2))" "$set_dir/icon_${s}x${s}@2x.png"
	done
	iconutil -c icns -o "$dir/arenile.icns" "$set_dir"
else
	echo "gen-icons: iconutil not found (macOS only): arenile.icns left unchanged" >&2
fi

ls -l "$dir"
