# 0010. Real-client compatibility testing in Compose

- Status: Accepted
- Diagram: docs/diagrams/architecture.svg

## Context

S3 compatibility is defined by what real clients send, which changes with SDK releases (for example default checksums in 2025). Unit tests against hand-built requests miss that. The host doctrine forbids installing go, node, or aws on the host.

## Decision

`compose.test.yaml` runs a faux3 service, a Node runner using the AWS SDK for JavaScript v3 (pinned versions, `node --test`), and an `amazon/aws-cli` runner, driven by `scripts/integration.sh`. The suite also covers restart persistence, SIGKILL recovery, concurrency, presigned URLs, CORS preflight with `FAUX3_CORS_ORIGINS` set, and a 1 GiB upload under a 128 MiB memory limit. Any S3 behavior change needs SDK and CLI tests, and every compatibility bug gets a regression test. Go unit tests and lint run in `golang` containers.

## Consequences

The integration suite is slower than unit tests but catches client-side changes. Dependabot updates to the SDK or CLI pins run the suite in CI.

## Alternatives

- Mock HTTP requests only: misses real client behavior.
- Run clients on the host: violates the host doctrine and varies by machine.
