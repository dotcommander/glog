package sqlite

import (
	"fmt"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
)

type logBucketKey struct {
	start  string
	hostID int64
	level  string
}

type fingerprintBucketKey struct {
	start       string
	fingerprint string
	level       string
}

type fingerprintDelta struct {
	count               int64
	firstSeen, lastSeen string
}

// updateSummaryBuckets collapses writes to identical keys within the bounded insert batch.
func updateSummaryBuckets(exec sqlExecer, logs []*entities.Log) error {
	logCounts := make(map[logBucketKey]int64, len(logs))
	fingerprints := make(map[fingerprintBucketKey]fingerprintDelta)
	logKeys := make([]logBucketKey, 0, len(logs))
	fingerprintKeys := make([]fingerprintBucketKey, 0, len(logs))
	for _, log := range logs {
		start := log.Timestamp.UTC().Truncate(time.Hour).Format(time.RFC3339)
		key := logBucketKey{start: start, hostID: log.HostID, level: string(log.Level)}
		if logCounts[key] == 0 {
			logKeys = append(logKeys, key)
		}
		logCounts[key]++
		if log.Fingerprint == "" {
			continue
		}
		fingerprintKey := fingerprintBucketKey{start: start, fingerprint: log.Fingerprint, level: string(log.Level)}
		ts := log.Timestamp.UTC().Format(time.RFC3339)
		delta, ok := fingerprints[fingerprintKey]
		if !ok {
			fingerprintKeys = append(fingerprintKeys, fingerprintKey)
			delta.firstSeen = ts
			delta.lastSeen = ts
		}
		delta.count++
		if ts < delta.firstSeen {
			delta.firstSeen = ts
		}
		if ts > delta.lastSeen {
			delta.lastSeen = ts
		}
		fingerprints[fingerprintKey] = delta
	}

	for _, key := range logKeys {
		if _, err := exec.Exec(`
			INSERT INTO log_buckets (bucket_start, bucket_width, host_id, level, count)
			VALUES (?, '1h', ?, ?, ?)
			ON CONFLICT(bucket_start, bucket_width, host_id, level)
			DO UPDATE SET count = count + excluded.count
		`, key.start, key.hostID, key.level, logCounts[key]); err != nil {
			return fmt.Errorf("failed to update log bucket: %w", err)
		}
	}
	for _, key := range fingerprintKeys {
		delta := fingerprints[key]
		if _, err := exec.Exec(`
			INSERT INTO fingerprint_buckets (
				bucket_start, bucket_width, fingerprint, level, count, first_seen, last_seen
			) VALUES (?, '1h', ?, ?, ?, ?, ?)
			ON CONFLICT(bucket_start, bucket_width, fingerprint, level)
			DO UPDATE SET
				count = count + excluded.count,
				first_seen = CASE WHEN excluded.first_seen < first_seen THEN excluded.first_seen ELSE first_seen END,
				last_seen = CASE WHEN excluded.last_seen > last_seen THEN excluded.last_seen ELSE last_seen END
		`, key.start, key.fingerprint, key.level, delta.count, delta.firstSeen, delta.lastSeen); err != nil {
			return fmt.Errorf("failed to update fingerprint bucket: %w", err)
		}
	}
	return nil
}
