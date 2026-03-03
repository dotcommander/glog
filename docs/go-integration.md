# Go integration

Send logs to glog from a Go application.

## Quick start

### Install

```bash
go install github.com/dotcommander/glog/cmd/glog@latest
```

### Register a host

```bash
glog host register --name "my-backend-service" --tag production --tag api
```

This creates a `glog.json` config file with your API key.

### Send logs

```bash
# Send a single log
glog log "User registration successful" --level info --field user_id=123

# Stream logs from your application
./my-app 2>&1 | glog log --level error
```

## Integration patterns

### Pattern 1: Direct CLI calls

Simplest option. Calls glog as a subprocess:

```go
package main

import (
    "fmt"
    "os/exec"
)

func logToGLog(level, message string, fields map[string]interface{}) error {
    args := []string{"log", message, "--level", level}

    for k, v := range fields {
        args = append(args, "--field", fmt.Sprintf("%s=%v", k, v))
    }

    cmd := exec.Command("glog", args...)
    return cmd.Run()
}

// Usage
logToGLog("info", "Payment processed", map[string]interface{}{
    "amount": 99.99,
    "user_id": 456,
    "payment_method": "credit_card",
})
```

### Pattern 2: HTTP API client (recommended)

Direct HTTP API calls — better performance than subprocess:

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type GLogClient struct {
    serverURL string
    apiKey    string
    client    *http.Client
}

type LogEntry struct {
    Level    string                 `json:"level"`
    Message  string                 `json:"message"`
    Fields   map[string]interface{} `json:"fields,omitempty"`
    Method   string                 `json:"method,omitempty"`
    Path     string                 `json:"path,omitempty"`
    Status   int                    `json:"status_code,omitempty"`
    Duration int                    `json:"duration_ms,omitempty"`
}

func NewGLogClient(serverURL, apiKey string) *GLogClient {
    return &GLogClient{
        serverURL: serverURL,
        apiKey:    apiKey,
        client: &http.Client{
            Timeout: 5 * time.Second,
        },
    }
}

func (c *GLogClient) SendLog(entry LogEntry) error {
    jsonData, err := json.Marshal(entry)
    if err != nil {
        return fmt.Errorf("marshaling log: %w", err)
    }

    req, err := http.NewRequest("POST", c.serverURL+"/api/v1/logs", bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("creating request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+c.apiKey)

    resp, err := c.client.Do(req)
    if err != nil {
        return fmt.Errorf("sending request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    return nil
}

// Usage
func main() {
    client := NewGLogClient("http://localhost:6016", "your-api-key")

    // Simple log
    client.SendLog(LogEntry{
        Level:   "info",
        Message: "Application started",
        Fields: map[string]interface{}{
            "version": "1.0.0",
            "pid":     12345,
        },
    })

    // HTTP request log
    client.SendLog(LogEntry{
        Level:    "info",
        Message:  "GET /api/users",
        Method:   "GET",
        Path:     "/api/users",
        Status:   200,
        Duration: 45,
        Fields: map[string]interface{}{
            "user_count": 150,
            "cache_hit":  true,
        },
    })
}
```

### Pattern 3: Logger wrapper

A thin wrapper that matches standard Go logging patterns:

```go
package main

import (
    "fmt"
    "os"
)

// Logger provides a structured logging interface
type Logger struct {
    client *GLogClient
    host   string
    source string
}

func NewLogger(client *GLogClient, host, source string) *Logger {
    return &Logger{
        client: client,
        host:   host,
        source: source,
    }
}

// log is the internal logging method
func (l *Logger) log(level, message string, fields map[string]interface{}) {
    if fields == nil {
        fields = make(map[string]interface{})
    }

    // Add standard fields
    fields["host"] = l.host
    if l.source != "" {
        fields["source"] = l.source
    }

    entry := LogEntry{
        Level:   level,
        Message: message,
        Fields:  fields,
    }

    // Don't block on logging errors
    go func() {
        if err := l.client.SendLog(entry); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to send log: %v\n", err)
        }
    }()
}

// Convenience methods
func (l *Logger) Debug(msg string, fields map[string]interface{}) {
    l.log("debug", msg, fields)
}

func (l *Logger) Info(msg string, fields map[string]interface{}) {
    l.log("info", msg, fields)
}

func (l *Logger) Warn(msg string, fields map[string]interface{}) {
    l.log("warn", msg, fields)
}

func (l *Logger) Error(msg string, fields map[string]interface{}) {
    l.log("error", msg, fields)
}

func (l *Logger) Fatal(msg string, fields map[string]interface{}) {
    l.log("fatal", msg, fields)
    os.Exit(1)
}
```

### Pattern 4: HTTP middleware

Logs all incoming requests with method, path, status, and duration:

```go
package main

import (
    "net/http"
    "time"
)

// LoggingMiddleware logs all HTTP requests
type LoggingMiddleware struct {
    logger *Logger
    next   http.Handler
}

func (m *LoggingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // Wrap response writer to capture status code
    wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

    m.next.ServeHTTP(wrapped, r)

    duration := time.Since(start).Milliseconds()

    // Determine log level based on status code
    level := "info"
    if wrapped.statusCode >= 400 && wrapped.statusCode < 500 {
        level = "warn"
    } else if wrapped.statusCode >= 500 {
        level = "error"
    }

    // Create log entry matching GLog's HTTP context format
    logEntry := LogEntry{
        Level:    level,
        Message:  fmt.Sprintf("%s %s", r.Method, r.URL.Path),
        Method:   r.Method,
        Path:     r.URL.Path,
        Status:   wrapped.statusCode,
        Duration: int(duration),
        Fields: map[string]interface{}{
            "remote_addr": r.RemoteAddr,
            "user_agent":  r.UserAgent(),
        },
    }

    m.logger.log(logEntry.Level, logEntry.Message, logEntry.Fields)
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

// Usage
func main() {
    client := NewGLogClient("http://localhost:6016", "your-api-key")
    logger := NewLogger(client, "api-server-1", "http-server")

    mux := http.NewServeMux()
    mux.HandleFunc("/", homeHandler)

    loggedMux := &LoggingMiddleware{logger: logger, next: mux}

    http.ListenAndServe(":8080", loggedMux)
}
```

### Pattern 5: Bulk logging

Batch logs and flush on a timer or size threshold:

```go
package main

import (
    "sync"
    "time"
)

// BulkLogger batches logs for efficient sending
type BulkLogger struct {
    client   *GLogClient
    logs     []LogEntry
    mux      sync.Mutex
    maxBatch int
    ticker   *time.Ticker
    done     chan bool
}

func NewBulkLogger(client *GLogClient, maxBatch int, flushInterval time.Duration) *BulkLogger {
    bl := &BulkLogger{
        client:   client,
        maxBatch: maxBatch,
        ticker:   time.NewTicker(flushInterval),
        done:     make(chan bool),
    }

    go bl.flushWorker()

    return bl
}

func (bl *BulkLogger) Log(entry LogEntry) {
    bl.mux.Lock()
    bl.logs = append(bl.logs, entry)
    shouldFlush := len(bl.logs) >= bl.maxBatch
    bl.mux.Unlock()

    if shouldFlush {
        bl.Flush()
    }
}

func (bl *BulkLogger) Flush() {
    bl.mux.Lock()
    if len(bl.logs) == 0 {
        bl.mux.Unlock()
        return
    }

    logsToSend := bl.logs
    bl.logs = make([]LogEntry, 0, bl.maxBatch)
    bl.mux.Unlock()

    go func() {
        if err := bl.sendBulk(logsToSend); err != nil {
            fmt.Fprintf(os.Stderr, "Failed to send bulk logs: %v\n", err)
        }
    }()
}

func (bl *BulkLogger) sendBulk(logs []LogEntry) error {
    jsonData, err := json.Marshal(map[string]interface{}{
        "logs": logs,
    })
    if err != nil {
        return err
    }

    req, err := http.NewRequest("POST", bl.client.serverURL+"/api/v1/logs/bulk", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+bl.client.apiKey)

    resp, err := bl.client.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }

    return nil
}

func (bl *BulkLogger) flushWorker() {
    for {
        select {
        case <-bl.ticker.C:
            bl.Flush()
        case <-bl.done:
            bl.Flush()
            return
        }
    }
}

func (bl *BulkLogger) Stop() {
    bl.ticker.Stop()
    close(bl.done)
}

// Usage
func main() {
    client := NewGLogClient("http://localhost:6016", "your-api-key")
    bulkLogger := NewBulkLogger(client, 100, 5*time.Second)
    defer bulkLogger.Stop()

    // Log as normal
    bulkLogger.Log(LogEntry{
        Level:   "info",
        Message: "Application event",
        Fields:  map[string]interface{}{"user_id": 123},
    })
}
```

## Configuration

Load from environment or config file:

```go
package main

import (
    "encoding/json"
    "os"
)

type Config struct {
    ServerURL string `json:"server"`
    APIKey    string `json:"api_key"`
    HostID    int    `json:"host_id"`
    HostName  string `json:"host_name"`
}

func LoadConfig() (*Config, error) {
    // Priority: env vars > config file > defaults
    config := &Config{
        ServerURL: getEnv("GLOG_SERVER", "http://localhost:6016"),
        APIKey:    os.Getenv("GLOG_API_KEY"),
    }

    // Try loading from glog.json if API key not in env
    if config.APIKey == "" {
        data, err := os.ReadFile("glog.json")
        if err == nil {
            if err := json.Unmarshal(data, config); err != nil {
                return nil, fmt.Errorf("parsing glog.json: %w", err)
            }
        }
    }

    if config.APIKey == "" {
        return nil, fmt.Errorf("API key not found in GLOG_API_KEY env or glog.json")
    }

    return config, nil
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

## Retry wrapper

Wrap the client with retry logic:

```go
package main

import (
    "time"
)

// RetryableClient wraps GLogClient with retry logic
type RetryableClient struct {
    *GLogClient
    maxRetries int
    retryDelay time.Duration
}

func NewRetryableClient(client *GLogClient, maxRetries int, retryDelay time.Duration) *RetryableClient {
    return &RetryableClient{
        GLogClient: client,
        maxRetries: maxRetries,
        retryDelay: retryDelay,
    }
}

func (c *RetryableClient) SendLogWithRetry(entry LogEntry) error {
    var lastErr error

    for i := 0; i < c.maxRetries; i++ {
        if err := c.SendLog(entry); err != nil {
            lastErr = err
            time.Sleep(c.retryDelay * time.Duration(i+1)) // Exponential backoff
            continue
        }
        return nil
    }

    return fmt.Errorf("failed after %d retries: %w", c.maxRetries, lastErr)
}
```

## Testing

```go
package main

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestGLogIntegration(t *testing.T) {
    // Create test server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") != "Bearer test-key" {
            t.Errorf("Expected Authorization header")
        }
        w.WriteHeader(http.StatusCreated)
    }))
    defer server.Close()

    // Test client
    client := NewGLogClient(server.URL, "test-key")

    err := client.SendLog(LogEntry{
        Level:   "info",
        Message: "Test log",
        Fields:  map[string]interface{}{"test": true},
    })

    if err != nil {
        t.Errorf("SendLog failed: %v", err)
    }
}
```

## Quick reference

- Use structured fields over embedding data in the message string.
- Use bulk logging (`BulkLogger`) when sending >10 logs/second.
- Log in a goroutine so logging failures never block the caller.
- Keep timeouts short (5s) so a slow glog server does not stall your app.
- Load config from environment variables (`GLOG_SERVER`, `GLOG_API_KEY`) so the same binary works in dev and prod without changes.


1. **Always use structured logging**: Include contextual fields
2. **Choose appropriate log levels**: Debug for development, Info for production
3. **Batch high-volume logs**: Use BulkLogger for performance
4. **Handle logging errors gracefully**: Don't let logging break your app
5. **Include request IDs**: For request tracing across services
6. **Set timeouts**: Prevent logging from blocking your application
7. **Use environment-specific config**: Different settings for dev/staging/prod
8. **Log security events**: Authentication, authorization failures
9. **Include performance metrics**: Duration, memory usage, queue sizes
10. **Follow 12-factor app principles**: Config in environment, not code

## Complete example: microservice

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

func main() {
    // Load configuration
    config, err := LoadConfig()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
        os.Exit(1)
    }

    // Create GLog client
    client := NewGLogClient(config.ServerURL, config.APIKey)
    logger := NewLogger(client, config.HostName, "order-service")

    // Create HTTP server with logging middleware
    mux := http.NewServeMux()
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/api/orders", orderHandler)

    loggedMux := &LoggingMiddleware{logger: logger, next: mux}

    server := &http.Server{
        Addr:    ":8080",
        Handler: loggedMux,
    }

    // Graceful shutdown
    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
        <-sigChan

        logger.Info("Shutting down server", nil)

        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        if err := server.Shutdown(ctx); err != nil {
            logger.Error("Server shutdown failed", map[string]interface{}{"error": err})
        }
    }()

    logger.Info("Server starting", map[string]interface{}{"port": 8080})

    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Fatal("Server failed", map[string]interface{}{"error": err})
    }
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func orderHandler(w http.ResponseWriter, r *http.Request) {
    // Simulate order processing
    time.Sleep(100 * time.Millisecond)

    w.WriteHeader(http.StatusCreated)
    w.Write([]byte(`{"order_id": "12345"}`))
}
```

## Troubleshooting

### Common Issues

1. **"API key invalid"**
   - Verify API key in config or environment
   - Check server URL is correct

2. **"Connection refused"**
   - Ensure GLog server is running
   - Verify network connectivity

3. **Logs not appearing in dashboard**
   - Check log level filters
   - Verify host is registered
   - Look for client-side errors

4. **High memory usage**
   - Reduce batch size
   - Decrease flush interval
   - Check for log spam

### Debug Mode

Enable debug logging for your client:

```go
func (c *GLogClient) SendLog(entry LogEntry) error {
    fmt.Printf("Sending log: %+v\n", entry) // Debug output

    // ... existing code ...
}
```

## Support

- [GitHub Issues](https://github.com/dotcommander/glog/issues)
- [SSE events API](api/events.md)
