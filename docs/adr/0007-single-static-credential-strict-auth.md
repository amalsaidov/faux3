# 0007. Single static credential, strict auth, and opt-in CORS

- Status: Accepted
- Diagram: docs/diagrams/architecture.svg

## Context

A local fake is tempting to run with auth off, but then code that signs incorrectly, uses the wrong region, or relies on CORS defaults passes locally and fails in AWS.

## Decision

- One static credential from `FAUX3_ACCESS_KEY_ID` and `FAUX3_SECRET_ACCESS_KEY` (both default `faux3`).
- Every request is authenticated. There is no bypass flag. Only `GET /-/healthz` and CORS preflight are unauthenticated.
- The credential scope region must equal `FAUX3_REGION`; a mismatch returns 400 AuthorizationHeaderMalformed.
- CORS is off unless `FAUX3_CORS_ORIGINS` is set to a comma list of origins or `*`. Preflight without it returns 403.
- Security posture: faux3 is a dev tool with public default credentials and plain http. It must not be exposed to untrusted networks.

## Consequences

Local behavior matches AWS on auth failures. Tests cover wrong secrets, tampered requests, and expired presigns. Browser examples must set the CORS variable explicitly.

## Alternatives

- `FAUX3_AUTH=off`: rejected; it hides signing bugs.
- Ignore the region: rejected for the same reason.
- Permissive CORS by default: rejected; it would mask missing CORS config.
