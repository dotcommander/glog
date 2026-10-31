package http

import (
	"fmt"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
)

func TestLogSnapshotPagingIgnoresNewInserts(t *testing.T) {
	t.Parallel()
	router, hostRepo, logRepo := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "snapshot")
	when := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		if err := logRepo.Create(&entities.Log{HostID: host.ID, Level: entities.LogLevelInfo,
			Message: fmt.Sprintf("log %d", i), Fields: entities.JSONMap{}, Timestamp: when, CreatedAt: when}); err != nil {
			t.Fatalf("create log: %v", err)
		}
	}
	first := requestJSON(t, router, stdhttp.MethodGet, "/api/v1/logs?limit=2&snapshot_id=0", nil, stdhttp.StatusOK)
	snapshot := first["snapshot_id"].(string)
	if got := int(first["total"].(float64)); got != 3 {
		t.Fatalf("snapshot total = %d, want 3", got)
	}
	if err := logRepo.Create(&entities.Log{HostID: host.ID, Level: entities.LogLevelInfo,
		Message: "newer timestamp", Fields: entities.JSONMap{}, Timestamp: when.Add(time.Minute), CreatedAt: when}); err != nil {
		t.Fatalf("create newer log: %v", err)
	}
	second := requestJSON(t, router, stdhttp.MethodGet,
		fmt.Sprintf("/api/v1/logs?limit=2&offset=2&snapshot_id=%s", snapshot), nil, stdhttp.StatusOK)
	if got := int(second["total"].(float64)); got != 3 {
		t.Fatalf("next page total = %d, want 3", got)
	}
	rows := second["logs"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["message"] != "log 0" {
		t.Fatalf("snapshot page = %v, want one old log", rows)
	}
	legacy := requestJSON(t, router, stdhttp.MethodGet, "/api/v1/logs?limit=2", nil, stdhttp.StatusOK)
	if got := int(legacy["total"].(float64)); got != 4 {
		t.Fatalf("legacy total = %d, want 4", got)
	}
	requestStatus(t, router, stdhttp.MethodGet, "/api/v1/logs?snapshot_id=-1", nil, stdhttp.StatusBadRequest)
}
