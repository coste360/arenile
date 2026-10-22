#!/usr/bin/env bash
# PostToolUse hook for Claude Code (Edit|Write|MultiEdit).
#
# Formats the file that was just edited:
#   - *.go (not generated)              -> gofumpt + goimports via "go tool"
#   - web/** ts/tsx/js/jsx/css/json/md  -> Prettier from the web/ workspace
#
# Formatting is best effort: a missing tool or a formatter error never fails
# the hook (always exit 0), it only prints a note on stderr.
set -u

input="$(cat || true)"

json_file_path() {
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$1" | jq -r '.tool_input.file_path // .tool_input.path // empty' 2>/dev/null
		return
	fi
	if command -v python3 >/dev/null 2>&1; then
		printf '%s' "$1" | python3 -I -c '
import json, sys
try:
    data = json.load(sys.stdin)
    ti = data.get("tool_input") or {}
    print(ti.get("file_path") or ti.get("path") or "")
except Exception:
    pass
' 2>/dev/null
	fi
}

note() { printf 'format-file.sh: %s\n' "$1" >&2; }

file_path="$(json_file_path "$input")"
[ -n "$file_path" ] || exit 0

root="${CLAUDE_PROJECT_DIR:-}"
if [ -z "$root" ]; then
	root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
root="${root%/}"

case "$file_path" in
/*) abs="$file_path" ;;
*) abs="$root/${file_path#./}" ;;
esac
[ -f "$abs" ] || exit 0

rel="${abs#"$root"/}"

case "$rel" in
*.gen.go | web/shared/api/generated/*)
	exit 0
	;;
*.go)
	if command -v go >/dev/null 2>&1; then
		(cd "$root" && go tool gofumpt -w "$abs") >/dev/null 2>&1 || note "gofumpt unavailable or failed on $rel"
		(cd "$root" && go tool goimports -local github.com/coste360/arenile -w "$abs") >/dev/null 2>&1 ||
			note "goimports unavailable or failed on $rel"
	else
		note "go not found, skipping Go formatting"
	fi
	;;
web/*.ts | web/*.tsx | web/*.js | web/*.jsx | web/*.mjs | web/*.cjs | web/*.css | web/*.json | web/*.md | web/*.html | web/*.yaml | web/*.yml)
	case "$rel" in
	*/node_modules/* | */dist/*) exit 0 ;;
	esac
	if command -v npx >/dev/null 2>&1 && [ -d "$root/web/node_modules" ]; then
		(cd "$root/web" && npx --no-install prettier --write --log-level warn "$abs") >/dev/null 2>&1 ||
			note "prettier unavailable or failed on $rel"
	else
		note "prettier not installed (run make setup), skipping $rel"
	fi
	;;
esac

exit 0
