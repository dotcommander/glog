# Architecture overview

GLog follows a **two-layer Clean Architecture** pattern that separates business logic from infrastructure concerns.

## Layer structure

```
┌─────────────────────────────────────────────────────────────────┐
│                         Domain Layer                             │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  entities/        - Business entities (Host, Log, JSONMap) │  │
│  │  ports/          - Repository interfaces (contracts)       │  │
│  │  services/       - Business logic (matching, derivation)   │  │
│  │  valueobjects/   - Value objects (Severity, levels)        │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ depends on
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Infrastructure Layer                          │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  http/           - Handlers, routes, middleware            │  │
│  │  persistence/    - SQLite repositories (implement ports)   │  │
│  │  sse/            - Server-Sent Events hub                  │  │
│  │  logging/        - Self-logging for GLog errors            │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Component diagram

```
                    ┌─────────────────┐
                    │   HTTP Client   │
                    └────────┬────────┘
                             │ HTTP/JSON
                             ▼
┌────────────────────────────────────────────────────────────────┐
│                         HTTP Layer                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │   Routes     │─▶│ Middleware   │─▶│     Handlers         │  │
│  │  (chi.Router)│  │  (auth, etc) │  │  (request/response)  │  │
│  └──────────────┘  └──────────────┘  └──────────┬───────────┘  │
└────────────────────────────────────────────────────┼────────────┘
                                                     │
                                                     │ calls directly
                                                     ▼
┌────────────────────────────────────────────────────────────────┐
│                      Repository Layer                           │
│  ┌──────────────────────┐  ┌──────────────────────────────┐   │
│  │   HostRepository     │  │    LogRepository             │   │
│  │  (SQLite impl)       │  │    (SQLite impl)             │   │
│  └──────────┬───────────┘  └──────────────┬───────────────┘   │
└─────────────┼─────────────────────────────┼───────────────────┘
              │                             │
              │ reads/writes                │ reads/writes
              ▼                             ▼
      ┌───────────────┐           ┌───────────────┐
      │ hosts table   │           │  logs table   │
      │  (SQLite WAL) │           │  (SQLite WAL) │
      └───────────────┘           └───────────────┘

┌────────────────────────────────────────────────────────────────┐
│                      Supporting Systems                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │  SSE Hub     │  │ Self-Logger  │  │  Pattern Matcher     │  │
│  │ (broadcast)  │  │ (GLog→GLog)  │  │  (metadata services) │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## Design decisions

### Why two-layer architecture?

**Decision**: Handlers call repositories directly without an intermediate service layer.

**Rationale**:
- Simple codebase doesn't need service orchestration
- Repository interfaces (in `domain/ports/`) allow swapping backends
- Business logic lives in `domain/services/` for reusable operations
- Reduces indirection for straightforward CRUD operations



### Why SQLite?

**Decision**: SQLite with WAL mode as the default database.

**Rationale**:
- Single-file deployment (no separate database server)
- WAL mode provides 10-100x concurrent read performance
- Sufficient for single-host deployments (< 100 hosts)
- Easy backup and migration path to PostgreSQL

**See**: [Database Design](database-design.md) for configuration and migration path

### Why Server-Sent Events (SSE)?

**Decision**: SSE for real-time log streaming instead of WebSockets.

**Rationale**:
- Unidirectional flow (server → client) matches log streaming use case
- Simpler implementation (no connection handshake)
- Automatic reconnection with browser-native support
- Lower resource overhead than persistent WebSocket connections



### Why clean architecture?

**Decision**: Domain layer contains no infrastructure dependencies.

**Rationale**:
- Entities and ports are testable without frameworks
- Easy to swap SQLite for PostgreSQL (interface-based)
- Business logic (pattern matching, metadata derivation) is reusable
- Clear dependency boundaries prevent coupling



## Data flow

### Log creation flow

```
Client POST /api/v1/logs
        │
        ▼
┌───────────────────┐
│  Auth Middleware  │─▶ Validate Bearer token
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐
│  Log Handler      │─▶ Parse JSON, validate level
└─────────┬─────────┘
          │
          ├──────────────┐
          │              ▼
          │    ┌───────────────────┐
          │    │ Pattern Matcher   │─▶ Extract metadata
          │    └─────────┬─────────┘
          │              │
          ▼              ▼
┌─────────────────────────────┐
│  LogRepository.Create()     │─▶ Insert with retry logic
└─────────┬───────────────────┘
          │
          ├──────────────┐
          │              ▼
          │    ┌───────────────────┐
          │    │  SSE Hub          │─▶ Broadcast log.created
          │    └───────────────────┘
          │
          ▼
    Return 201 Created
```

### Host registration flow

```
Client POST /api/v1/hosts
        │
        ▼
┌───────────────────┐
│  Host Handler     │─▶ Generate API key (glog_v1_<46-char-hex>)
└─────────┬─────────┘
          │
          ▼
┌─────────────────────────────┐
│  HostRepository.Create()    │─▶ Insert with generated key
└─────────┬───────────────────┘
          │
          ├──────────────┐
          │              ▼
          │    ┌───────────────────┐
          │    │  SSE Hub          │─▶ Broadcast host.registered
          │    └───────────────────┘
          │
          ▼
    Return 201 Created + API key
```

## Dependency rules

1. **Domain Layer**: Zero dependencies on infrastructure
   - `entities/`: Pure Go structs
   - `ports/`: Interfaces only
   - `services/`: Business logic, no HTTP/DB

2. **Infrastructure Layer**: Depends on domain interfaces
   - `http/`: Handlers depend on `domain/ports` interfaces
   - `persistence/`: Repositories implement `domain/ports` interfaces

3. **External Boundaries**:
   - HTTP: Chi router for routing only
   - Database: `modernc.org/sqlite` (pure Go, no CGo)
   - Config: Environment variables + file (Viper not needed)

## Concurrency strategy

### SQLite concurrency

**Challenge**: SQLite allows only one writer at a time.

**Solution**: Three-pronged approach

1. **WAL Mode** (Write-Ahead Logging)
   ```sql
   PRAGMA journal_mode=WAL;
   ```
   - Readers don't block writers
   - Writers don't block readers

2. **Single Writer Connection**
   ```go
   db.SetMaxOpenConns(1)
   ```
   - Serialize writes at connection pool level
   - Prevents "database is locked" errors

3. **Retry Logic with Exponential Backoff**
   ```go
   for i := 0; i < maxRetries; i++ {
       err := // ... db operation
       if isTransientError(err) {
           time.Sleep(10ms * (1 << i))
           continue
       }
   }
   ```



### SSE broadcasting

**Challenge**: Broadcast to multiple clients without blocking.

**Solution**: Buffered channels with goroutine per client

```go
type Hub struct {
    clients map chan<- Event
    register chan chan<- Event
    broadcast chan Event
}
```

- Each client has dedicated goroutine
- Non-blocking broadcast via buffered channels
- Automatic cleanup on disconnect



## Error handling strategy

### Transient errors

**SQLite errors that retry**:
- `database is locked`
- `database is busy`
- `SQLITE_BUSY`

**Action**: Exponential backoff (10ms → 20ms → 40ms)

### Permanent errors

**Errors that fail immediately**:
- Invalid API key
- Malformed JSON
- Constraint violation (duplicate host name)

**Action**: Return 400/401/409 to client

### Self-logging

**GLog logs its own errors** to debug production issues:

```go
logger.LogSQLError("FindByAPIKey", query, err)
// Output: [GLOG] 2026-01-06 22:30:04 | database_locked | FindByAPIKey | retryable=true
```

**See**: [Database Design](database-design.md) for error patterns

## Security model

### Authentication

- **Write operations**: Bearer token required (API key)
- **Read operations**: Public (no authentication)
- **API key format**: `glog_v1_<46-char-hex>`

### Input validation

- **Log levels**: Validated against constants in `domain/valueobjects/`
- **API keys**: Length check (54 chars) + format validation
- **JSON fields**: Schema validation via entities

### Rate limiting

**Not implemented** (planned for multi-host deployments)



## Testing strategy

### Unit tests

**Domain layer**:
- Entity validation (Host, Log, JSONMap)
- Service logic (pattern matching, metadata derivation)
- Value objects (severity levels)

### Integration tests

**Infrastructure layer**:
- HTTP handlers (test via httptest.NewRecorder)
- Repository operations (test with in-memory SQLite)
- SSE broadcasting (test via concurrent clients)

### Concurrency tests

**Critical for SQLite**:
- 10+ simultaneous writers
- Verify retry logic engages
- Confirm no data loss


## Performance characteristics

### Throughput

**Single host**:
- Read operations: 1000+ req/s (SQLite WAL)
- Write operations: 100-200 logs/s (single writer bottleneck)

**Multi-host** (> 100 hosts):
- Consider PostgreSQL migration
- Connection pooling via PgBouncer
- Partitioning for large datasets

### Optimization

**Already implemented**:
- WAL mode: Readers don't block writers
- Connection pooling: Single writer, multiple readers
- 64MB cache: Via migration 003
- Indexed queries: host_id, created_at, level



## Related docs

- [Database design](database-design.md) — schema, migrations, SQLite config
- [Domain design](domain-design.md) — entities, ports, services
- [SSE events API](../api/events.md) — event format reference
- [Deployment](../guides/deployment.md) — production setup
