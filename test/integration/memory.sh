#!/bin/sh
# Streams a 1 GiB object through faux3 running under a 128 MiB memory
# limit, reads it back, and checks the container was never OOM-killed and
# stayed under the limit. Runs on the host with only docker.
set -eu
cd "$(dirname "$0")/../.."
dc() { docker compose -f compose.test.yaml "$@"; }
fail() { echo "FAIL memory: $*" >&2; exit 1; }
SIZE=1073741824
LIMIT_MIB=128

dc --profile mem up -d --wait faux3-memtest >/dev/null 2>&1
cid=$(dc --profile mem ps -q faux3-memtest)
[ -n "$cid" ] || fail "faux3-memtest did not start"

stats=$(mktemp)
trap 'kill "$poll" 2>/dev/null || true; rm -f "$stats"' EXIT
(while :; do docker stats --no-stream --format '{{.MemUsage}}' "$cid" 2>/dev/null >>"$stats"; done) &
poll=$!

tool() { dc run --rm -T -e S3_ENDPOINT=http://faux3-memtest:9000 node-tests node tools/stream-object.mjs "$@" 2>/dev/null | tail -n 1; }
echo "memory: put 1 GiB"
want=$(tool put mem big "$SIZE")
echo "memory: get 1 GiB"
got=$(tool get mem big)
kill "$poll" 2>/dev/null || true
wait "$poll" 2>/dev/null || true
[ -n "$want" ] && [ "$want" = "$got" ] || fail "sha256 $got want $want"

[ "$(docker inspect -f '{{.State.OOMKilled}}' "$cid")" = false ] || fail "container was OOM-killed"
[ "$(docker inspect -f '{{.State.Running}}' "$cid")" = true ] || fail "container is not running"
[ "$(docker inspect -f '{{.RestartCount}}' "$cid")" = 0 ] || fail "container restarted"

# MemUsage looks like "41.3MiB / 128MiB"; convert the usage to MiB.
peak=$(awk '{
  v = $1; n = v + 0
  if (v ~ /GiB/) n *= 1024; else if (v ~ /KiB/) n /= 1024; else if (v ~ /[0-9]B$/ && v !~ /iB/) n /= 1048576
  if (n > max) max = n
} END { printf "%.1f", max }' "$stats")
samples=$(wc -l <"$stats" | tr -d ' ')
echo "memory: peak ${peak} MiB over ${samples} samples (limit ${LIMIT_MIB} MiB)"
[ "$samples" -gt 0 ] || fail "no docker stats samples"
awk -v p="$peak" -v l="$LIMIT_MIB" 'BEGIN { exit !(p < l) }' || fail "peak ${peak} MiB not below ${LIMIT_MIB} MiB"

tool get mem big >/dev/null # warm check that it is still served
dc --profile mem stop faux3-memtest >/dev/null 2>&1
echo "PASS memory"
