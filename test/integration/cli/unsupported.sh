# High-level commands that need operations outside the strict nine fail
# with NotImplemented. docs/limitations.md lists them.
. /tests/lib.sh
B=$(new_bucket unsup)
trap 'cleanup_bucket "$B"; rm -rf "$TMP"' EXIT

aws s3 mb "s3://$B" >/dev/null
echo x | aws s3 cp - "s3://$B/a" --only-show-errors

# aws s3 mv between S3 locations uses CopyObject.
expect_fail NotImplemented aws s3 mv "s3://$B/a" "s3://$B/b"
# S3-to-S3 aws s3 cp uses CopyObject.
expect_fail NotImplemented aws s3 cp "s3://$B/a" "s3://$B/c"
# Nothing was moved or copied.
aws s3api head-object --bucket "$B" --key a >/dev/null
expect_fail "404\|Not Found" aws s3api head-object --bucket "$B" --key b
