# aws s3 presign then a plain curl GET.
. /tests/lib.sh
B=$(new_bucket presign)
trap 'cleanup_bucket "$B"; rm -rf "$TMP"' EXIT

aws s3 mb "s3://$B" >/dev/null
head -c 4096 /dev/urandom >"$TMP/body"
aws s3 cp "$TMP/body" "s3://$B/some key+plus" --only-show-errors
url=$(aws s3 presign "s3://$B/some key+plus" --expires-in 300)
curl -fsS -o "$TMP/out" "$url"
same_file "$TMP/body" "$TMP/out"

# A tampered signature (same length, one hex digit changed) is rejected.
last=${url: -1}
if [ "$last" = 0 ]; then flip=1; else flip=0; fi
code=$(curl -s -o /dev/null -w '%{http_code}' "${url%?}$flip")
[ "$code" = 403 ] || { echo "tampered presign status $code" >&2; exit 1; }
