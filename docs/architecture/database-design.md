# Database design

## SQLite configuration

### WAL mode

**Enabled via migration 001**:
```sql
PRAGMA journal_mode=WAL;
```

**Benefits**:
- Readers don't block writers
- Writers don't block readers
- 10-100x concurrent read performance
- Better crash recovery

**Files created**:
```
test.db       - Main database
test.db-shm   - Shared memory
test.db-wal   - Write-ahead log
```

### Connection pooling

**Single writer configuration**:
```go
db.SetMaxOpenConns(1)   // SQLite works best with single writer
db.SetMaxIdleConns(1)
db.SetConnMaxLifetime(time.Hour)
```

**Why single writer?**
- SQLite allows only one writer at a time
- Connection pool serializes writes
- Prevents "database is locked" errors

### Busy timeout

**Retry for 5 seconds on lock**:
```sql
PRAGMA busy_timeout = 5000;
```

**Behavior**:
- SQL statement waits up to 5 seconds
- Automatically retries if database is locked
- Returns `SQLITE_BUSY` if timeout exceeded

### Performance tuning

**Via migration 003**:
```sql
PRAGMA synchronous = NORMAL;    -- Faster writes with WAL
PRAGMA cache_size = -64000;     -- 64MB cache (-KB value)
PRAGMA temp_store = MEMORY;     -- Temp tables in RAM
PRAGMA foreign_keys = ON;       -- Enforce FK constraints
```

## Schema

### Hosts table

```sql
CREATE TABLE hosts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    api_key TEXT NOT NULL UNIQUE,
    tags TEXT DEFAULT '[]',
    description TEXT,
    status TEXT DEFAULT 'unknown',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Indexes**:
```sql
CREATE INDEX idx_hosts_api_key ON hosts(api_key);
CREATE INDEX idx_hosts_status ON hosts(status);
CREATE INDEX idx_hosts_created_at ON hosts(created_at);
```

### Logs table

```sql
CREATE TABLE logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id INTEGER NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    fields TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);
```

**Indexes**:
```sql
CREATE INDEX idx_logs_host_id ON logs(host_id);
CREATE INDEX idx_logs_created_at ON logs(created_at DESC);
CREATE INDEX idx_logs_level ON logs(level);
CREATE INDEX idx_logs_host_created ON logs(host_id, created_at DESC);
```

## Retry logic

### Transient errors

**Errors that should retry**:
- `database is locked`
- `database is busy`
- `SQLITE_BUSY`

### Implementation

```go
const maxRetries = 3

func (r *LogRepository) Create(log *entities.Log) error {
    for i := 0; i < maxRetries; i++ {
        err := r.createLog(log)
        if err == nil {
            return nil
        }

        if i < maxRetries-1 && isTransientError(err) {
            backoff := time.Millisecond * 10 * (1 << i) // 10ms, 20ms, 40ms
            time.Sleep(backoff)
            continue
        }

        return err
    }
    return nil
}

func isTransientError(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "locked") ||
           strings.Contains(msg, "busy")
}
```

### Backoff strategy

**Exponential backoff**:
- Attempt 1: Immediate
- Attempt 2: 10ms delay
- Attempt 3: 20ms delay
- Failure after 3 attempts

**Why exponential?**
- First failure: Likely transient lock, retry immediately
- Second failure: Lock held longer, small delay
- Third failure: Persistent issue, longer delay

## Concurrency handling

### Read operations

**No blocking**:
- Multiple readers can run simultaneously
- Readers don't wait for writers
- Writers don't wait for readers

**Example**:
```go
func (r *LogRepository) FindByHostID(hostID int64, limit, offset int) ([]*entities.Log, error) {
    // No retry needed for reads (WAL mode)
    query := `SELECT id, host_id, level, message, fields, created_at
              FROM logs WHERE host_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`
    // ...
}
```

### Write operations

**Serialized via connection pool**:
```go
func (r *LogRepository) Create(log *entities.Log) error {
    // Retry logic handles transient locks
    for i := 0; i < maxRetries; i++ {
        err := r.createLog(log)
        // ... retry with backoff
    }
}
```

**Bypass connection pool for bulk inserts**:
```go
func (r *LogRepository) BulkCreate(logs []*entities.Log) error {
    tx, _ := r.db.Begin()
    defer tx.Rollback()

    stmt, _ := tx.Prepare(`INSERT INTO logs (...) VALUES (...)`)
    defer stmt.Close()

    for _, log := range logs {
        stmt.Exec(...)
    }

    return tx.Commit()
}
```

## Error handling

### Self-logging

**GLog logs its own errors**:
```go
type GLogLogger struct {
    hostID  int64
    apiKey  string
    baseURL string
}

func (l *GLogLogger) LogSQLError(operation, query string, err error) {
    if l == nil {
        return  // Avoid infinite loop
    }

    log := &entities.Log{
        Level:   "error",
        Message: fmt.Sprintf("SQLite error in %s: %v", operation, err),
        Fields: map[string]interface{}{
            "query":      query,
            "retryable":  isTransientError(err),
            "error_type": getErrorType(err),
        },
    }

    // Send to GLog server (non-blocking)
    go l.sendLog(log)
}
```

**Usage in repositories**:
```go
err := r.createLog(log)
if err != nil && isTransientError(err) {
    r.logger.LogSQLError("Create", query, err)
}
```

## Migration to PostgreSQL

### When to migrate

**Signs you need PostgreSQL**:
- > 100 hosts sending logs
- > 1000 logs/second sustained
- Need full-text search
- Need partitioning for retention policies

### Migration steps

1. **Add PostgreSQL repository**:
   ```
   internal/infrastructure/persistence/postgresql/
   ├── database.go
   ├── host_repository.go
   └── log_repository.go
   ```

2. **Implement same interfaces**:
   ```go
   type HostRepository interface {
       Create(host *entities.Host) error
       FindByAPIKey(apiKey string) (*entities.Host, error)
       // ... same interface as SQLite
   }
   ```

3. **Factory pattern**:
   ```go
   func NewRepository(dbType, conn string) (HostRepository, error) {
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

4. **Configuration**:
   ```json
   {
     "database": {
       "type": "postgresql",
       "connection": "postgres://user:pass@host/glog?sslmode=require"
     }
   }
   ```

### Schema differences

**PostgreSQL advantages**:
- Native JSONB type (vs. TEXT for JSONMap)
- Full-text search (GIN indexes)
- Partitioning by date
- Better concurrent writes

**Migration script**:
```sql
-- Export from SQLite
sqlite3 test.db .dump > dump.sql

-- Import to PostgreSQL (with adjustments)
psql glog < dump.sql

-- Add PostgreSQL-specific features
ALTER TABLE logs ALTER COLUMN fields TYPE JSONB USING fields::JSONB;
CREATE INDEX idx_logs_fields ON logs USING GIN (fields);
```

## Backup strategy

### SQLite backup

**Simple file copy**:
```bash
cp test.db test.db.backup.$(date +%Y%m%d)
```

**Online backup** (via SQLite API):
```go
func (r *Repository) Backup(path string) error {
    _, err := r.db.Exec("VACUUM INTO ?", path)
    return err
}
```

### Restore

**Replace database file**:
```bash
mv test.db.backup.20250107 test.db
```

**Note**: WAL files are regenerated automatically

## Monitoring

### Health check

```go
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    stats := h.db.Stats()
    health := map[string]interface{}{
        "database": map[string]interface{}{
            "path":   h.dbPath,
            "status": "ok",
            "stats": map[string]interface{}{
                "max_open_connections": stats.MaxOpenConnections,
                "open_connections":     stats.OpenConnections,
                "in_use":              stats.InUse,
                "idle":                stats.Idle,
                "wait_count":          stats.WaitCount,
                "wait_duration":       stats.WaitDuration,
            },
        },
    }
    json.NewEncoder(w).Encode(health)
}
```

### Performance metrics

**Track these**:
- `wait_count`: Number of connections waited for
- `wait_duration`: Total time waiting for connections
- `open_connections`: Current connection count
- `idle`: Idle connections in pool

**Alert if**:
- `wait_count` increasing rapidly
- `wait_duration` > 100ms
- `open_connections` consistently at max

## Troubleshooting

### "database is locked"

**Cause**: Multiple writers competing (rare with our config)

**Fix**:
1. Verify WAL mode: `sqlite3 test.db "PRAGMA journal_mode;"`
2. Check busy_timeout: `sqlite3 test.db "PRAGMA busy_timeout;"`
3. Verify single writer: `db.SetMaxOpenConns(1)`

### WAL files missing

**Check**: `ls -la test.db*`

**If missing WAL**:
```bash
sqlite3 test.db "PRAGMA journal_mode=WAL;"
```

### Performance degradation

**Check cache size**:
```bash
sqlite3 test.db "PRAGMA cache_size;"
```

**Should return**: `-64000` (64MB)

**If not**, re-run migration 003
