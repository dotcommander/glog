-- +goose Up
-- Create hosts table for storing registered hosts that can send logs
CREATE TABLE IF NOT EXISTS hosts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE CHECK(length(name) > 0 AND length(name) <= 255),
    api_key TEXT NOT NULL UNIQUE CHECK(length(api_key) = 54),
    tags TEXT CHECK(json_valid(tags)),  -- JSON array of strings
    status TEXT NOT NULL DEFAULT 'unknown' CHECK(status IN ('unknown', 'online', 'offline', 'degraded')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_log_id INTEGER,
    log_count INTEGER NOT NULL DEFAULT 0 CHECK(log_count >= 0),
    error_count INTEGER NOT NULL DEFAULT 0 CHECK(error_count >= 0),
    error_rate REAL NOT NULL DEFAULT 0.0 CHECK(error_rate >= 0.0 AND error_rate <= 1.0),
    description TEXT CHECK(length(description) <= 1000),
    hostname TEXT CHECK(length(hostname) <= 255),
    ip TEXT CHECK(length(ip) <= 45),  -- IPv6 max length
    user_agent TEXT CHECK(length(user_agent) <= 500),
    metadata TEXT DEFAULT '{}' CHECK(json_valid(metadata))
);

-- Indexes for common queries
CREATE INDEX idx_hosts_api_key ON hosts(api_key);
CREATE INDEX idx_hosts_name ON hosts(name);
CREATE INDEX idx_hosts_status ON hosts(status);
CREATE INDEX idx_hosts_last_seen ON hosts(last_seen DESC);
CREATE INDEX idx_hosts_created_at ON hosts(created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_hosts_created_at;
DROP INDEX IF EXISTS idx_hosts_last_seen;
DROP INDEX IF EXISTS idx_hosts_status;
DROP INDEX IF EXISTS idx_hosts_name;
DROP INDEX IF EXISTS idx_hosts_api_key;
DROP TABLE IF EXISTS hosts;
