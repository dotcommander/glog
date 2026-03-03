# SSE events API

Real-time log streaming using Server-Sent Events (SSE).

## Overview

Connect to this endpoint to receive a stream of events as logs are created and hosts are registered.

## Endpoint

```
GET /api/v1/events
```

**Authentication:** None required (public endpoint)

## Connection

### Headers

The endpoint sets standard SSE headers:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
Access-Control-Allow-Origin: *
```

### Connection Event

Upon connecting, you'll receive an initial `connected` event:

```json
event: connected
data: {"status":"connected","timestamp":"2026-01-18T23:10:45Z"}

```

### Keep-alive

A `: keep-alive` comment is sent periodically to prevent proxies from closing idle connections.

## Event types

### `log.created`

Emitted when a new log entry is created.

**Payload Schema:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique log identifier |
| `host_id` | integer | ID of the host that created the log |
| `level` | string | Log level (`trace`, `debug`, `info`, `warn`, `error`, `fatal`) |
| `message` | string | Log message |
| `fields` | object (optional) | Additional structured data |
| `timestamp` | string | ISO 8601 timestamp when the log occurred |
| `created_at` | string | ISO 8601 timestamp when the log was stored |
| `host` | object (optional) | Host details (name, tags, status) |

**Example:**

```json
event: log.created
data: {"id":1234,"host_id":5,"level":"error","message":"Database connection failed","fields":{"error":"timeout","retry_count":3},"timestamp":"2026-01-18T23:10:45Z","created_at":"2026-01-18T23:10:45Z","host":{"id":5,"name":"production-api","tags":["prod","api"],"status":"healthy"}}

```

### `host.registered`

Emitted when a new host is registered.

**Payload Schema:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | integer | Unique host identifier |
| `name` | string | Host name |
| `tags` | array | List of tag strings |
| `status` | string | Host status (`unknown`, `healthy`, `degraded`, `down`) |
| `created_at` | string | ISO 8601 timestamp when host was registered |
| `last_seen` | string | ISO 8601 timestamp of last activity |
| `log_count` | integer | Total number of logs from this host |
| `error_count` | integer | Number of error-level logs |
| `error_rate` | float | Error rate as percentage |
| `description` | string (optional) | Host description |
| `api_key` | string | API key for this host |

**Example:**

```json
event: host.registered
data: {"id":5,"name":"production-api","tags":["prod","api"],"status":"unknown","created_at":"2026-01-18T23:10:45Z","last_seen":"2026-01-18T23:10:45Z","log_count":0,"error_count":0,"error_rate":0,"api_key":"glog_v1_b7cfad39c28529d5124da53fcb34493d132d706a82f2fa"}

```

### `host.updated`

Emitted when host information is updated (e.g., status change, statistics refresh).

**Payload Schema:** Same as `host.registered`

## Examples

### curl

```bash
curl -N http://localhost:6016/api/v1/events
```

The `-N` flag disables buffering, ensuring events are displayed as they arrive.

### JavaScript

```javascript
const eventSource = new EventSource('http://localhost:6016/api/v1/events');

// Connection established
eventSource.addEventListener('connected', (e) => {
  console.log('SSE connected:', JSON.parse(e.data));
});

// New log created
eventSource.addEventListener('log.created', (e) => {
  const log = JSON.parse(e.data);
  console.log('New log:', log.message, `(${log.level})`);

  // Update UI, trigger alerts, etc.
});

// Host registered
eventSource.addEventListener('host.registered', (e) => {
  const host = JSON.parse(e.data);
  console.log('New host:', host.name);
});

// Host updated
eventSource.addEventListener('host.updated', (e) => {
  const host = JSON.parse(e.data);
  console.log('Host updated:', host.name, `status: ${host.status}`);
});

// Connection error handling
eventSource.onerror = (error) => {
  console.error('SSE error:', error);
  // EventSource will attempt to reconnect automatically
};

// Close connection when done
// eventSource.close();
```

### JavaScript with custom reconnection

```javascript
function connectEvents() {
  const eventSource = new EventSource('/api/v1/events');

  eventSource.onopen = () => {
    console.log('Events connected');
  };

  eventSource.addEventListener('log.created', (e) => {
    const log = JSON.parse(e.data);
    onLogCreated(log);
  });

  eventSource.onerror = (e) => {
    console.error('Connection lost, reconnecting...');
    eventSource.close();
    // Exponential backoff before reconnect
    setTimeout(connectEvents, 5000);
  };
}

connectEvents();
```

### Python

```python
import requests
import json

def stream_events():
    with requests.get('http://localhost:6016/api/v1/events', stream=True) as response:
        response.raise_for_status()

        for line in response.iter_lines():
            if line:
                line = line.decode('utf-8')

                if line.startswith('event: '):
                    event_type = line[7:]
                elif line.startswith('data: '):
                    data = json.loads(line[6:])
                    print(f"Event: {event_type}, Data: {data}")

stream_events()
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
)

type Event struct {
    Type string
    Data json.RawMessage
}

func streamEvents() error {
    resp, err := http.Get("http://localhost:6016/api/v1/events")
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    decoder := json.NewDecoder(resp.Body)
    var eventType string
    var eventData json.RawMessage

    reader := io.Reader(resp.Body)
    scanner := bufio.NewScanner(reader)

    for scanner.Scan() {
        line := scanner.Text()

        if strings.HasPrefix(line, "event: ") {
            eventType = strings.TrimPrefix(line, "event: ")
        } else if strings.HasPrefix(line, "data: ") {
            data := strings.TrimPrefix(line, "data: ")
            if err := json.Unmarshal([]byte(data), &eventData); err != nil {
                continue
            }

            fmt.Printf("Event: %s, Data: %s\n", eventType, eventData)
        }
    }

    return scanner.Err()
}
```

## Notes

Browsers reconnect automatically. Other clients should implement exponential backoff.

If a client receives events slower than they arrive, its channel fills and the server disconnects it. The client will reconnect and resume from the current stream position.

Always close the `EventSource` or HTTP connection when done.


1. **Reconnection**: SSE includes automatic reconnection in browsers. For other clients, implement exponential backoff.
2. **Buffer Management**: Set appropriate buffer sizes for high-volume scenarios. Events may be dropped if the client can't keep up.
3. **Event Filtering**: Consider server-side filtering for specific hosts or log levels to reduce bandwidth.
4. **Graceful Shutdown**: Always close the EventSource or HTTP connection when done to free server resources.

## Troubleshooting

### No events received

- Verify the server is running: `curl http://localhost:6016/health`
- Check for proxy or firewall blocking SSE connections
- Ensure the client doesn't have buffering enabled

### Connection drops

- SSE automatically reconnects in browsers
- Implement custom reconnection logic for other clients
- Check server logs for client timeout issues

### High memory usage

- Close unused connections
- Consider implementing server-side event filtering
- Monitor `/health` endpoint for active client count
