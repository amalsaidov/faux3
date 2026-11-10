# Faux3

One container. One volume. One S3 endpoint. No AWS required.

Faux3 is an MIT-licensed, single-container, S3-compatible object store for local Docker development. It speaks a strict subset of the S3 API over path-style addressing, validates SigV4 on every request, and keeps everything in one data volume.

Status: pre-alpha. Releases stay on 0.x; expect breaking changes between minor versions.
