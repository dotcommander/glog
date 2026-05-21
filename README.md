<p align="center">
  <img src="docs/glogo-readme-wide.webp" alt="GLog" width="100%">
</p>

<p align="center">
  <a href="https://github.com/dotcommander/glog/releases"><img src="https://img.shields.io/github/v/release/dotcommander/glog?style=flat" alt="Release"></a>
  <a href="https://github.com/dotcommander/glog/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/dotcommander/glog/release.yml?style=flat&label=release%20build" alt="Build"></a>
  <a href="https://github.com/dotcommander/glog/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/dotcommander/glog?style=flat" alt="Go Version"></a>
  <a href="https://github.com/dotcommander/glog/blob/main/LICENSE"><img src="https://img.shields.io/github/license/dotcommander/glog?style=flat" alt="License"></a>
</p>

---

You could spin up Elasticsearch, Logstash, and Kibana. Configure a Loki cluster. Pay Datadog per gigabyte. Set up Fluentd with seventeen plugins.

Or: run one binary. Ship logs. Debug with a live workspace.

GLog is a local-first logging and debugging workspace that runs on SQLite and ships as a single binary. It gives small teams and coding agents a live dashboard, structured log search, rollups, repeated-pattern grouping, exports, and an inbox for notes, tasks, bugs, questions, and handoffs.

![GLog dashboard](docs/readme-dashboard.png)

## 30-second demo

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/dotcommander/glog/main/scripts/install.sh | sh

# Verify the install and see exact next steps
glog doctor

# Start the server (database creates itself)
glog serve

# In another terminal: register a host, get an API key
glog host register --server http://localhost:6016 --name my-server
# Writes config to ./glog.json — no manual setup

# Ship a log
glog log "Deployment complete" --level info --field version=2.1.0

# Pipe anything
tail -f /var/log/syslog | glog log --stream --level info
```

Server opens at `http://localhost:6016`. Dashboard and API, same port.

## Install

**Binary** (recommended):

```bash
curl -fsSL https://raw.githubusercontent.com/dotcommander/glog/main/scripts/install.sh | sh
glog doctor
```

The installer downloads the right macOS/Linux archive, installs `glog` into `/usr/local/bin` or `~/.local/bin`, warns if that directory is not on `PATH`, and prints the first server/register/log commands.

You can also grab archives manually from the [releases page](https://github.com/dotcommander/glog/releases). macOS, Linux, and Windows builds are published for amd64 and arm64.

**Go install**:

```bash
go install github.com/dotcommander/glog/cmd/glog@latest
```

**From source**:

```bash
git clone https://github.com/dotcommander/glog && cd glog
make          # Builds frontend + backend, starts on :6016
make install  # Puts glog in ~/go/bin
```

## CLI

```bash
glog serve                              # Start server (:6016 by default)
glog serve --addr :8080 --db logs.db    # Custom port and database path
glog serve --durability strict          # SQLite sync profile: strict, balanced, fast

glog host register --name prod-api      # Register host, saves config to ./glog.json
glog host list                          # Show registered hosts

glog log "Something happened"           # Send a log (level: info)
glog log --level error "Disk full"      # Explicit level
glog log --field region=us-east-1 "OK"  # Attach structured fields

journalctl -f | glog log --stream       # Stream stdin, batched at 100 lines
cat app.log | glog log --level warn     # Pipe a file

glog migrate --db glog.db               # Run database migrations
glog inspect db --db glog.db --json     # Offline database diagnostics
glog doctor                             # Check PATH, config, server, and next steps
glog inbox add "Follow up on flaky test" # Leave an agent note/task
glog inbox list                         # Show open agent inbox items
glog inbox done 123                     # Complete an inbox item
glog version                            # Print version and build time
```

Configuration resolves in order: flags, then `GLOG_SERVER`/`GLOG_API_KEY` env vars, then `./glog.json`.

## For LLM coding agents

Use GLog as your local debugging memory when a task spans commands, browser checks, or multiple agent sessions.

```bash
# 1. Find or start the local server.
glog doctor
glog serve

# 2. Register the current workspace once, then reuse ./glog.json.
glog host register --server http://localhost:6016 --name "$(basename "$PWD")"

# 3. Send useful evidence while debugging.
glog log --level info "starting frontend smoke test" \
  --field repo="$(basename "$PWD")" \
  --field command="bun run build"

# 4. Leave durable follow-up work in the agent inbox.
glog inbox add "Recheck SSE reconnect after hub change" \
  --kind task \
  --priority normal \
  --repo "$(basename "$PWD")" \
  --session "$USER-$(date +%Y%m%d-%H%M%S)" \
  --tag sse \
  --body "Evidence belongs in logs. This item is the next action."

# 5. Before finishing, review open work and close completed items.
glog inbox list --status open --repo "$(basename "$PWD")"
glog inbox done 123
```

Agent rules of thumb:

- Put runtime evidence, command output summaries, errors, and browser observations in logs.
- Put decisions, blocked questions, next tasks, bugs, and handoffs in the inbox.
- Include `repo`, `session`, and focused tags so future agents can filter quickly.
- Link the exact error text, fingerprint, or log ID in the inbox body when a task depends on evidence.
- Mark inbox items done when the task is handled; leave blocked questions open with `--kind question --priority high`.

Full reference: [Agent inbox](docs/api/inbox.md).

## API

### Hosts

| Method | Path | Auth | |
|--------|------|------|-|
| POST | `/api/v1/hosts` | - | Register host, returns API key |
| GET | `/api/v1/hosts` | - | List all hosts |
| GET | `/api/v1/hosts/{id}` | - | Get host details |
| GET | `/api/v1/hosts/{id}/stats` | - | Host log statistics |

### Logs

| Method | Path | Auth | |
|--------|------|------|-|
| POST | `/api/v1/logs` | Bearer | Create log entry |
| POST | `/api/v1/logs/bulk` | Bearer | Bulk create (up to 1000) |
| GET | `/api/v1/logs` | - | Query with filters |
| GET | `/api/v1/logs/{id}` | - | Get single log |
| GET | `/api/v1/stats/logs` | - | Hourly log count buckets |
| GET | `/api/v1/stats/fingerprints` | - | Hourly fingerprint buckets |

### Agent Inbox

| Method | Path | Auth | |
|--------|------|------|-|
| GET | `/api/v1/inbox` | - | List notes/tasks/questions/handoffs |
| POST | `/api/v1/inbox` | - | Create inbox item |
| GET | `/api/v1/inbox/{id}` | - | Get inbox item |
| PATCH | `/api/v1/inbox/{id}` | - | Update inbox item |
| DELETE | `/api/v1/inbox/{id}` | - | Delete inbox item |
| POST | `/api/v1/inbox/{id}/done` | - | Mark inbox item done |

### Stream & Export

| Method | Path | Auth | |
|--------|------|------|-|
| GET | `/api/v1/events` | - | SSE live stream |
| GET | `/api/v1/export/{json,csv,ndjson}` | Bearer | Export logs |
| GET | `/health` | - | Health check |

Auth means `Authorization: Bearer glog_v1_<key>` — the key you get from `host register`.

```bash
# Create a log via curl
curl -X POST http://localhost:6016/api/v1/logs \
  -H "Authorization: Bearer glog_v1_<key>" \
  -H "Content-Type: application/json" \
  -d '{"level":"error","message":"Payment failed","fields":{"order_id":"abc123"}}'

# Stream live events
curl -N http://localhost:6016/api/v1/events
```

## Under the hood

Go backend. Svelte 5 frontend. SQLite in WAL mode. Everything runs from one binary.

```
┌─────────────────────────────────────────────────┐
│  CLI (cobra)          Browser (Svelte 5)        │
│  glog log/host/serve  localhost:6016            │
└──────────┬────────────────────┬─────────────────┘
           │                    │
     ┌─────▼────────────────────▼─────┐
     │       HTTP / SSE (chi/v5)      │
     │  handlers → repos (no service  │
     │  layer, no ceremony)           │
     └──────────────┬─────────────────┘
                    │
          ┌─────────▼─────────┐
          │   SQLite (WAL)    │
          │   single file     │
          │   embedded migrations │
          └───────────────────┘
```

Handlers call repositories directly. The domain layer defines entities and interfaces — no infrastructure imports. Swapping SQLite for PostgreSQL means writing new repository implementations. Nothing else changes.

```
cmd/glog/           CLI and server entry point
internal/
  domain/           Entities, repository interfaces, pattern matching
  infrastructure/   HTTP handlers, SQLite repos, SSE hub
web/                Svelte 5 frontend (built into web/build/)
```

## Philosophy

Small tools should stay understandable. Your logs belong on your hardware. The database is a single file you can `cp` to back up. The server is a single binary you can `scp` to deploy.

GLog is intentionally modest infrastructure: start the binary, point your apps or coding agents at it, inspect the evidence, and leave durable follow-up work in the inbox when the debugging thread needs to continue.

## Why not X?

| Tool | What you get | What it costs |
|------|-------------|---------------|
| ELK Stack | Full-text search, Kibana | JVM heap tuning, three services, YAML therapy |
| Loki + Grafana | Label-based queries | Promtail sidecar, chunk storage, LogQL |
| Datadog / Splunk | Everything, managed | Per-GB pricing, vendor lock-in |
| Papertrail | Hosted tailing, simple search | Monthly bill, data leaves your network |
| **GLog** | Live dashboard, rollups, exports, agent inbox | One binary, SQLite, local-first |

GLog is for teams and coding agents that need operational evidence without a logging platform project. If you need petabyte-scale search across a fleet of thousands, use the big tools. If you want a local, inspectable place to capture logs, spot patterns, and hand off debugging state, this is the one.

## Docs

- [Go integration](docs/go-integration.md) — embed glog in your Go services
- [Getting started](docs/getting-started.md) — first install, first server, first log
- [Deployment](docs/guides/deployment.md) — launchd, Docker, systemd, nginx
- [SSE events](docs/api/events.md) — real-time event format
- [Agent inbox](docs/api/inbox.md) — notes, tasks, questions, and handoffs for coding agents
- [Architecture](docs/architecture/index.md) — design decisions
- [Invariants](docs/architecture/invariants.md) — behavior future changes must preserve
- [Contributing](docs/contributing.md) — commit format, test requirements

## License

MIT. Because logs shouldn't require permission.
