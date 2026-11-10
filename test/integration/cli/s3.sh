# High-level aws s3 commands against faux3.
. /tests/lib.sh
B=$(new_bucket s3hl)
trap 'cleanup_bucket "$B"; rm -rf "$TMP"' EXIT

aws s3 mb "s3://$B" >/dev/null
head -c 1000000 /dev/urandom >"$TMP/1mb"
head -c 20000000 /dev/urandom >"$TMP/20mb"

aws s3 cp "$TMP/1mb" "s3://$B/files/1mb" --only-show-errors
aws s3 cp "$TMP/20mb" "s3://$B/files/20mb" --only-show-errors
aws s3 cp "s3://$B/files/1mb" "$TMP/1mb.out" --only-show-errors
aws s3 cp "s3://$B/files/20mb" "$TMP/20mb.out" --only-show-errors
same_file "$TMP/1mb" "$TMP/1mb.out"
same_file "$TMP/20mb" "$TMP/20mb.out"

echo "from stdin" | aws s3 cp - "s3://$B/stdin.txt" --only-show-errors
[ "$(aws s3 cp "s3://$B/stdin.txt" - --only-show-errors)" = "from stdin" ]

mkdir -p "$TMP/tree/sub"
echo a >"$TMP/tree/a.txt"
echo b >"$TMP/tree/sub/b.txt"
echo "c c" >"$TMP/tree/sub/c c.txt"
aws s3 sync "$TMP/tree" "s3://$B/tree" --only-show-errors
# A second sync uploads nothing.
[ -z "$(aws s3 sync "$TMP/tree" "s3://$B/tree")" ]
aws s3 sync "s3://$B/tree" "$TMP/tree.out" --only-show-errors
same_file "$TMP/tree/sub/c c.txt" "$TMP/tree.out/sub/c c.txt"

listing=$(aws s3 ls "s3://$B" --recursive)
for k in files/1mb files/20mb stdin.txt tree/a.txt tree/sub/b.txt "tree/sub/c c.txt"; do
  grep -q " $k\$" <<<"$listing" || { echo "missing $k in listing" >&2; echo "$listing" >&2; exit 1; }
done

aws s3 rm "s3://$B/stdin.txt" --only-show-errors
if aws s3 ls "s3://$B/stdin.txt" >/dev/null; then echo "stdin.txt still listed" >&2; exit 1; fi

# aws s3 sync --delete and aws s3 rm --recursive delete one key at a time
# with DeleteObject in CLI v2, so they work without DeleteObjects.
rm "$TMP/tree/a.txt"
aws s3 sync "$TMP/tree" "s3://$B/tree" --delete --only-show-errors
if aws s3 ls "s3://$B/tree/a.txt" >/dev/null; then echo "sync --delete kept tree/a.txt" >&2; exit 1; fi
aws s3 rm "s3://$B" --recursive --only-show-errors
[ -z "$(aws s3 ls "s3://$B" --recursive)" ]
aws s3 rb "s3://$B" >/dev/null
