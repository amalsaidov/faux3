# Changelog

All notable changes to this project are documented in this file.
The format is based on Keep a Changelog, and this project uses Semantic Versioning (pre-1.0, 0.x).

## [Unreleased]

### Added

- S3 API server (`internal/s3api`) serving the nine supported operations: CreateBucket, ListBuckets, HeadBucket, DeleteBucket, PutObject, GetObject, HeadObject, DeleteObject, and ListObjectsV2.
- Path-style routing that parses the raw path by hand, so keys containing `..`, `//`, encoded slashes, or a leading slash round-trip unchanged.
- SigV4 header and presigned authentication on every request except `GET /-/healthz` and CORS preflight.
- PutObject with plain, unsigned, and aws-chunked payloads, `x-amz-checksum-*` and `Content-MD5` verification, content headers, `x-amz-meta-*` user metadata, and `If-None-Match: *` conditional writes.
- GetObject and HeadObject with single byte ranges, conditional requests, `response-*` header overrides, and stored checksums returned when `x-amz-checksum-mode: ENABLED` is sent.
- ListObjectsV2 with prefix, delimiter, max-keys, continuation tokens, start-after, fetch-owner, and `encoding-type=url`.
- A minimal GetBucketLocation stub for AWS CLI region discovery.
- 501 NotImplemented for every other subresource, POST requests, ListObjects V1, CopyObject, tagging, non-private canned ACLs, and server-side encryption headers.
- Opt-in CORS through `FAUX3_CORS_ORIGINS`, with preflight answered before authentication.
- `x-amz-request-id` and `x-amz-id-2` on every response, panic recovery, and a JSON access log line per request.
- Real-client integration suite in Docker Compose (`scripts/integration.sh`): AWS SDK for JavaScript v3 and AWS CLI v2 tests, a restart and SIGKILL persistence test, and a 1 GiB upload under a 128 MiB memory limit.
- User metadata response headers are sent lowercase (`x-amz-meta-color`), so the AWS CLI reports the key as written.
- Unsigned aws-chunked uploads stream chunks of any size; the AWS CLI sends a whole file as one chunk. Only signed chunks keep the 16 MiB buffer cap.
