# 0001. Record architecture decisions

- Status: Accepted
- Diagram: docs/diagrams/architecture.svg

## Context

Faux3 makes deliberate trade-offs (strict op set, strict auth, filesystem layout) that contributors and coding agents will be tempted to revisit. The reasons need to live next to the code.

## Decision

Record significant decisions as MADR-lite files in `docs/adr/`, numbered `NNNN-title.md`, each with Status, Context, Decision, Consequences, and Alternatives. Any new S3 operation, roadmap item, storage format change, or change to auth or release policy starts with a new ADR. Superseded ADRs stay in place with their status updated.

## Consequences

Changes that cross an ADR need a new ADR first. The index in [README.md](README.md) lists every record.

## Alternatives

- Decisions in issues or PR descriptions only: lost from the repository and invisible to offline agents.
- A single design document: grows stale and hides when each decision was made.
