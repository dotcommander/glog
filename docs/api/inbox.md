# Agent inbox API

Create an inbox item for a coding agent:

```bash
curl -s -X POST http://localhost:6016/api/v1/inbox \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Verify SSE reconnect behavior",
    "body": "Re-run the browser smoke after touching the SSE hub.",
    "kind": "task",
    "priority": "normal",
    "repo": "dotcommander/glog",
    "session_id": "2026-05-15-responsive-ui",
    "tags": ["frontend", "sse"]
  }'
```

The agent inbox is a stateful coordination surface for notes, tasks, questions, bugs, and handoffs. Keep runtime evidence in logs and use inbox items to track what an agent or human should do next.

## CLI

```bash
glog inbox add "Verify SSE reconnect behavior" \
  --server http://localhost:6016 \
  --body "Re-run the browser smoke after touching the SSE hub." \
  --kind task \
  --priority normal \
  --repo dotcommander/glog \
  --session 2026-05-15-responsive-ui \
  --tag frontend \
  --tag sse

glog inbox list --server http://localhost:6016 --status open
glog inbox done 123 --server http://localhost:6016
```

Configuration for `--server` follows normal CLI resolution: flag, then `GLOG_SERVER`, then `./glog.json`.

## Data model

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | integer | generated | Inbox item ID |
| `title` | string | required | Short item title |
| `body` | string | `""` | Details, evidence, or next action |
| `status` | string | `open` | `open`, `in_progress`, `blocked`, `done`, `dismissed` |
| `kind` | string | `task` | `note`, `task`, `question`, `bug`, `handoff` |
| `priority` | string | `normal` | `low`, `normal`, `high` |
| `source` | string | `agent` | Producer such as `agent`, `human`, or `system` |
| `repo` | string | `""` | Repository or workspace name |
| `session_id` | string | `""` | Agent/session/run identifier |
| `tags` | string array | `[]` | Freeform labels |
| `linked_log_ids` | string array | `[]` | Related log IDs encoded as strings |
| `linked_fingerprints` | string array | `[]` | Related log fingerprints |
| `created_by` | string | `""` | Creator identity |
| `claimed_by` | string | `""` | Worker/agent currently owning it |
| `created_at` | timestamp | generated | Creation time |
| `updated_at` | timestamp | generated | Last update time |
| `due_at` | RFC3339 timestamp | omitted | Optional due time |

## Endpoints

### List items

```http
GET /api/v1/inbox
```

Query parameters:

| Parameter | Description |
|-----------|-------------|
| `status` | Filter by status |
| `kind` | Filter by kind |
| `repo` | Filter by repository/workspace |
| `session_id` | Filter by session ID |
| `source` | Filter by source |
| `limit` | Page size, normalized by server defaults/maximums |
| `offset` | Pagination offset |

Example:

```bash
curl -s "http://localhost:6016/api/v1/inbox?status=open&repo=dotcommander/glog"
```

Response:

```json
{
  "items": [
    {
      "id": 123,
      "title": "Verify SSE reconnect behavior",
      "body": "Re-run the browser smoke after touching the SSE hub.",
      "status": "open",
      "kind": "task",
      "priority": "normal",
      "source": "agent",
      "repo": "dotcommander/glog",
      "session_id": "2026-05-15-responsive-ui",
      "tags": ["frontend", "sse"],
      "linked_log_ids": [],
      "linked_fingerprints": [],
      "created_at": "2026-05-15T17:08:06Z",
      "updated_at": "2026-05-15T17:08:06Z"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

### Create item

```http
POST /api/v1/inbox
Content-Type: application/json
```

Required field: `title`.

```bash
curl -s -X POST http://localhost:6016/api/v1/inbox \
  -H "Content-Type: application/json" \
  -d '{"title":"Ask user for production DB path","kind":"question","priority":"high"}'
```

Returns `201 Created` with the created item.

### Get item

```http
GET /api/v1/inbox/{id}
```

Returns `404 Not Found` when the item does not exist.

### Update item

```http
PATCH /api/v1/inbox/{id}
Content-Type: application/json
```

The update payload uses the same fields as create. Empty strings preserve existing optional string fields for partial updates, except `body` can currently only be cleared by replacing the full item through code. Arrays replace the stored arrays when provided.

```bash
curl -s -X PATCH http://localhost:6016/api/v1/inbox/123 \
  -H "Content-Type: application/json" \
  -d '{"status":"in_progress","claimed_by":"codex"}'
```

### Mark done

```http
POST /api/v1/inbox/{id}/done
```

Shortcut for setting `status` to `done`.

```bash
curl -s -X POST http://localhost:6016/api/v1/inbox/123/done
```

### Delete item

```http
DELETE /api/v1/inbox/{id}
```

Returns `204 No Content` when deleted.

## Authentication

Inbox endpoints are currently unauthenticated, like the dashboard read APIs. This keeps local agent workflows low-friction. Do not expose a GLog server with public inbox mutations to the internet without an external auth layer or a future authenticated mutation policy.
