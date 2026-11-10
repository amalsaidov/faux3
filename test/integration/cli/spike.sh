#!/usr/bin/env bash
# Wire-capture spike: issue each CLI upload transport once against faux3 so
# the server debug log records the payload mode headers. Faux3 answers 501
# at this stage, so failures are expected and only reported.
set -uo pipefail
aws --version
tmp=$(mktemp)
printf 'hello faux3 wire capture\n' >"$tmp"
try() {
  local name=$1
  shift
  if out=$("$@" 2>&1); then echo "$name: ok"; else echo "$name: failed (expected): $(tail -n1 <<<"$out")"; fi
}
try s3api-put-object aws s3api put-object --bucket spike --key s3api --body "$tmp"
try s3-cp-file aws s3 cp "$tmp" s3://spike/cp-file
try s3-cp-stdin aws s3 cp - s3://spike/cp-stdin <"$tmp"
url=$(aws s3 presign s3://spike/s3api --expires-in 300)
echo "s3-presign: ${url%%X-Amz-Signature=*}X-Amz-Signature=..."
try presigned-get curl -sS -f -o /dev/null "$url"
rm -f "$tmp"
