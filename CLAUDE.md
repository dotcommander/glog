# GLog - Claude's Internal Documentation

Development guide for GLog maintainers. Multi-host log utility with Clean Architecture.

## Project Overview

**GLog** is a minimalist multi-host log collection and dashboard system inspired by Papertrail. Built with Go backend and Svelte 5 frontend.

### Core Philosophy
- **Minimal dependencies**: Chi, SQLite, Svelte 5
- **Two-layer architecture**: Domain → Infrastructure (handlers call repos directly)
- **SQLite-first**: WAL mode + retry logic for concurrency
- **Real-time**: Server-Sent Events (SSE) for live updates

## Architecture

### Project Structure

```
internal/
├── constants/        # Shared constants (timeouts, sizes)
├── domain/           # Business logic, entities, value objects
│   ├── entities/     # Host, Log, JSONMap
│   ├── ports/        # Repository interfaces
│   ├── services/     # Pattern matching, metadata derivation
│   └── valueobjects/ # Severity levels
└── infrastructure/   # External concerns
    ├── http/         # Handlers, routes, middleware
    │   ├── handlers/ # Request handlers (call repos directly)
    │   └── middleware/
    ├── persistence/  # SQLite repositories
    ├── sse/          # Server-Sent Events hub
    └── logging/      # Self-logging for GLog errors
web/                  # Svelte 5 frontend (built to web/build/)
```

### Key Design Decisions

**Why SQLite?**
- Single-file deployment
- WAL mode gives 10-100x concurrent read performance
- Retry logic handles transient "database is locked" errors
- Easy backup (just copy the .db file)
- See SQLite WAL mode documentation for patterns

**Why Two-Layer Architecture?**
- Simple codebase: handlers call repositories directly
- Domain contains entities/interfaces, infrastructure implements them
- Repository interfaces in `domain/ports/` allow swapping backends (PostgreSQL later)

**API Key Format**
```
glog_v1_<46-char-hex>
Total: 54 characters
Example: glog_v1_b7cfad39c28529d5124da53fcb34493d132d706a82f2fa
```

## Database Configuration

### SQLite Concurrency Optimizations

**WAL Mode (Persistent)**
```sql
PRAGMA journal_mode=WAL;        -- Readers don't block writers
```

**Connection Pooling (Single Writer)**
```go
db.SetMaxOpenConns(1)   // SQLite works best with single writer
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(time.Hour)
```

**Busy Timeout**
```sql
PRAGMA busy_timeout = 5000;     -- Retry for 5 seconds on lock
```

**Performance Tuning**
```sql
PRAGMA synchronous = NORMAL;    -- Faster writes with WAL
PRAGMA cache_size = -64000;     -- 64MB cache (set by migration 003)
PRAGMA temp_store = MEMORY;     -- Temp tables in RAM
PRAGMA foreign_keys = ON;       -- Enforce FK constraints
```

See `internal/infrastructure/persistence/sqlite/database.go` for full configuration.

### Retry Logic Pattern

All repository methods that write use exponential backoff:

```go
const maxRetries = 3
for i := 0; i < maxRetries; i++ {
    err := // ... db operation
    if err == nil {
        return result, nil
    }
    if i < maxRetries-1 && isTransientError(err) {
        backoff := time.Millisecond * 10 * (1 << i) // 10ms, 20ms, 40ms
        time.Sleep(backoff)
        continue
    }
    return nil, err
}
```

**Transient Errors:**
- `"locked"`
- `"busy"`
- `"SQLITE_BUSY"`

## API Endpoints

### Authentication

Write operations require Bearer token authentication. Read operations are public:

```bash
curl -H "Authorization: Bearer glog_v1_<your-api-key>" \
     -H "Content-Type: application/json" \
     http://localhost:6016/api/v1/logs
```

### Host Management

**Register Host (Public)**
```bash
POST /api/v1/hosts
Content-Type: application/json

{
  "name": "production-api",
  "tags": ["prod", "api"],
  "description": "Production API server"
}

Response 201:
{
  "id": 1,
  "name": "production-api",
  "api_key": "glog_v1_b7cfad39c28529d5124da53fcb34493d132d706a82f2fa",
  "tags": ["prod", "api"],
  "status": "unknown",
  "created_at": "2026-01-06T19:20:00Z",
  "last_seen": "2026-01-06T19:20:00Z",
  "log_count": 0,
  "error_count": 0,
  "error_rate": 0
}
```

**List Hosts (Public)**
```bash
GET /api/v1/hosts

Response 200:
{
  "hosts": [...],
  "total": 10
}
```

**Get Host (Public)**
```bash
GET /api/v1/hosts/{id}
```

**Get Host Stats (Public)**
```bash
GET /api/v1/hosts/{id}/stats
```

### Log Operations

**Create Log (Authenticated)**
```bash
POST /api/v1/logs
Authorization: Bearer glog_v1_<api-key>
Content-Type: application/json

{
  "level": "error",
  "message": "Database connection failed",
  "fields": {
    "error": "timeout",
    "retry_count": 3
  }
}
```

**Query Logs (Public)**
```bash
GET /api/v1/logs?limit=100&offset=0&level=error&host_id=1
```

**Bulk Create Logs (Authenticated)**
```bash
POST /api/v1/logs/bulk
Authorization: Bearer glog_v1_<api-key>

{
  "logs": [
    {
      "level": "info",
      "message": "First log"
    },
    {
      "level": "error",
      "message": "Second log",
      "fields": {"code": 500}
    }
  ]
}
```

### Real-time Updates

**SSE Stream (Public)**
```bash
GET /api/v1/events

# Streams events:
event: log.created
data: {"id": 123, "level": "error", ...}

event: host.registered
data: {"id": 5, "name": "new-host", ...}
```

### Export Endpoints (Authenticated)

```bash
GET /api/v1/export/json     # Export logs as JSON
GET /api/v1/export/csv      # Export logs as CSV
GET /api/v1/export/ndjson   # Export logs as newline-delimited JSON
Authorization: Bearer glog_v1_<api-key>
```

### Health & Info

```bash
GET /health            # Health check
GET /                  # API info (or Svelte frontend if --web flag)
GET /api/v1/logs/{id}  # Get specific log
```

## CLI Usage

### Installation

```bash
go install github.com/dotcommander/glog/cmd/glog@latest
```

Or build from source:

```bash
git clone https://github.com/dotcommander/glog
cd glog
make build  # Creates bin/glog
```

### Configuration

GLog CLI looks for config in this order:
1. Command line flags
2. Environment variables (`GLOG_SERVER`, `GLOG_API_KEY`)
3. Config file (default: `./glog.json`)

**Config file format:**
```json
{
  "server": "https://logs.example.com",
  "api_key": "glog_v1_...",
  "host_id": 1,
  "host_name": "my-host"
}
```

### Commands

**Register Host**
```bash
# Interactive (prompts for details)
glog host register

# Or with flags
glog host register \
  --name "production-api" \
  --tag prod \
  --tag api \
  --server https://logs.example.com \
  --config ./glog.json

# Output includes API key to save
```

**Send Logs**

```bash
# Send single log
glog log "User login successful" --level info

# With fields
glog log "Payment processed" \
  --level info \
  --field amount=99.99 \
  --field currency=USD \
  --field user_id=12345

# From stdin
echo "Error from cron job" | glog log --level error

# Pipe from another process
my-application 2>&1 | glog log --level error

# Multiple logs (read from file)
glog log --level debug < application.log
```

**List Hosts**
```bash
glog host list --server https://logs.example.com
```

**Check CLI Config**
```bash
glog --config ./glog.json info
```

## Development

### Prerequisites

- Go 1.21+
- SQLite 3.x (pure Go driver: `modernc.org/sqlite`)
- Make (optional, for Makefile targets)

### Setup

```bash
git clone https://github.com/dotcommander/glog
cd glog
go mod init github.com/dotcommander/glog
go mod tidy
```

### Running Tests

```bash
# Unit tests
go test ./internal/domain/...
go test ./internal/infrastructure/...

# All tests
make test
```

### Database Migrations

Migration files in:
```
internal/infrastructure/persistence/sqlite/migrations/
├── 001_create_hosts_table.sql
├── 002_create_logs_table.sql
└── 003_add_performance_indexes.sql
```

**Apply migrations:**
```bash
# Manual
sqlite3 test.db < migrations/001_create_hosts_table.sql

# Or use CLI
glog migrate --db ./test.db
```

### Debug Logging

Enable debug logging for SQLite operations:

```bash
export GLOG_LOG_LEVEL=debug
./bin/test
```

Logs to stdout:
```
[GLOG] [2026-01-06 22:30:04] SQLite retry: attempt=1, backoff=10ms, error=database locked
[GLOG] [2026-01-06 22:30:04] SQLite retry: attempt=2, backoff=20ms, success=true
```

## Self-Logging

GLog logs its own errors to help debug issues:

```go
logger := logging.NewGLogLogger(hostID, apiKey, serverURL)

// Log SQLite errors
if err != nil && isTransientError(err) {
    logger.LogSQLError("FindByAPIKey", query, err)
    // Output: [GLOG] 2026-01-06 22:30:04 | database_locked | FindByAPIKey | retryable=true
}

// Log rate limiting
logger.LogRateLimitExceeded(hostID, apiKey, "100 req/s")
```

See `internal/infrastructure/logging/glog_logger.go` for full implementation.

## Performance Tuning

### For High Volume (1000+ logs/sec)

1. **Enable WAL Mode** ✅ (already done)
2. **Single Writer Connection** ✅ (already configured)
3. **Batch Inserts**: Use `/api/v1/logs/bulk` instead of individual requests
4. **Large Cache**: Already set to 64MB via migration 003
5. **Consider PostgreSQL**: See migration path below

### Connection Pool Metrics

Monitor these in production:

```go
// In handlers/health.go
stats := db.Stats()
health := map[string]interface{}{
    "max_open_connections": stats.MaxOpenConnections,
    "open_connections":     stats.OpenConnections,
    "in_use":              stats.InUse,
    "idle":                stats.Idle,
    "wait_count":          stats.WaitCount,
    "wait_duration":       stats.WaitDuration,
}
```

## PostgreSQL Migration Path

For true multi-host deployments (>100 hosts, >10k logs/sec):

### Why PostgreSQL?
- Handles concurrent writers natively
- Better transaction isolation
- Connection pooling (PgBouncer)
- Full-text search (GIN indexes)
- Partitioning for large datasets

### Migration Steps

1. **Add PostgreSQL Repository**
   ```
   internal/infrastructure/persistence/postgresql/
   ├── database.go
   ├── host_repository.go  # Implements same interface
   └── log_repository.go
   ```

2. **Repository Interface**
   ```go
   type HostRepository interface {
       Create(host *entities.Host) error
       FindByAPIKey(apiKey string) (*entities.Host, error)
       // ... other methods
   }
   ```

3. **Factory Pattern**
   ```go
   func NewRepository(dbType string, conn string) (HostRepository, error) {
       switch dbType {
       case "sqlite":
           return sqlite.NewHostRepository(conn)
       case "postgresql":
           return postgresql.NewHostRepository(conn)
       default:
           return nil, fmt.Errorf("unknown db type: %s", dbType)
       }
   }
   ```

4. **Configuration**
   ```json
   {
     "database": {
       "type": "postgresql",
       "connection": "postgres://user:pass@host/db?sslmode=require"
     }
   }
   ```

## Troubleshooting

### "database is locked"

**Cause**: Multiple writers competing for lock (rare with our config)

**Fix**: 
- Retry logic already implemented ✅
- Check WAL mode enabled: `sqlite3 test.db "PRAGMA journal_mode;"`
- Verify busy_timeout: `sqlite3 test.db "PRAGMA busy_timeout;"` (should be 5000)

### "Invalid API key"

**Cause**: Usually metadata NULL issue or WAL mode not active

**Fix**:
```bash
# Clean database
mv test.db /tmp/backup.db
./bin/test  # Start fresh with correct schema
```

### "failed to unmarshal JSONMap value"

**Cause**: Metadata column NULL instead of '{}'

**Fix**: Database needs clean migration with `DEFAULT '{}'`

### WAL Files Not Appearing

```bash
ls -la test.db*  # Should see .db-shm and .db-wal

# If not present, WAL wasn't enabled properly
sqlite3 test.db "PRAGMA journal_mode=WAL;"
# Returns: "wal"
```

## Contributing

### Commit Message Format

```
type(scope): description

type: feat, fix, docs, test, refactor, chore
scope: http, sqlite, cli, api, domain

Examples:
- feat(http): add bulk log endpoint
- fix(sqlite): handle database locked errors
- docs: update API endpoint documentation
```

### Testing Requirements

- Unit tests for domain layer
- Integration tests for HTTP handlers
- Test concurrent operations (10+ simultaneous requests)
- Verify SQLite WAL mode enabled in tests

### Code Style

- No global variables
- Dependency injection for repositories
- Context-aware operations
- Structured logging (not fmt.Printf in production)

## Deployment

### Single Host (SQLite)

```bash
# Build
make build

# Run server
./bin/glog serve --db ./glog.db --addr :6016

# With web frontend
./bin/glog serve --db ./glog.db --web ./web/build

# Or with Docker
docker run -v $(pwd)/data:/data -p 6016:6016 glog:latest
```

### Multi-Host (PostgreSQL)

```bash
# Environment variables
export GLOG_DB_TYPE=postgresql
export GLOG_DB_CONN=postgres://user:pass@host/glog
export GLOG_PORT=6016

./bin/glog serve
```

### Docker Compose

```yaml
version: '3.8'
services:
  glog:
    image: glog:latest
    ports:
      - "6016:6016"
    volumes:
      - ./data:/data
    environment:
      - GLOG_DB_PATH=/data/glog.db
      - GLOG_PORT=6016
    restart: unless-stopped
```

## Monitoring

### Health Endpoints

```bash
# Basic health
curl http://localhost:6016/health

# With database stats
curl http://localhost:6016/health?detailed=true

# Expected response:
{
  "status": "healthy",
  "timestamp": "2026-01-06T22:30:00Z",
  "database": {
    "path": "./test.db",
    "size": 45056,
    "status": "ok"
  },
  "sse": {
    "clients": 0
  }
}
```

### Prometheus Metrics (Future)

Planned metrics:
- `glog_logs_total{level="error",host_id="1"}`
- `glog_api_requests_total{endpoint="/api/v1/logs",status="200"}`
- `glog_database_errors_total{error_type="locked"}`
- `glog_sse_clients_connected`

## License

MIT License - see LICENSE file for details

## Support

- GitHub Issues: https://github.com/dotcommander/glog/issues
- Documentation: https://github.com/dotcommander/glog/wiki
- API Reference: http://localhost:6016/api/v1 (when running)

---

**Last Updated**: 2026-01-07
**Version**: 1.0.0
**Status**: Core API Complete, Svelte 5 Frontend Complete
