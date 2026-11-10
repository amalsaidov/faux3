# Configuration

Faux3 reads `FAUX3_*` environment variables at startup. An invalid value stops startup with a message naming the variable. Secrets are never logged.

| Variable | Default | Meaning |
| --- | --- | --- |
| `FAUX3_ACCESS_KEY_ID` | `faux3` | Static access key |
| `FAUX3_SECRET_ACCESS_KEY` | `faux3` | Static secret key |
| `FAUX3_REGION` | `us-east-1` | Region that must appear in the SigV4 credential scope |
| `FAUX3_DATA_DIR` | `/data` | Storage root |
| `FAUX3_PORT` | `9000` | Listen port (listens on 0.0.0.0) |
| `FAUX3_CORS_ORIGINS` | unset | Comma list of origins, or `*`. Unset disables CORS entirely |
| `FAUX3_FSYNC` | `true` | fsync data, metadata, and directories on every write |
| `FAUX3_MAX_OBJECT_SIZE` | `5368709120` | Largest object in bytes; larger uploads fail with EntityTooLarge |
| `FAUX3_CLOCK_SKEW` | `15m` | Allowed request time skew, as a Go duration |
| `FAUX3_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`; JSON logs to stdout |

The default credentials are public knowledge. Change them if anything else can reach the port, and read [SECURITY.md](../SECURITY.md).

## Data directory and bind mounts

The image runs as the distroless `nonroot` user, uid 65532. A named volume (`-v faux3-data:/data`) works as is. A bind mount of a host directory is usually owned by your host user, and faux3 then exits at startup with a message naming uid 65532. Fix it one of two ways:

- Run the container as your user: `docker run --user "$(id -u):$(id -g)" ...`, or in Compose:

  ```yaml
  services:
    faux3:
      image: ghcr.io/change-fabric/faux3:0.1.0
      user: "${UID}:${GID}"
      volumes: ["./data:/data"]
  ```

  Export `UID` and `GID` in your shell first, since most shells do not export them.
- Or give the directory to uid 65532: `sudo chown -R 65532:65532 ./data`.

## CORS and browser uploads

Faux3 sends no CORS headers unless `FAUX3_CORS_ORIGINS` is set, and answers preflight requests with 403 until it is. To let a page served from `http://localhost:5173` upload with a presigned PUT URL:

```yaml
environment:
  FAUX3_CORS_ORIGINS: http://localhost:5173
```

Recent AWS SDK for JavaScript v3 presigners hoist a CRC32 checksum of an empty body into the URL. The upload then fails with BadDigest, exactly as on real S3. Create the presigning client with `requestChecksumCalculation: "WHEN_REQUIRED"`:

```js
const s3 = new S3Client({
  endpoint: "http://localhost:9000",
  region: "us-east-1",
  forcePathStyle: true,
  credentials: { accessKeyId: "faux3", secretAccessKey: "faux3" },
  requestChecksumCalculation: "WHEN_REQUIRED",
});
const url = await getSignedUrl(s3, new PutObjectCommand({ Bucket: "demo", Key: "upload.bin" }), { expiresIn: 900 });
```

A working page is in [examples/js/browser-upload.html](../examples/js/browser-upload.html).

## Health check

`GET /-/healthz` answers `ok` without authentication. The image's Docker HEALTHCHECK runs `/faux3 healthcheck`, since the distroless image has no shell or curl. In Compose:

```yaml
healthcheck:
  test: ["CMD", "/faux3", "healthcheck"]
  interval: 2s
  retries: 30
```
