# 0009. Releases to GHCR and 0.x versioning

- Status: Accepted
- Diagram: none

## Context

Faux3 is pre-alpha. The maintainer requires that the project start at v0.1.0 and never be promoted to v1 unless the maintainer intentionally chooses that.

## Decision

- Publish multi-arch images to `ghcr.io/change-fabric/faux3` only, from `.github/workflows/release.yml` on `vX.Y.Z` tags using `GITHUB_TOKEN`. Docker Hub is future work.
- The first release is v0.1.0. Versions stay 0.x; a breaking change bumps the minor version.
- Image tags: `X.Y.Z`, `X.Y`, and `latest`. No bare major tag (it would be `0`).
- The release workflow has a guard step that fails unless the tag matches `^v0\.[0-9]+\.[0-9]+$`. No tool, bot, or agent may produce v1.0.0. Only the maintainer may change this ADR and the guard.
- CHANGELOG.md follows Keep a Changelog, maintained by hand; the release notes for a tag are extracted from it.
- The first GHCR package is private by default and must be made public once in the package settings.

## Consequences

Users pin a 0.x minor and expect breaking changes between minors. Dependabot and contributors cannot cut a v1 by accident.

## Alternatives

- Docker Hub as well: deferred, see ROADMAP.md.
- Start at v1.0.0: rejected by the maintainer.
- Automated changelog and version bumps: risk producing v1.
