#!/usr/bin/env bash
# Runs a command with ARENILE_TEST_POSTGRES_URL pointing at an ephemeral
# PostgreSQL container (make test-db, make test-race). When the variable is
# already set the command runs against that server; without Docker it runs
# with the variable unset (the integration tests then cover SQLite only and
# say so).
#
#   scripts/with-postgres.sh <label> <command...>
set -euo pipefail

label="${1:?label}"
shift
image="${POSTGRES_IMAGE:-postgres:17-alpine}"

if [ -n "${ARENILE_TEST_POSTGRES_URL:-}" ]; then
	echo "$label: using ARENILE_TEST_POSTGRES_URL from the environment"
	exec "$@"
fi
if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
	echo "WARNING: docker not available: $label runs on SQLite only" >&2
	exec "$@"
fi

cid="$(docker run -d --rm \
	-e POSTGRES_USER=arenile -e POSTGRES_PASSWORD=arenile -e POSTGRES_DB=arenile_test \
	-p 127.0.0.1::5432 "$image" \
	-c fsync=off -c synchronous_commit=off -c full_page_writes=off -c max_connections=200)"
trap 'docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT INT TERM
port="$(docker port "$cid" 5432/tcp | head -n1 | sed 's/.*://')"
echo "$label: postgres container ${cid:0:12} on 127.0.0.1:$port"
ready=0
for _ in $(seq 1 60); do
	if docker exec "$cid" pg_isready -U arenile -d arenile_test -h 127.0.0.1 >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 1
done
if [ "$ready" != 1 ]; then
	echo "$label: postgres did not become ready" >&2
	docker logs "$cid" >&2 || true
	exit 1
fi
ARENILE_TEST_POSTGRES_URL="postgres://arenile:arenile@127.0.0.1:$port/arenile_test?sslmode=disable" "$@"
