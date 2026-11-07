#!/bin/sh
# Runs the Go tests and the web lint/build. Used by the worker and by CI.
# When DATABASE_URL is unset, starts a throwaway Postgres cluster in a temp dir.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
CACHE="$ROOT/.cache"

# Keep caches the environment already provides (the worker sets GOTMPDIR and
# GOCACHE to an executable, writable mount); otherwise use a folder in the repo.
export GOTOOLCHAIN=local
export GOTMPDIR="${GOTMPDIR:-$CACHE/go-tmp}"
export GOCACHE="${GOCACHE:-$CACHE/go-build}"
export GOPATH="${GOPATH:-$CACHE/gopath}"
export NEXT_TELEMETRY_DISABLED=1
export npm_config_cache="${npm_config_cache:-$CACHE/npm}"
mkdir -p "$GOTMPDIR" "$GOCACHE" "$GOPATH" "$npm_config_cache"
sh "$ROOT/scripts/test-ops.sh"

PGDIR=""
cleanup() {
	if [ -n "$PGDIR" ]; then
		pg_ctl -D "$PGDIR/data" -m fast stop >/dev/null 2>&1 || true
		rm -rf "$PGDIR"
	fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -z "${DATABASE_URL:-}" ]; then
	PGDIR=$(mktemp -d)
	PGPORT=54329
	initdb -U postgres --auth=trust -D "$PGDIR/data" >/dev/null
	pg_ctl -w -D "$PGDIR/data" -l "$PGDIR/pg.log" \
		-o "-c listen_addresses=127.0.0.1 -p $PGPORT -k $PGDIR" start >/dev/null
	DATABASE_URL="postgres://postgres@127.0.0.1:$PGPORT/postgres?sslmode=disable"
	export DATABASE_URL
fi

echo "== server: go test"
(cd "$ROOT/server" && go vet ./... && go test ./...)

echo "== web: lint and build"
# npm ci deletes node_modules first, which fails on a read-only checkout, so
# only install when the existing node_modules does not match package.json.
(cd "$ROOT/web" \
	&& { npm ls --offline >/dev/null 2>&1 || npm ci --prefer-offline --no-audit --no-fund; } \
	&& npm run lint 	&& if [ -w . ]; then npm run build; else
		# the omi post-run hook mounts the worktree read-only, so next build cannot write .next;
		# typecheck instead. The worker run and CI do the full build.
		echo "web is read-only: next build skipped, running tsc --noEmit"; npx --no-install tsc --noEmit --incremental false; fi)
