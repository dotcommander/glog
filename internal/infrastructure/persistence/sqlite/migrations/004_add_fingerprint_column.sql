-- +goose Up
ALTER TABLE logs ADD COLUMN fingerprint TEXT;
CREATE INDEX idx_logs_fingerprint ON logs(fingerprint);
CREATE INDEX idx_logs_fingerprint_timestamp ON logs(fingerprint, timestamp DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_logs_fingerprint_timestamp;
DROP INDEX IF EXISTS idx_logs_fingerprint;
