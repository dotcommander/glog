-- +goose Up
-- Performance optimizations and additional indexes

-- Covering index for dashboard queries (host logs with pagination)
CREATE INDEX idx_logs_dashboard ON logs(host_id, timestamp DESC, id);

-- Index for recent logs
CREATE INDEX idx_logs_recent ON logs(timestamp DESC, id);

-- Index for search by message content (shorter messages are more commonly searched)
CREATE INDEX idx_logs_message ON logs(message);

-- Composite index for stats queries
CREATE INDEX idx_logs_stats ON logs(host_id, level, timestamp);

-- Enable SQLite WAL mode for better concurrency
PRAGMA journal_mode=WAL;

-- Optimize SQLite settings
PRAGMA synchronous=NORMAL;  -- Balance between safety and performance
PRAGMA cache_size=-64000;   -- 64MB cache (negative means KB)
PRAGMA temp_store=MEMORY;   -- Use memory for temp tables

-- +goose Down
PRAGMA journal_mode=DELETE;  -- Revert to default
PRAGMA synchronous=FULL;
PRAGMA cache_size=0;
PRAGMA temp_store=DEFAULT;

DROP INDEX IF EXISTS idx_logs_stats;
DROP INDEX IF EXISTS idx_logs_message;
DROP INDEX IF EXISTS idx_logs_recent;
DROP INDEX IF EXISTS idx_logs_dashboard;
