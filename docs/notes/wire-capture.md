# Wire capture: client payload modes over plain http

Captured 2026-11-10 with `FAUX3_LOG_LEVEL=debug` against the Phase 2 server
(every S3 request answers 501, the body is drained and its first 200 bytes
logged). Reproduce with the commands in plan Phase 2 Verify; the scripts are
`test/integration/node/spike/capture.mjs` and `test/integration/cli/spike.sh`.

Versions:

- AWS SDK for JavaScript v3: `@aws-sdk/client-s3`, `@aws-sdk/lib-storage`,
  `@aws-sdk/s3-request-presigner` 3.1165.0 on Node v22.23.4 (`node:22-alpine`).
- AWS CLI v2: aws-cli/2.39.7 Python/3.14.8 (`amazon/aws-cli:2.39.7`), config
  `addressing_style = path`, `multipart_threshold = 64MB`.

Body: the 25-byte string `hello faux3 wire capture\n`.

| Transport | x-amz-content-sha256 | Content-Encoding | Length headers | x-amz-trailer | Checksum headers |
| --- | --- | --- | --- | --- | --- |
| SDK PutObject Buffer | hex sha256 of body | none | Content-Length 25 | none | x-amz-sdk-checksum-algorithm CRC32, x-amz-checksum-crc32 header |
| SDK PutObject string | hex sha256 of body | none | Content-Length 25 | none | x-amz-sdk-checksum-algorithm CRC32, x-amz-checksum-crc32 header |
| SDK PutObject Readable stream + ContentLength | STREAMING-UNSIGNED-PAYLOAD-TRAILER | aws-chunked | Transfer-Encoding chunked, no Content-Length, x-amz-decoded-content-length 25 | x-amz-checksum-crc32 | x-amz-sdk-checksum-algorithm CRC32; checksum value in trailer |
| SDK lib-storage Upload (under part size) | hex sha256 of body | none | Content-Length 25 | none | x-amz-sdk-checksum-algorithm CRC32, x-amz-checksum-crc32 header |
| SDK presigned PUT (fetch) | X-Amz-Content-Sha256=UNSIGNED-PAYLOAD in query | none | Content-Length 25 | none | query x-amz-sdk-checksum-algorithm=CRC32 and x-amz-checksum-crc32=AAAAAA== (CRC32 of the empty body) |
| SDK GetObject ChecksumMode ENABLED | e3b0c442... (sha256 of empty) | none | none | none | x-amz-checksum-mode ENABLED |
| CLI s3api put-object --body file | hex sha256 of body | none | Content-Length 25 | none | x-amz-sdk-checksum-algorithm CRC64NVME, x-amz-checksum-crc64nvme header |
| CLI s3 cp file | STREAMING-UNSIGNED-PAYLOAD-TRAILER | aws-chunked | Content-Length 75 (framed), x-amz-decoded-content-length 25 | x-amz-checksum-crc64nvme | no x-amz-sdk-checksum-algorithm header; checksum value in trailer |
| CLI s3 cp - (stdin) | hex sha256 of body | none | Content-Length 25 | none | x-amz-sdk-checksum-algorithm CRC64NVME, x-amz-checksum-crc64nvme header |
| CLI s3 presign + curl GET | none (query auth, X-Amz-SignedHeaders=host) | none | none | none | none |

Observed aws-chunked framing (unsigned trailer, single data chunk):

```
19\r\nhello faux3 wire capture\n\r\n0\r\nx-amz-checksum-crc32:X9wasw==\r\n\r\n
```

No client sent STREAMING-AWS4-HMAC-SHA256-PAYLOAD or the signed-trailer mode
over plain http. No client sent Content-MD5.

## Amendments to Phase 3 (implementation detail only)

1. The unsigned-trailer mode arrives in two shapes: with `Transfer-Encoding:
   chunked` and no Content-Length (SDK stream) and with a framed
   Content-Length (CLI `s3 cp`). The chunked reader must accept both and use
   `x-amz-decoded-content-length` for the size check.
2. The trailer may arrive without `x-amz-sdk-checksum-algorithm` (CLI). Take
   the algorithm from the `x-amz-trailer` header name.
3. CLI defaults to CRC64NVME, the SDK to CRC32; both must verify from day one.
4. A default presigned PUT hoists `x-amz-checksum-crc32` of the empty body
   into the query. Verifying it against the uploaded bytes fails for any
   non-empty body, as real S3 does; tests and docs must presign with
   `requestChecksumCalculation: "WHEN_REQUIRED"` (already in the presign
   test plan).
5. The signed streaming modes are still implemented per the spec (other SDKs
   and https endpoints use them) and are covered by the AWS docs vectors
   rather than by this capture.
