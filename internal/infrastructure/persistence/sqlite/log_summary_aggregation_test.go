package sqlite

import (
	"testing"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
)

func TestBulkSummaryAggregationPreservesKeysAndExtrema(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	host := createTestHost(t, NewHostRepository(db), "summary")
	repo := NewLogRepository(db)
	base := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	logs := []*entities.Log{}
	for _, minutes := range []int{40, 5, 25} {
		logs = append(logs, &entities.Log{HostID: host.ID, Level: entities.LogLevelError,
			Message: "error", Fields: entities.JSONMap{}, Timestamp: base.Add(time.Duration(minutes) * time.Minute),
			CreatedAt: base, Fingerprint: "shared"})
	}
	logs = append(logs, &entities.Log{HostID: host.ID, Level: entities.LogLevelInfo,
		Message: "info", Fields: entities.JSONMap{}, Timestamp: base, CreatedAt: base, Fingerprint: "shared"})
	if _, err := repo.BulkCreate(logs); err != nil {
		t.Fatalf("BulkCreate: %v", err)
	}
	var count int64
	if err := db.Conn().QueryRow("SELECT count FROM log_buckets WHERE host_id = ? AND level = 'error'", host.ID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("error log bucket count = %d, err = %v", count, err)
	}
	var first, last string
	if err := db.Conn().QueryRow("SELECT count, first_seen, last_seen FROM fingerprint_buckets WHERE fingerprint = 'shared' AND level = 'error'").Scan(&count, &first, &last); err != nil {
		t.Fatalf("fingerprint bucket: %v", err)
	}
	if count != 3 || first != base.Add(5*time.Minute).Format(time.RFC3339) || last != base.Add(40*time.Minute).Format(time.RFC3339) {
		t.Fatalf("fingerprint bucket = count %d, first %q, last %q", count, first, last)
	}
	// Existing rows must also merge with a later batch rather than being replaced.
	if _, err := repo.BulkCreate([]*entities.Log{{HostID: host.ID, Level: entities.LogLevelError,
		Message: "later batch", Fields: entities.JSONMap{}, Timestamp: base.Add(2 * time.Minute), CreatedAt: base, Fingerprint: "shared"}}); err != nil {
		t.Fatalf("second BulkCreate: %v", err)
	}
	if err := db.Conn().QueryRow("SELECT count, first_seen, last_seen FROM fingerprint_buckets WHERE fingerprint = 'shared' AND level = 'error'").Scan(&count, &first, &last); err != nil {
		t.Fatalf("merged fingerprint bucket: %v", err)
	}
	if count != 4 || first != base.Add(2*time.Minute).Format(time.RFC3339) || last != base.Add(40*time.Minute).Format(time.RFC3339) {
		t.Fatalf("merged fingerprint bucket = count %d, first %q, last %q", count, first, last)
	}
}
