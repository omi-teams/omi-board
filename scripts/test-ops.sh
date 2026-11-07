#!/bin/sh
# Regression checks for configurable compose ports and test-runner signals.
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)

mapping=$(sed -n '/^  server:/,/^  web:/s/^      - "\(8080:.*\)"$/\1/p' "$ROOT/docker-compose.yml")
default_mapping=$(unset PORT; eval "printf '%s' \"$mapping\"")
custom_mapping=$(PORT=9090; eval "printf '%s' \"$mapping\"")
[ "$default_mapping" = '8080:8080' ]
[ "$custom_mapping" = '8080:9090' ]

# The post-run hook mounts the checkout read-only and supplies writable GOTMPDIR.
TESTDIR=$(mktemp -d "${GOTMPDIR:-${TMPDIR:-/tmp}}/ops-test.XXXXXX")
trap 'rm -rf "$TESTDIR"' EXIT
cleanup_code=$(sed -n '/^PGDIR=""$/,/^if \[ -z /{ /^if \[ -z /!p; }' "$ROOT/scripts/test.sh")

for signal in INT TERM; do
    mkdir "$TESTDIR/cluster"
    status=0
    (
        export TESTDIR
        sh -c "$cleanup_code
pg_ctl() { printf '%s\n' stopped >> \"\$TESTDIR/cleanup\"; }
PGDIR=\"\$TESTDIR/cluster\"
kill -$signal \"\$\$\"
printf '%s\n' continued-after-signal
" > "$TESTDIR/output"
    ) || status=$?
    case "$signal" in
        INT) [ "$status" -eq 130 ] ;;
        TERM) [ "$status" -eq 143 ] ;;
    esac
    [ ! -s "$TESTDIR/output" ]
    [ ! -d "$TESTDIR/cluster" ]
    [ "$(cat "$TESTDIR/cleanup")" = stopped ]
    rm "$TESTDIR/cleanup"
done
echo 'ops regression checks passed (ports, INT/TERM exit and cleanup)'
