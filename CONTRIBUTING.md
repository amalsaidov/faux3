# Contributing to Faux3

Thanks for helping. Faux3 is pre-alpha; read [AGENTS.md](AGENTS.md) for the project rules, which apply to humans too.

## Finding work

- [ROADMAP.md](ROADMAP.md) lists the operations and features that may come next.
- Issues labeled `roadmap` track roadmap items. Most are also labeled `good first issue` and `help wanted`: they are well scoped and a good way in.
- Roadmap work starts with an ADR in `docs/adr/`. Open a PR with the ADR first, then the implementation once it is accepted.

## Development setup

You need Docker and nothing else. Do not install Go, Node, or the AWS CLI on your host for this project.

```sh
scripts/lint.sh          # gofmt, go vet, staticcheck
scripts/test.sh          # unit tests with -race
scripts/integration.sh   # AWS SDK for JavaScript v3 and AWS CLI v2 suites in Compose
docker build -t faux3:dev .
docker compose up --build   # run faux3 from source on port 9000
```

## Tests

- Every change keeps lint, unit tests, and integration tests green.
- Any change to S3 behavior needs both an SDK test (`test/integration/node`) and a CLI test (`test/integration/cli`).
- Every compatibility bug fix includes a regression test.

## Commits and changelog

- Small commits with conventional prefixes (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `refactor:`).
- Add a line to `## [Unreleased]` in CHANGELOG.md for every user-visible change.
- Repository prose is plain ASCII.
- DCO sign-off is not required.

## Versioning

Faux3 uses SemVer on 0.x. A breaking change bumps the minor version. Moving to v1 is a maintainer decision only; the release workflow rejects any tag that does not match `v0.Y.Z` (ADR 0009).

## Releasing (maintainers)

1. Move the Unreleased entries in CHANGELOG.md under `## [X.Y.Z] - YYYY-MM-DD`.
2. Commit, then tag: `git tag v0.Y.Z`.
3. Push the tag: `git push origin v0.Y.Z`. The release workflow builds linux/amd64 and linux/arm64 images and pushes them to `ghcr.io/change-fabric/faux3`.
4. First release only: GHCR creates the package as private. Make it public once in the package settings, then confirm an anonymous `docker pull` works.

## Code of conduct

Be kind and assume good intent.
