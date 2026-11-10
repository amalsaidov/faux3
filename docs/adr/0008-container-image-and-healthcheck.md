# 0008. Container image and health check

- Status: Accepted
- Diagram: docs/diagrams/architecture.svg

## Context

The image should be small, run without root, work on Intel and Apple silicon, and report health to Compose without a shell.

## Decision

- Multi-stage build: a `golang` stage on `$BUILDPLATFORM` cross-compiles for `$TARGETOS/$TARGETARCH` (no QEMU), and the result is copied into `gcr.io/distroless/static-debian12:nonroot`.
- Runs as uid 65532. `/data` is created owned by that user, so named volumes work unchanged. An unwritable data dir fails startup with a message naming uid 65532 and the `user:` fix.
- `GET /-/healthz` returns `ok`, and a `faux3 healthcheck` subcommand calls it for the Docker HEALTHCHECK, since distroless has no curl.
- Images are built for linux/amd64 and linux/arm64. Base images are pinned by digest.

## Consequences

Bind mounts owned by the host user need `user:` or a chown, documented in docs/configuration.md. Debugging inside the container needs a sidecar, since there is no shell.

## Alternatives

- Alpine base: adds a shell and package manager that faux3 does not need.
- scratch: no CA certs or passwd entries; distroless is as small and better maintained.
