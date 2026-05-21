# Deployment

Deploy as a bare binary, macOS launchd service, Docker container, or systemd service. SQLite is the only database dependency — no separate database server required.

## Contents

- [Prerequisites](#prerequisites)
- [Single-Host Deployment (SQLite)](#single-host-deployment-sqlite)
- [macOS launchd service](#macos-launchd-service)
- [Docker Deployment](#docker-deployment)
- [Docker Compose Deployment](#docker-compose-deployment)
- [Systemd Service](#systemd-service)
- [Production Configuration](#production-configuration)
- [Reverse Proxy Configuration](#reverse-proxy-configuration)
- [Monitoring](#monitoring)
- [Backup and Recovery](#backup-and-recovery)

## Prerequisites

- **Go 1.25+** (for building from source; see `go.mod` for the exact toolchain version)
- **SQLite 3.x** (included via pure Go driver)
- **50MB+ disk space** for database and logs
- **Port 6016** available (default)

## Single-host deployment (SQLite)

The simplest deployment uses SQLite with WAL mode, suitable for deployments with up to 100 hosts sending <1000 logs/second.

### Build

```bash
# Clone repository
git clone https://github.com/dotcommander/glog
cd glog

# Build binary
make build
# Or manually:
go build -o bin/glog ./cmd/glog
```

### Run

```bash
# Basic server (API only)
./bin/glog serve --db ./glog.db --addr :6016

# With web frontend
./bin/glog serve --db ./glog.db --web ./web/build --addr :6016
```

## macOS launchd service

For a developer workstation or small local deployment on macOS, use the
repo `Justfile` to install a user-level LaunchAgent.

```bash
just service-install
```

The recipe builds `web/build`, installs the current CLI to `~/go/bin/glog`,
writes `~/Library/LaunchAgents/dev.glog.plist`, and starts the service with:

```bash
glog serve \
  --addr 127.0.0.1:6016 \
  --db ./glog.db \
  --web ./web/build
```

Check it:

```bash
just service-status
just health
open http://127.0.0.1:6016
```

Operate it:

```bash
just service-restart
just service-logs
just service-uninstall
```

Use a different local port:

```bash
GLOG_ADDR=127.0.0.1:6020 just service-install
```

Manual launchd commands, if you need them:

```bash
launchctl print gui/$(id -u)/dev.glog
launchctl kickstart -k gui/$(id -u)/dev.glog
launchctl bootout gui/$(id -u)/dev.glog
```

Logs are written beside the checkout:

```text
glog.launchd.log
glog.launchd.err.log
```

### Server flags

The server reads its database path and listen address from command-line flags
(there are no `GLOG_*` server environment variables). Pass them explicitly:

```bash
# Set database path and listen address
./bin/glog serve --db /var/lib/glog/glog.db --addr :6016
```

### Directory layout

```
/opt/glog/
├── bin/
│   └── glog           # Server binary
├── data/
│   └── glog.db        # SQLite database
├── web/
│   └── build/         # SvelteKit frontend (optional)
└── config/
    └── glog.json      # CLI config (for sending logs)
```

## Docker

### Dockerfile

Create a `Dockerfile` in the project root:

```dockerfile
# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy source
COPY . .

# Build frontend (optional)
RUN cd web && bun install && bun run build

# Build binary
RUN go build -o /usr/local/bin/glog ./cmd/glog

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /usr/local/bin/glog /usr/local/bin/glog

# Copy frontend build (optional)
COPY --from=builder /app/web/build /app/web/build

# Create data directory
RUN mkdir -p /data

EXPOSE 6016

# Run as non-root user
RUN adduser -D -g '' glog
USER glog

CMD ["glog", "serve", "--db", "/data/glog.db", "--web", "/app/web/build", "--addr", ":6016"]
```

### Build and run

```bash
# Build image
docker build -t glog:latest .

# Run container
docker run -d \
  --name glog \
  -p 6016:6016 \
  -v $(pwd)/data:/data \
  -v $(pwd)/glog.db:/data/glog.db \
  --restart unless-stopped \
  glog:latest
```

## Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  glog:
    build: .
    image: glog:latest
    container_name: glog
    ports:
      - "6016:6016"
    volumes:
      # Persist database
      - ./data:/data
      # Optional: Custom config
      # - ./config/glog.json:/etc/glog/config.json:ro
    # The image CMD already runs `glog serve --db /data/glog.db --addr :6016`.
    # To change the db path or address, override `command:` here rather than
    # setting environment variables (the server has no GLOG_* env vars).
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:6016/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

  # Optional: Caddy reverse proxy
  caddy:
    image: caddy:2-alpine
    container_name: glog-caddy
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    restart: unless-stopped
    depends_on:
      - glog

volumes:
  caddy_data:
  caddy_config:
```

### Run

```bash
# Start services
docker-compose up -d

# View logs
docker-compose logs -f glog

# Stop services
docker-compose down
```

## systemd service

For production deployments on Linux, create a systemd service:

### Service file

Create `/etc/systemd/system/glog.service`:

```ini
[Unit]
Description=GLog Log Aggregation Server
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=glog
Group=glog
WorkingDirectory=/opt/glog

# Build binary
ExecStart=/opt/glog/bin/glog serve \
  --db /var/lib/glog/glog.db \
  --web /opt/glog/web/build \
  --addr :6016

# Auto-restart on failure
Restart=always
RestartSec=5s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/glog /var/log/glog
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=glog

[Install]
WantedBy=multi-user.target
```

### Setup

```bash
# Create user and directories
sudo useradd -r -s /bin/false glog
sudo mkdir -p /opt/glog/{bin,web/build}
sudo mkdir -p /var/lib/glog
sudo mkdir -p /var/log/glog

# Copy binary and files
sudo cp bin/glog /opt/glog/bin/
sudo cp -r web/build /opt/glog/web/

# Set permissions
sudo chown -R glog:glog /opt/glog
sudo chown -R glog:glog /var/lib/glog
sudo chown -R glog:glog /var/log/glog

# Install service
sudo cp glog.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable glog
sudo systemctl start glog

# Check status
sudo systemctl status glog
```

## Production configuration

### Database performance

GLog automatically enables SQLite optimizations:

- **WAL mode**: Enabled for concurrent reads
- **Busy timeout**: 5 seconds with retry logic
- **Cache size**: 64MB (via migration)
- **Connection pool**: Single writer for SQLite

### Resource limits

For systemd, add to service file:

```ini
[Service]
# Resource limits
MemoryLimit=512M
MemoryMax=1G
CPUQuota=100%
TasksMax=512
```

For Docker, add to compose:

```yaml
services:
  glog:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
```

### Security

1. **Run as non-root user**: Already configured in systemd and Docker examples
2. **Firewall rules**: Only expose necessary ports
3. **TLS termination**: Use reverse proxy for HTTPS
4. **API key management**: Store keys in environment variables or secret managers
5. **Database permissions**: `chmod 600` on database file

## Reverse proxy

### Caddy

Automatic HTTPS with Caddy 2:

```
# Caddyfile
logs.example.com {
    reverse_proxy localhost:6016

    # Optional: Basic auth
    basicauth {
        admin $2a$14$...
    }

    # Logging
    log {
        output file /var/log/caddy/glog-access.log
    }
}
```

### Nginx

```nginx
server {
    listen 80;
    server_name logs.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name logs.example.com;

    ssl_certificate /etc/ssl/certs/logs.example.com.crt;
    ssl_certificate_key /etc/ssl/private/logs.example.com.key;

    location / {
        proxy_pass http://localhost:6016;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_buffering off;
        proxy_cache off;
    }

    # Health check bypass auth
    location /health {
        proxy_pass http://localhost:6016;
        access_log off;
    }
}
```

## Monitoring

### Health Check Endpoint

```bash
# Basic health
curl http://localhost:6016/health

# Detailed with database stats
curl http://localhost:6016/health?detailed=true
```

Response:

```json
{
  "status": "healthy",
  "timestamp": "2026-01-18T22:30:00Z",
  "database": {
    "path": "./glog.db",
    "size": 45056,
    "status": "ok"
  },
  "sse": {
    "clients": 3
  }
}
```

### Log Monitoring

```bash
# Systemd journal
sudo journalctl -u glog -f

# Docker logs
docker logs -f glog
```

### Metrics to Monitor

- **Database size**: `ls -lh glog.db`
- **SSE connections**: `/health?detailed=true`
- **Response time**: curl response times
- **Disk usage**: `df -h /var/lib/glog`

## Backup and recovery

### Backup

```bash
# Simple backup (SQLite supports online backups)
cp glog.db glog.db.backup.$(date +%Y%m%d)

# Or with compression
sqlite3 glog.db ".backup glog.db.backup"
gzip glog.db.backup
```

### Automated backup

```bash
#!/bin/bash
# /opt/glog/scripts/backup.sh

BACKUP_DIR="/var/backups/glog"
DB_PATH="/var/lib/glog/glog.db"
RETENTION_DAYS=30

mkdir -p "$BACKUP_DIR"

# Create backup
sqlite3 "$DB_PATH" ".backup $BACKUP_DIR/glog.db.$(date +%Y%m%d%H%M%S)"

# Compress
gzip "$BACKUP_DIR"/glog.db.*

# Clean old backups
find "$BACKUP_DIR" -name "glog.db.*.gz" -mtime +$RETENTION_DAYS -delete
```

Add to crontab:

```bash
# Daily backup at 2 AM
0 2 * * * /opt/glog/scripts/backup.sh
```

### Recovery

```bash
# Stop service
sudo systemctl stop glog

# Restore from backup
gunzip glog.db.20260118.gz
cp glog.db.20260118 /var/lib/glog/glog.db

# Start service
sudo systemctl start glog
```

## Scaling

### A future PostgreSQL backend

> **Not currently implemented.** GLog ships with SQLite as its only backend.
> A PostgreSQL backend is a possible future migration path, not a supported
> deployment option today.

A future PostgreSQL backend would help if you experience:

- >100 hosts sending logs
- >1000 logs/second sustained
- Frequent "database locked" errors
- Need for multi-region deployment

See [Database design](../architecture/database-design.md#migration-to-postgresql) for the forward-looking migration design.

### Horizontal scaling

GLog is designed as a single-node application and currently runs against a
single SQLite database. Horizontal scaling is **not supported today**; it would
require the future PostgreSQL backend described above. A possible future design:

1. **A shared SQL backend** (e.g. the future PostgreSQL repository): would allow multiple GLog instances
2. **Load balancer**: distribute read requests
3. **Write coordination**: single writer or partition by host_id

## Troubleshooting

### Service fails to start

```bash
# Check logs
sudo journalctl -u glog -n 50

# Verify permissions
ls -la /var/lib/glog/glog.db

# Test binary manually
sudo -u glog /opt/glog/bin/glog serve --db /var/lib/glog/glog.db
```

### Database locked errors

```bash
# Check WAL mode
sqlite3 /var/lib/glog/glog.db "PRAGMA journal_mode;"

# Check for other processes
lsof /var/lib/glog/glog.db
```

### High memory usage

- **Cause**: Large cache or many SSE connections
- **Fix**: Reduce cache size, add connection limits
- **Monitor**: `docker stats` or `systemctl show glog`

## Related docs

- [SSE events API](../api/events.md)
- [Go integration](../go-integration.md)
- [Architecture](../architecture/index.md)

---
