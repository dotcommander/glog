-- +goose Up
-- Hourly summary tables for dashboard and operator aggregate queries.
CREATE TABLE IF NOT EXISTS log_buckets (
    bucket_start DATETIME NOT NULL,
    bucket_width TEXT NOT NULL,
    host_id INTEGER NOT NULL,
    level TEXT NOT NULL CHECK(level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    count INTEGER NOT NULL DEFAULT 0 CHECK(count >= 0),
    PRIMARY KEY (bucket_start, bucket_width, host_id, level),
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS fingerprint_buckets (
    bucket_start DATETIME NOT NULL,
    bucket_width TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    level TEXT NOT NULL CHECK(level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    count INTEGER NOT NULL DEFAULT 0 CHECK(count >= 0),
    first_seen DATETIME NOT NULL,
    last_seen DATETIME NOT NULL,
    PRIMARY KEY (bucket_start, bucket_width, fingerprint, level)
);

CREATE INDEX idx_log_buckets_range ON log_buckets(bucket_start, bucket_width);
CREATE INDEX idx_log_buckets_host ON log_buckets(host_id, bucket_start);
CREATE INDEX idx_fingerprint_buckets_range ON fingerprint_buckets(bucket_start, bucket_width);
CREATE INDEX idx_fingerprint_buckets_fingerprint ON fingerprint_buckets(fingerprint, bucket_start);

-- +goose Down
DROP INDEX IF EXISTS idx_fingerprint_buckets_fingerprint;
DROP INDEX IF EXISTS idx_fingerprint_buckets_range;
DROP INDEX IF EXISTS idx_log_buckets_host;
DROP INDEX IF EXISTS idx_log_buckets_range;
DROP TABLE IF EXISTS fingerprint_buckets;
DROP TABLE IF EXISTS log_buckets;
