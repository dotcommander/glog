# Domain design

## Overview

The domain layer contains business logic, entities, and interfaces. It has zero dependencies on infrastructure (HTTP, database, frameworks).

## Structure

```
internal/domain/
├── entities/         # Business entities
│   ├── host.go
│   ├── log.go
│   └── jsonmap.go
├── ports/           # Repository interfaces
│   ├── host_repository.go
│   └── log_repository.go
├── services/        # Business logic services
│   └── pattern_matcher.go
└── valueobjects/    # Value objects
    └── severity.go
```

## Entities

### Host

**Business entity representing a log host**:
```go
type Host struct {
    ID          int64     `json:"id"`
    Name        string    `json:"name"`
    APIKey      string    `json:"api_key"`
    Tags        []string  `json:"tags"`
    Description string    `json:"description"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    LastSeen    time.Time `json:"last_seen"`
}
```

**Validation rules**:
- `Name`: Required, unique, 1-100 characters
- `APIKey`: Required, unique, format `glog_v1_<46-char-hex>`
- `Status`: One of `online`, `offline`, `unknown`
- `Tags`: Array of strings, max 10 tags

### Log

**Business entity representing a log entry**:
```go
type Log struct {
    ID        int64                  `json:"id"`
    HostID    int64                  `json:"host_id"`
    Level     string                 `json:"level"`
    Message   string                 `json:"message"`
    Fields    map[string]interface{} `json:"fields"`
    CreatedAt time.Time              `json:"created_at"`
}
```

**Validation rules**:
- `HostID`: Required, must reference valid host
- `Level`: Required, one of valid levels (see valueobjects)
- `Message`: Required, max 10,000 characters
- `Fields`: Optional, JSON object

### JSONMap

**Value object for JSON fields**:
```go
type JSONMap map[string]interface{}

func (j JSONMap) Scan(value interface{}) error {
    // Implement sql.Scanner
}

func (j JSONMap) Value() (driver.Value, error) {
    // Implement driver.Valuer
}
```

**Purpose**: Handle JSON serialization/deserialization for database

## Ports (interfaces)

### HostRepository

**Interface for host data access**:
```go
type HostRepository interface {
    Create(host *Host) error
    FindByID(id int64) (*Host, error)
    FindByAPIKey(apiKey string) (*Host, error)
    FindAll(limit, offset int) ([]*Host, error)
    Update(host *Host) error
    Delete(id int64) error
    UpdateStatus(id int64, status string) error
    UpdateLastSeen(id int64, lastSeen time.Time) error
}
```

### LogRepository

**Interface for log data access**:
```go
type LogRepository interface {
    Create(log *Log) error
    FindByID(id int64) (*Log, error)
    FindByHostID(hostID int64, limit, offset int) ([]*Log, error)
    FindByLevel(level string, limit, offset int) ([]*Log, error)
    FindAll(limit, offset int) ([]*Log, error)
    BulkCreate(logs []*Log) error
    Delete(id int64) error
    DeleteByHostID(hostID int64) error
    GetStats(hostID int64) (*HostStats, error)
}
```

**Benefits of interfaces**:
- Swap SQLite for PostgreSQL without changing handlers
- Mock repositories for testing
- Clear contract for repository behavior

## Services

### PatternMatcher

**Business logic for extracting metadata from log messages**:
```go
type PatternMatcher interface {
    ExtractMetadata(message string) JSONMap
    MatchSeverity(message string) string
    DetectErrorType(message string) string
}
```

**Example implementation**:
```go
func (m *PatternMatcher) ExtractMetadata(message string) JSONMap {
    metadata := make(JSONMap)

    // Extract error codes
    if reError.MatchString(message) {
        metadata["error_type"] = "error"
    }

    // Extract URLs
    if urls := reURL.FindAllString(message, -1); len(urls) > 0 {
        metadata["urls"] = urls
    }

    // Extract request IDs
    if match := reRequestID.FindStringSubmatch(message); len(match) > 1 {
        metadata["request_id"] = match[1]
    }

    return metadata
}
```

**Usage in handlers**:
```go
func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    log := parseLog(r)

    // Use domain service for business logic
    metadata := h.matcher.ExtractMetadata(log.Message)
    for k, v := range metadata {
        log.Fields[k] = v
    }

    err := h.repo.Create(log)
    // ...
}
```

## Value objects

### Severity

**Value object for log levels**:
```go
const (
    LevelDebug = "debug"
    LevelInfo  = "info"
    LevelWarn  = "warn"
    LevelError = "error"
    LevelFatal = "fatal"
)

var ValidLevels = []string{
    LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal,
}

func IsValidLevel(level string) bool {
    for _, l := range ValidLevels {
        if level == l {
            return true
        }
    }
    return false
}
```

**Benefits**:
- Single source of truth for valid levels
- Easy to add new levels
- Consistent validation across codebase

## Design principles

### Dependency rule

**Domain layer depends on nothing**:
- No HTTP dependencies
- No database dependencies
- No framework dependencies
- Pure Go structs and interfaces

**Infrastructure depends on domain**:
```go
// Infrastructure implements domain interfaces
type SQLiteHostRepository struct {
    db *sql.DB
}

func (r *SQLiteHostRepository) Create(host *entities.Host) error {
    // Implements domain.HostRepository interface
}
```

### Entity validation

**Validate in entities, not handlers**:
```go
func (h *Host) Validate() error {
    if h.Name == "" {
        return fmt.Errorf("name is required")
    }
    if len(h.Name) > 100 {
        return fmt.Errorf("name too long")
    }
    if !IsValidAPIKey(h.APIKey) {
        return fmt.Errorf("invalid API key format")
    }
    return nil
}
```

**Usage**:
```go
host := &Host{Name: "test", APIKey: "invalid"}
if err := host.Validate(); err != nil {
    return err
}
```

### Immutability

**Entities are mutable** (Go convention), but value objects are immutable:

```go
// Mutable entity
func (h *Host) SetStatus(status string) {
    h.Status = status
}

// Immutable value object (return new instance)
func (s Severity) WithLevel(level string) Severity {
    return Severity{Level: level}
}
```

## Testing

### Unit tests

**Test domain logic without infrastructure**:
```go
func TestPatternMatcher_ExtractMetadata(t *testing.T) {
    matcher := NewPatternMatcher()

    tests := []struct {
        message  string
        expected JSONMap
    }{
        {
            message: "Error: connection timeout",
            expected: JSONMap{"error_type": "error"},
        },
        {
            message: "Request ID: abc-123 completed",
            expected: JSONMap{"request_id": "abc-123"},
        },
    }

    for _, tt := range tests {
        result := matcher.ExtractMetadata(tt.message)
        assert.Equal(t, tt.expected, result)
    }
}
```

### Mock repositories

**Mock for handler testing**:
```go
type MockHostRepository struct {
    hosts map[int64]*Host
}

func (m *MockHostRepository) Create(host *Host) error {
    host.ID = int64(len(m.hosts) + 1)
    m.hosts[host.ID] = host
    return nil
}

func (m *MockHostRepository) FindByAPIKey(apiKey string) (*Host, error) {
    for _, host := range m.hosts {
        if host.APIKey == apiKey {
            return host, nil
        }
    }
    return nil, fmt.Errorf("not found")
}
```

**Usage in tests**:
```go
func TestHostHandler_CreateHost(t *testing.T) {
    mockRepo := NewMockHostRepository()
    handler := NewHostHandler(mockRepo)

    req := httptest.NewRequest("POST", "/api/v1/hosts", body)
    w := httptest.NewRecorder()

    handler.ServeHTTP(w, req)

    assert.Equal(t, 201, w.Code)
}
```

## Best practices

### Entity design

1. **Keep entities simple**: Just data + validation
2. **No business logic in entities**: Use services
3. **No infrastructure concerns**: No HTTP, no database

### Interface design

1. **Interface in domain**: Define in `domain/ports/`
2. **Implementation in infrastructure**: Implement in `infrastructure/persistence/`
3. **Interface segregation**: Small, focused interfaces

### Service design

1. **Pure functions**: No side effects when possible
2. **Reusable logic**: Used by multiple handlers
3. **Testable**: No dependencies on HTTP/DB

## Common patterns

### Factory pattern

**Create entities with validation**:
```go
func NewHost(name, description string, tags []string) (*Host, error) {
    host := &Host{
        Name:        name,
        Description: description,
        Tags:        tags,
        Status:      "unknown",
        CreatedAt:   time.Now(),
        LastSeen:    time.Now(),
    }

    if err := host.Validate(); err != nil {
        return nil, err
    }

    return host, nil
}
```

### Builder pattern

**Construct complex queries**:
```go
type LogQueryBuilder struct {
    hostID  *int64
    level   *string
    limit   int
    offset  int
}

func (b *LogQueryBuilder) WithHostID(id int64) *LogQueryBuilder {
    b.hostID = &id
    return b
}

func (b *LogQueryBuilder) WithLevel(level string) *LogQueryBuilder {
    b.level = &level
    return b
}

func (b *LogQueryBuilder) Build() []*Log {
    // Execute query
}
```

### Specification pattern

**Encapsulate business rules**:
```go
type Specification interface {
    IsSatisfied(entity interface{}) bool
}

type HostHasTag struct {
    Tag string
}

func (h HostHasTag) IsSatisfied(entity interface{}) bool {
    host := entity.(*Host)
    for _, tag := range host.Tags {
        if tag == h.Tag {
            return true
        }
    }
    return false
}
```

## Extension guide

### Adding entities

1. **Create entity in `domain/entities/`**
2. **Add validation methods**
3. **Create interface in `domain/ports/`**
4. **Implement repository in `infrastructure/persistence/`**
5. **Create handler in `infrastructure/http/handlers/`**
6. **Add route**

### Adding services

1. **Create interface in `domain/services/`**
2. **Implement in `domain/services/`**
3. **Inject into handlers**
4. **Write unit tests**

### Refactoring entities

1. **Update entity struct**
2. **Update validation**
3. **Update repository SQL**
4. **Update handler parsing**
5. **Run tests**

## Anti-patterns

### ❌ Business Logic in Handlers

```go
// Bad
func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    // Business logic in handler
    if strings.Contains(log.Message, "error") {
        log.Fields["error_type"] = "error"
    }
}

// Good
func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    // Delegate to domain service
    metadata := h.matcher.ExtractMetadata(log.Message)
    log.Fields = metadata
}
```

### ❌ Direct Database Access

```go
// Bad
func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    db := h.db  // Direct DB access
    db.Exec("INSERT INTO logs ...")
}

// Good
func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    // Use repository interface
    h.repo.Create(log)
}
```

### ❌ Fat Entities

```go
// Bad
type Log struct {
    // ... fields ...
    Save() error           // No DB logic in entities
    ValidateLevel() error  // No validation logic
}

// Good
type Log struct {
    // ... fields only ...
}

// Validation in separate method
func (l *Log) Validate() error { ... }

// Persistence in repository
func (r *LogRepository) Create(log *Log) error { ... }
```
