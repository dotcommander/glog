package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
)

func newTestDB(t *testing.T) *Database {
	t.Helper()

	db, err := New(filepath.Join(t.TempDir(), "glog.db"))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := db.MigrateFS(EmbeddedMigrations, "migrations"); err != nil {
		t.Fatalf("MigrateFS failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func createTestHost(t *testing.T, repo *HostRepository, name string) *entities.Host {
	t.Helper()

	host, err := entities.NewHost(name, nil)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	if err := repo.Create(host); err != nil {
		t.Fatalf("Create host failed: %v", err)
	}
	return host
}

func TestLogRepositoryCreateUpdatesHourlyBuckets(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	hostRepo := NewHostRepository(db)
	logRepo := NewLogRepository(db)
	host := createTestHost(t, hostRepo, "api")
	ts := time.Date(2026, 5, 15, 12, 34, 0, 0, time.UTC)

	log := &entities.Log{
		HostID:      host.ID,
		Level:       entities.LogLevelError,
		Message:     "payment failed for order 123",
		Fields:      entities.JSONMap{},
		Timestamp:   ts,
		CreatedAt:   ts,
		Fingerprint: "payment failed for order <number>",
	}
	if err := logRepo.Create(log); err != nil {
		t.Fatalf("Create log failed: %v", err)
	}

	from := ts.Truncate(time.Hour).Format(time.RFC3339)
	to := ts.Truncate(time.Hour).Add(time.Hour).Format(time.RFC3339)
	buckets, err := logRepo.FindLogBuckets(LogFilters{FromDate: from, ToDate: to})
	if err != nil {
		t.Fatalf("FindLogBuckets failed: %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("bucket count mismatch: got %d want 1", len(buckets))
	}
	if buckets[0].HostID != host.ID || buckets[0].Level != "error" || buckets[0].Count != 1 {
		t.Fatalf("unexpected log bucket: %+v", buckets[0])
	}

	fpBuckets, err := logRepo.FindFingerprintBuckets(LogFilters{FromDate: from, ToDate: to})
	if err != nil {
		t.Fatalf("FindFingerprintBuckets failed: %v", err)
	}
	if len(fpBuckets) != 1 {
		t.Fatalf("fingerprint bucket count mismatch: got %d want 1", len(fpBuckets))
	}
	if fpBuckets[0].Fingerprint != log.Fingerprint || fpBuckets[0].Count != 1 {
		t.Fatalf("unexpected fingerprint bucket: %+v", fpBuckets[0])
	}

	updatedHost, err := hostRepo.FindByID(host.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if updatedHost.LogCount != 1 || updatedHost.ErrorCount != 1 || updatedHost.ErrorRate != 1 {
		t.Fatalf("unexpected host counters: logs=%d errors=%d rate=%v", updatedHost.LogCount, updatedHost.ErrorCount, updatedHost.ErrorRate)
	}
}

func TestLogRepositoryBulkCreateAggregatesHourlyBuckets(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	hostRepo := NewHostRepository(db)
	logRepo := NewLogRepository(db)
	host := createTestHost(t, hostRepo, "worker")
	base := time.Date(2026, 5, 15, 9, 5, 0, 0, time.UTC)

	logs := []*entities.Log{
		{HostID: host.ID, Level: entities.LogLevelInfo, Message: "ok one", Fields: entities.JSONMap{}, Timestamp: base, CreatedAt: base},
		{HostID: host.ID, Level: entities.LogLevelInfo, Message: "ok two", Fields: entities.JSONMap{}, Timestamp: base.Add(10 * time.Minute), CreatedAt: base},
		{HostID: host.ID, Level: entities.LogLevelWarn, Message: "slow", Fields: entities.JSONMap{}, Timestamp: base.Add(20 * time.Minute), CreatedAt: base},
		{HostID: host.ID, Level: entities.LogLevelFatal, Message: "failed", Fields: entities.JSONMap{}, Timestamp: base.Add(30 * time.Minute), CreatedAt: base},
	}
	if _, err := logRepo.BulkCreate(logs); err != nil {
		t.Fatalf("BulkCreate failed: %v", err)
	}

	buckets, err := logRepo.FindLogBuckets(LogFilters{FromDate: base.Truncate(time.Hour).Format(time.RFC3339)})
	if err != nil {
		t.Fatalf("FindLogBuckets failed: %v", err)
	}
	if len(buckets) != 3 {
		t.Fatalf("bucket count mismatch: got %d want 3", len(buckets))
	}

	counts := map[string]int64{}
	for _, bucket := range buckets {
		counts[bucket.Level] = bucket.Count
	}
	if counts["info"] != 2 || counts["warn"] != 1 || counts["fatal"] != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}

	updatedHost, err := hostRepo.FindByID(host.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if updatedHost.LogCount != 4 || updatedHost.ErrorCount != 1 || updatedHost.ErrorRate != 0.25 {
		t.Fatalf("unexpected host counters: logs=%d errors=%d rate=%v", updatedHost.LogCount, updatedHost.ErrorCount, updatedHost.ErrorRate)
	}
}

func TestLogRepositoryRollsBackWhenCounterUpdateFails(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 15, 12, 34, 0, 0, time.UTC)
	tests := []struct {
		name  string
		write func(*LogRepository, int64) error
	}{
		{
			name: "create",
			write: func(repo *LogRepository, hostID int64) error {
				return repo.Create(&entities.Log{
					HostID: hostID, Level: entities.LogLevelError, Message: "failed",
					Fields: entities.JSONMap{}, Timestamp: ts, CreatedAt: ts, Fingerprint: "failure",
				})
			},
		},
		{
			name: "bulk create",
			write: func(repo *LogRepository, hostID int64) error {
				_, err := repo.BulkCreate([]*entities.Log{
					{HostID: hostID, Level: entities.LogLevelInfo, Message: "first", Fields: entities.JSONMap{}, Timestamp: ts, CreatedAt: ts},
					{HostID: hostID, Level: entities.LogLevelError, Message: "second", Fields: entities.JSONMap{}, Timestamp: ts, CreatedAt: ts, Fingerprint: "failure"},
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t)
			hostRepo := NewHostRepository(db)
			logRepo := NewLogRepository(db)
			host := createTestHost(t, hostRepo, "rollback-"+tt.name)
			if _, err := db.Conn().Exec(`
				CREATE TRIGGER fail_host_counter_update
				BEFORE UPDATE OF log_count ON hosts
				BEGIN
					SELECT RAISE(FAIL, 'counter update failed');
				END
			`); err != nil {
				t.Fatalf("create trigger failed: %v", err)
			}

			if err := tt.write(logRepo, host.ID); err == nil {
				t.Fatal("log write succeeded; want counter update failure")
			}

			for _, table := range []string{"logs", "log_buckets", "fingerprint_buckets"} {
				var count int
				if err := db.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatalf("count %s failed: %v", table, err)
				}
				if count != 0 {
					t.Fatalf("%s count = %d; want 0 after rollback", table, count)
				}
			}

			updatedHost, err := hostRepo.FindByID(host.ID)
			if err != nil {
				t.Fatalf("FindByID failed: %v", err)
			}
			if updatedHost.LogCount != 0 || updatedHost.ErrorCount != 0 || updatedHost.ErrorRate != 0 {
				t.Fatalf("host counters changed after rollback: logs=%d errors=%d rate=%v", updatedHost.LogCount, updatedHost.ErrorCount, updatedHost.ErrorRate)
			}
		})
	}
}

func TestLogRepositoryFindGroupedScansAggregateTimestamps(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	hostRepo := NewHostRepository(db)
	logRepo := NewLogRepository(db)
	host := createTestHost(t, hostRepo, "api")
	location := time.FixedZone("EDT", -4*60*60)
	firstSeen := time.Date(2026, 5, 15, 13, 55, 48, 664259000, location)
	lastSeen := firstSeen.Add(2 * time.Minute)

	logs := []*entities.Log{
		{HostID: host.ID, Level: entities.LogLevelDebug, Message: "LLM smoke: baseline captured", Fields: entities.JSONMap{}, Timestamp: firstSeen, CreatedAt: firstSeen, Fingerprint: "llm-smoke"},
		{HostID: host.ID, Level: entities.LogLevelError, Message: "LLM smoke: simulated failure", Fields: entities.JSONMap{}, Timestamp: lastSeen, CreatedAt: lastSeen, Fingerprint: "llm-smoke"},
	}
	if _, err := logRepo.BulkCreate(logs); err != nil {
		t.Fatalf("BulkCreate failed: %v", err)
	}

	groups, total, err := logRepo.FindGrouped(LogFilters{Limit: 10})
	if err != nil {
		t.Fatalf("FindGrouped failed: %v", err)
	}
	if total != 1 || len(groups) != 1 {
		t.Fatalf("group count mismatch: total=%d len=%d", total, len(groups))
	}
	group := groups[0]
	if group.Fingerprint != "llm-smoke" || group.Count != 2 || group.Level != "error" {
		t.Fatalf("unexpected group: %+v", group)
	}
	if !group.FirstSeen.Equal(firstSeen) || !group.LastSeen.Equal(lastSeen) {
		t.Fatalf("unexpected time range: first=%s want=%s last=%s want=%s", group.FirstSeen, firstSeen, group.LastSeen, lastSeen)
	}
}

func TestHostRepositoryListExcludesInternalByDefault(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	hostRepo := NewHostRepository(db)
	createTestHost(t, hostRepo, "api")
	createTestHost(t, hostRepo, constants.InternalHost)

	hosts, err := hostRepo.List(false)
	if err != nil {
		t.Fatalf("List(false) failed: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "api" {
		t.Fatalf("expected only api host, got %+v", hosts)
	}

	hosts, err = hostRepo.List(true)
	if err != nil {
		t.Fatalf("List(true) failed: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected both hosts, got %d", len(hosts))
	}
}
