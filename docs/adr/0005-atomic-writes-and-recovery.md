# 0005. Atomic writes and startup recovery

- Status: Accepted
- Diagram: docs/diagrams/put-object-sequence.svg

## Context

Concurrent readers must never see a torn object, a failed upload must leave the old object intact, and data must survive a container restart or kill.

## Decision

PutObject streams the body into a new directory under `.faux3/tmp/` (same filesystem), hashing as it goes, with no full-body buffering. After size and checksum checks pass, it fsyncs the data (when `FAUX3_FSYNC=true`, the default), writes and fsyncs `meta.json`, then under a striped per-key write lock renames the old key directory aside, renames the new one into place, fsyncs the parent, and updates the index. The old directory is removed after the lock is released. Readers open the data file under a read lock and stream from the open descriptor, which survives a later rename.

At startup, faux3 empties `.faux3/tmp/`, removes key directories missing `meta.json`, and skips (with a warning) any `meta.json` whose key hashes elsewhere.

## Consequences

- Readers see the old or the new object, never a mix.
- Any failed upload leaves no trace once recovery runs.
- fsync costs throughput; `FAUX3_FSYNC=false` trades durability for speed.

## Alternatives

- Write in place: torn reads and partial objects after a crash.
- A write-ahead log: more code than a dev tool needs.
