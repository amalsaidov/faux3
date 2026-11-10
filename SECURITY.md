# Security policy

## Scope and posture

Faux3 is a local development tool. Never expose it to untrusted networks or the internet.

- The default credentials (`faux3` / `faux3`) are public.
- Faux3 serves plain http only; there is no TLS.
- There is one static credential and no authorization model beyond it.
- Data is stored unencrypted in the data volume.

The documented examples bind the port to localhost (`-p 127.0.0.1:9000:9000`); keep it that way, because the default credentials are public. Compose services reach Faux3 by service name (`http://faux3:9000`) without a published port.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub private vulnerability reporting: the Security tab of https://github.com/change-fabric/faux3, then "Report a vulnerability". Do not open a public issue.

Useful reports include a SigV4 verification bypass, path traversal out of the data directory, or a crash triggered by a request.

## Supported versions

Only the latest 0.x release receives fixes.
