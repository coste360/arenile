#!/usr/bin/env bash
# PreToolUse hook for Claude Code (Edit|Write|MultiEdit).
#
# Reads the hook JSON payload on stdin and blocks (exit 2, reason on stderr)
# edits to:
#   - generated Go code:          *.gen.go, *_gen.go
#   - generated TypeScript code:  web/shared/api/generated/**, web/shared/src/map/kit/generated/**
#   - migrations already merged:  internal/migrations/** present on branch main
#
# Any other situation (unparseable input, no file path, not a git repo,
# no main branch) exits 0 so the hook never blocks unrelated work.
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

file_path="$(json_file_path "$input")"
[ -n "$file_path" ] || exit 0

# Resolve the repository root: prefer the project dir given by Claude Code.
root="${CLAUDE_PROJECT_DIR:-}"
if [ -z "$root" ]; then
	root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
root="${root%/}"

# Repository-relative path (absolute paths outside the repo are left untouched).
case "$file_path" in
/*)
	case "$file_path" in
	"$root"/*) rel="${file_path#"$root"/}" ;;
	*)
		# Handle symlinked roots (e.g. /tmp vs /private/tmp on macOS).
		real_root="$(cd "$root" 2>/dev/null && pwd -P || printf '%s' "$root")"
		case "$file_path" in
		"$real_root"/*) rel="${file_path#"$real_root"/}" ;;
		*) exit 0 ;;
		esac
		;;
	esac
	;;
*) rel="${file_path#./}" ;;
esac

block() {
	printf 'BLOCKED by scripts/agents/guard-generated.sh: %s\n%s\n' "$rel" "$1" >&2
	exit 2
}

case "$rel" in
*.gen.go)
	block "This is generated Go code. Edit api/openapi.yaml (or the generator config) and run 'make gen' instead."
	;;
*_gen.go)
	block "This is generated Go code (map kit: web/scripts/gen-map-kit.mjs). Edit the source (assets/beach-map-kit or the generator) and run 'make gen' instead."
	;;
web/shared/api/generated/*)
	block "This is the generated TypeScript API client. Edit api/openapi.yaml and run 'make gen' instead."
	;;
web/shared/src/map/kit/generated/*)
	block "This is the generated beach map kit. Edit assets/beach-map-kit (manifest + svg) or web/scripts/gen-map-kit.mjs and run 'make gen' instead."
	;;
internal/migrations/*)
	top="$(git -C "$root" rev-parse --show-toplevel 2>/dev/null || true)"
	real_root="$(cd "$root" 2>/dev/null && pwd -P || printf '%s' "$root")"
	real_top="$(cd "$top" 2>/dev/null && pwd -P || printf '%s' "$top")"
	# Only consult git when the project root is the repository root, so that
	# a nested non-git directory never inherits an unrelated parent repo.
	if [ -n "$top" ] && [ "$real_top" = "$real_root" ]; then
		for ref in main origin/main; do
			if git -C "$root" rev-parse --verify --quiet "$ref^{commit}" >/dev/null 2>&1 &&
				git -C "$root" cat-file -e "$ref:$rel" 2>/dev/null; then
				block "This migration is already on $ref and must never be modified. Add a new migration with the next number for both internal/migrations/sqlite and internal/migrations/postgres (see /new-migration)."
			fi
		done
	fi
	;;
esac

exit 0
