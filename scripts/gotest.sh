#!/usr/bin/env bash
# Runs `go test` with the given arguments. When GOTEST_RERUN_FAILS is set (CI)
# and gotestsum is installed, runs through gotestsum instead and reruns only
# the tests that failed, up to GOTEST_RERUN_FAILS times: a flaky test is
# retried on its own, never the whole package or job. A test that fails on
# every attempt still fails the run.
#
#   scripts/gotest.sh [go test flags...] <packages...>
#   GOTEST_RERUN_FAILS=2 scripts/gotest.sh -tags=integration ./...
set -euo pipefail

rerun="${GOTEST_RERUN_FAILS:-}"
if [ -z "$rerun" ] || [ "$rerun" = 0 ] || ! command -v gotestsum >/dev/null 2>&1; then
	if [ -n "$rerun" ] && [ "$rerun" != 0 ]; then
		echo "WARNING: gotestsum not installed: running go test without reruns" >&2
	fi
	exec go test "$@"
fi

# gotestsum wants the packages in --packages and the go test flags after --.
flags=()
pkgs=()
expect_value=0
for arg in "$@"; do
	if [ "$expect_value" = 1 ]; then
		flags+=("$arg")
		expect_value=0
		continue
	fi
	case "$arg" in
	-run | -skip | -tags | -timeout | -count | -p | -parallel | -cpu | -coverprofile | -covermode | -coverpkg)
		flags+=("$arg")
		expect_value=1
		;;
	-*) flags+=("$arg") ;;
	*) pkgs+=("$arg") ;;
	esac
done
[ "${#pkgs[@]}" -gt 0 ] || pkgs=(./...)

exec gotestsum --format testname \
	--rerun-fails="$rerun" --rerun-fails-max-failures=10 \
	--packages="${pkgs[*]}" -- ${flags[@]+"${flags[@]}"}
