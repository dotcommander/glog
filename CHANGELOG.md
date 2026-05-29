# Changelog

## v0.4.0 (2026-05-29)

### Features

- add CLI config fallback coverage and bounded HTTP error-body handling

### Improvements

- split log HTTP handlers into focused read/write/filter helpers
- validate export level filters and reject unknown JSON request fields
- cap JSON request bodies and stdin log message reads
- add dashboard typecheck script and favicon/static asset tracking
- add dashboard `just` recipe and refresh maintenance notes

## v0.3.0 (2026-05-15)

### Features

- add real-time SSE tail command
- add log fingerprinting, grouped log summaries, and statistics endpoints
- add agent inbox API, CLI, dashboard view, and persistence
- add install doctor, database inspect, and first-run diagnostics
- add macOS launchd service recipes with `Justfile`

### Improvements

- improve responsive dashboard layout and host/filter polish
- improve installer guidance for first-time users
- refresh README positioning, dashboard screenshot, and README hero art
- document getting started, deployment, inbox usage, architecture, and invariants
