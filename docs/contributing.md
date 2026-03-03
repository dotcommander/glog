# Contributing

## Getting started

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linting
5. Submit a pull request

## Commit format

Conventional Commits:

```text
type(scope): description
```

### Types

| Type      | Purpose                                    |
| --------- | ------------------------------------------ |
| `feat`    | New feature                                |
| `fix`     | Bug fix                                    |
| `docs`    | Documentation changes                      |
| `test`    | Adding or updating tests                   |
| `refactor`| Code refactoring (no behavior change)      |
| `chore`   | Build, tooling, dependency updates         |

### Scopes

| Scope     | Area                               |
| --------- | ---------------------------------- |
| `http`    | HTTP handlers, routes, middleware  |
| `sqlite`  | SQLite persistence layer           |
| `cli`     | Command-line interface             |
| `api`     | API contracts, endpoints           |
| `domain`  | Business logic, entities           |
| `sse`     | Server-Sent Events                 |

### Examples

```bash
feat(http): add bulk log endpoint
fix(sqlite): handle database locked errors
docs: update API endpoint documentation
test(domain): add table-driven tests for log validation
refactor(persistence): extract retry logic to helper
chore: upgrade go.mod dependencies
```

## Testing

Before submitting a PR:

### Run tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...
```

### Coverage targets

- Domain layer: Unit tests for all business logic
- HTTP handlers: Integration tests with mocked repos
- SQLite repositories: Integration tests with test database
- Concurrent operations: Test with 10+ simultaneous requests
- Edge cases: Empty inputs, nil values, boundary conditions

### Full test run

```bash
# Full test suite
make test

# Or manually
go test -v -race ./internal/...
```

### Code quality

```bash
# Format code
gofmt -w .

# Lint (if golangci-lint installed)
golangci-lint run

# Build check
go build ./...
```

## Code style

### No global variables

All dependencies must be injected:

```go
// Bad
var db *sql.DB

func handler() {
    db.Query(...)
}

// Good
func handler(db *sql.DB) {
    db.Query(...)
}
```

### Dependency injection

Use interface injection for repositories:

```go
type HostHandler struct {
    repo domain.HostRepository
}

func NewHostHandler(repo domain.HostRepository) *HostHandler {
    return &HostHandler{repo: repo}
}
```

### Context-aware operations

All database and HTTP operations must accept context:

```go
func (r *LogRepository) Create(ctx context.Context, log *entities.Log) error {
    _, err := r.db.ExecContext(ctx, "INSERT INTO logs ...")
    return err
}

func (h *LogHandler) CreateLog(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    // Use ctx for all downstream operations
}
```

### Structured logging

Use structured logging, not `fmt.Printf`:

```go
import "log/slog"

// Good
slog.Info("log created", "id", log.ID, "level", log.Level)

// Bad
fmt.Printf("log created: %v\n", log)
```

### Error handling

- Always check errors
- Wrap errors with context: `fmt.Errorf("failed to save host: %w", err)`
- Return errors from handlers, don't panic

## Pull request process

1. Update documentation if needed
2. Ensure all tests pass
3. Update CHANGELOG.md (if applicable)
4. Your PR will be reviewed and may require changes
5. Once approved, maintainers will merge

## Questions

Open an issue or comment on an existing PR.
