#!/bin/sh
# Data survives a restart, and a SIGKILL mid-upload leaves the previous
# version intact with an empty staging directory. Runs on the host with
# only docker; clients run in the compose test containers.
set -eu
cd "$(dirname "$0")/../.."
dc() { docker compose -f compose.test.yaml "$@"; }
cli() { dc run --rm -T --entrypoint bash cli-tests -c "$1" 2>/dev/null; }
node_tool() { dc run --rm -T node-tests node tools/stream-object.mjs "$@" 2>/dev/null | tail -n 1; }
fail() { echo "FAIL persistence: $*" >&2; exit 1; }

B=persist
cli "aws s3api create-bucket --bucket $B >/dev/null 2>&1 || true"

echo "persistence: put, restart, get"
want=$(cli "head -c 3000000 /dev/urandom >/tmp/f && aws s3api put-object --bucket $B --key keep --body /tmp/f >/dev/null && sha256sum </tmp/f | cut -d' ' -f1")
dc restart faux3 >/dev/null 2>&1
dc up -d --wait faux3 >/dev/null 2>&1
got=$(cli "aws s3api get-object --bucket $B --key keep /tmp/o >/dev/null && sha256sum </tmp/o | cut -d' ' -f1")
[ -n "$want" ] && [ "$want" = "$got" ] || fail "after restart sha256 $got want $want"

echo "persistence: SIGKILL during a 200 MB upload"
v1=$(cli "head -c 1000000 /dev/urandom >/tmp/f && aws s3api put-object --bucket $B --key big --body /tmp/f >/dev/null && sha256sum </tmp/f | cut -d' ' -f1")
# About 4 s of upload at 20 ms per MiB; kill it after 2 s.
node_tool put "$B" big 209715200 20 >/dev/null 2>&1 &
upload=$!
sleep 2
dc kill -s KILL faux3 >/dev/null 2>&1
wait "$upload" && fail "upload finished before the kill; nothing was tested"
dc up -d --wait faux3 >/dev/null 2>&1
got=$(cli "aws s3api get-object --bucket $B --key big /tmp/o >/dev/null && sha256sum </tmp/o | cut -d' ' -f1")
[ "$v1" = "$got" ] || fail "previous version not intact: sha256 $got want $v1"

leftover=$(docker run --rm -v faux3-test_faux3-test-data:/data alpine:3.22 ls -A /data/.faux3/tmp)
[ -z "$leftover" ] || fail ".faux3/tmp not empty after restart: $leftover"

cli "aws s3api delete-object --bucket $B --key keep && aws s3api delete-object --bucket $B --key big && aws s3api delete-bucket --bucket $B"
echo "PASS persistence"
