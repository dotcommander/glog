# GLog Invariants

These rules define behavior that future changes must preserve.

## Storage

- SQLite is the durable source of truth.
- WAL mode remains enabled for normal operation.
- Durability profiles may tune SQLite synchronization, but they must not replace SQLite durability with a custom storage engine.

## Logs

- Log timestamps may arrive out of order because clients can buffer, retry, or have skewed clocks.
- Raw logs are never rewritten for rollups; summary tables are derived acceleration data.
- Exports must support bounded-memory paths for large result sets.

## Hosts And Secrets

- API keys are returned only during host registration.
- Host list and host detail responses must not expose stored API keys.
- `_glog` is a reserved internal host name for server self-metrics and is hidden from default host lists.

## APIs

- New aggregate APIs must not change raw log list semantics.
- Internal telemetry is opt-in for list/filter views through an explicit `include_internal=true` style flag.

## Agent Inbox

- Inbox items are coordination state, not raw log evidence.
- Inbox status transitions must not mutate linked logs or fingerprints.
- Inbox items may link to logs or fingerprints, but raw logs remain authoritative for debugging.
- Inbox items should preserve enough repo/session/source metadata for agent handoff across sessions.

## Operations

- Offline inspection commands must not require a running server.
- Server health should expose enough SQLite and runtime state to diagnose durability and live-stream behavior.
