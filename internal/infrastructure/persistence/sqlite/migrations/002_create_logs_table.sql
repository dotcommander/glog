-- +goose Up
-- Create logs table for storing log entries from hosts
CREATE TABLE IF NOT EXISTS logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id INTEGER NOT NULL,
    level TEXT NOT NULL DEFAULT 'info' CHECK(level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    message TEXT NOT NULL CHECK(length(message) > 0 AND length(message) <= 10000),
    fields TEXT DEFAULT '{}' CHECK(json_valid(fields)),  -- JSON object
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- HTTP request context (optional) - allow both NULL and 0 for "not set"
    http_method TEXT CHECK(http_method IS NULL OR http_method = '' OR http_method IN ('GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'HEAD', 'OPTIONS', 'CONNECT', 'TRACE')),
    http_url TEXT CHECK(http_url IS NULL OR http_url = '' OR length(http_url) <= 2048),
    http_status INTEGER CHECK(http_status IS NULL OR http_status = 0 OR (http_status >= 100 AND http_status <= 599)),
    http_response_time_ms INTEGER CHECK(http_response_time_ms IS NULL OR http_response_time_ms = 0 OR http_response_time_ms >= 0),

    -- Derived metadata (from pattern matching)
    derived_level TEXT CHECK(derived_level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    derived_source TEXT CHECK(length(derived_source) <= 255),
    derived_category TEXT CHECK(length(derived_category) <= 255),

    -- Foreign key constraint
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX idx_logs_host_id ON logs(host_id);
CREATE INDEX idx_logs_level ON logs(level);
CREATE INDEX idx_logs_timestamp ON logs(timestamp DESC);
CREATE INDEX idx_logs_created_at ON logs(created_at DESC);

-- Composite indexes for common queries
CREATE INDEX idx_logs_host_timestamp ON logs(host_id, timestamp DESC);
CREATE INDEX idx_logs_level_timestamp ON logs(level, timestamp DESC);

-- Partial index for error logs (most common query)
CREATE INDEX idx_logs_errors ON logs(host_id, timestamp) WHERE level IN ('error', 'fatal');

-- Index for HTTP logs
CREATE INDEX idx_logs_http ON logs(http_method, http_url, http_status) WHERE http_method IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_logs_http;
DROP INDEX IF EXISTS idx_logs_errors;
DROP INDEX IF EXISTS idx_logs_level_timestamp;
DROP INDEX IF EXISTS idx_logs_host_timestamp;
DROP INDEX IF EXISTS idx_logs_created_at;
DROP INDEX IF EXISTS idx_logs_timestamp;
DROP INDEX IF EXISTS idx_logs_level;
DROP INDEX IF EXISTS idx_logs_host_id;
DROP TABLE IF EXISTS logs;
