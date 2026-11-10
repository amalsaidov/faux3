# JavaScript examples

Small scripts using the AWS SDK for JavaScript v3 against faux3.

- `client.mjs`: the S3 client settings faux3 needs (`endpoint`, `forcePathStyle: true`, region, static credentials).
- `upload.mjs`: create a bucket if missing, put, get, and list.
- `presign.mjs`: presigned PUT and GET URLs, exercised with `fetch`.
- `browser-upload.html`: upload from a browser to a presigned URL. Needs `FAUX3_CORS_ORIGINS`.

Environment: `S3_ENDPOINT` (default `http://localhost:9000`), `S3_BUCKET` (default `example`), `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` (default `faux3`), `AWS_REGION` (default `us-east-1`).

Run them in a container, no host Node needed. The easiest path is the Compose example:

```sh
docker compose -f examples/compose/compose.yaml up --build --abort-on-container-exit --exit-code-from app
docker compose -f examples/compose/compose.yaml down -v
```

Against a faux3 already listening on the host:

```sh
docker run --rm -v "$PWD/examples/js:/app" -w /app -e S3_ENDPOINT=http://host.docker.internal:9000 \
  node:22-alpine sh -c "npm ci && node upload.mjs && node presign.mjs"
```
