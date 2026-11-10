# 0002. Go, standard library only

- Status: Accepted
- Diagram: docs/diagrams/architecture.svg

## Context

Faux3 must ship as one small container that starts instantly and is easy to audit. The server needs HTTP, XML, hashing, and filesystem access.

## Decision

Write the server in Go (1.26, the newest GA release at build time) using only the standard library. `go.mod` has no third-party requirements. The binary is built with `CGO_ENABLED=0` as a static executable.

## Consequences

- No dependency updates to chase for the server, and a small attack surface.
- CRC64NVME, the aws-chunked decoder, and SigV4 are implemented in-repo and tested against AWS documentation vectors and real clients.
- Go toolchain commands run in containers through `scripts/go.sh`; no host Go install.

## Alternatives

- TypeScript on Node: larger image, slower start, a dependency tree.
- Rust: strong fit, but slower iteration for the maintainers.
- Go with the AWS SDK for signing: pulls a large module graph for a small job.
