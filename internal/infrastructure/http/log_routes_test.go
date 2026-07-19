package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

func TestGroupedLogsRouteReturnsAggregateTimes(t *testing.T) {
	t.Parallel()

	router, hostRepo, logRepo := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "api")
	firstSeen := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)
	lastSeen := firstSeen.Add(time.Minute)
	logs := []*entities.Log{
		{HostID: host.ID, Level: entities.LogLevelDebug, Message: "LLM smoke baseline", Fields: entities.JSONMap{}, Timestamp: firstSeen, CreatedAt: firstSeen, Fingerprint: "llm-smoke"},
		{HostID: host.ID, Level: entities.LogLevelError, Message: "LLM smoke failure", Fields: entities.JSONMap{}, Timestamp: lastSeen, CreatedAt: lastSeen, Fingerprint: "llm-smoke"},
	}
	if _, err := logRepo.BulkCreate(logs); err != nil {
		t.Fatalf("BulkCreate() error = %v", err)
	}

	resp := requestJSON(t, router, stdhttp.MethodGet, "/api/v1/logs/groups?search=LLM%20smoke&limit=10", nil, stdhttp.StatusOK)
	if got := int(resp["total"].(float64)); got != 1 {
		t.Fatalf("total = %d; want 1", got)
	}
	groups := resp["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("groups len = %d; want 1", len(groups))
	}
	group := groups[0].(map[string]any)
	if got := group["fingerprint"]; got != "llm-smoke" {
		t.Fatalf("fingerprint = %v; want llm-smoke", got)
	}
	if got := int(group["count"].(float64)); got != 2 {
		t.Fatalf("count = %d; want 2", got)
	}
	if group["first_seen"] == "" || group["last_seen"] == "" {
		t.Fatalf("missing aggregate timestamps: %+v", group)
	}
}

func TestHostsRouteReturnsErrorRatePercent(t *testing.T) {
	t.Parallel()

	router, hostRepo, logRepo := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "api")
	now := time.Now()
	logs := []*entities.Log{
		{HostID: host.ID, Level: entities.LogLevelInfo, Message: "ok", Fields: entities.JSONMap{}, Timestamp: now, CreatedAt: now},
		{HostID: host.ID, Level: entities.LogLevelError, Message: "failed", Fields: entities.JSONMap{}, Timestamp: now, CreatedAt: now},
	}
	if _, err := logRepo.BulkCreate(logs); err != nil {
		t.Fatalf("BulkCreate() error = %v", err)
	}

	resp := requestJSON(t, router, stdhttp.MethodGet, "/api/v1/hosts", nil, stdhttp.StatusOK)
	hosts := resp["hosts"].([]any)
	if len(hosts) != 1 {
		t.Fatalf("hosts len = %d; want 1", len(hosts))
	}
	got := hosts[0].(map[string]any)["error_rate"].(float64)
	if got != 50 {
		t.Fatalf("error_rate = %v; want 50", got)
	}
}

func TestLogRoutesRejectUnknownJSONFields(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "api")

	body := bytes.NewBufferString(`{"level":"info","message":"hello","unexpected":true}`)
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/logs", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+host.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("POST /api/v1/logs status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("unknown field")) {
		t.Fatalf("body = %q; want unknown field error", rec.Body.String())
	}
}

func TestExportRejectsInvalidLevelFilter(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "api")

	req := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/export/json?level=nope", nil)
	req.Header.Set("Authorization", "Bearer "+host.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("GET /api/v1/export/json status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
}

func TestBulkLogRejectsOversizedRequestBody(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "api")
	payload, err := json.Marshal(map[string]any{
		"logs": []map[string]any{{
			"level":   "info",
			"message": strings.Repeat("x", 12*1024*1024),
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/logs/bulk", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+host.APIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("POST /api/v1/logs/bulk status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
}

func newLogRouteTestDeps(t *testing.T) (stdhttp.Handler, *sqlite.HostRepository, *sqlite.LogRepository) {
	t.Helper()

	db, err := sqlite.New(filepath.Join(t.TempDir(), "glog.db"))
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close() error = %v", err)
		}
	})
	if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
		t.Fatalf("MigrateFS() error = %v", err)
	}

	hostRepo := sqlite.NewHostRepository(db)
	logRepo := sqlite.NewLogRepository(db)
	hub := sse.NewHub()
	h := handlers.NewHandlers(hostRepo, logRepo, hub)

	router := NewRouter(&RouterDeps{
		DB:       db,
		HostRepo: hostRepo,
		LogRepo:  logRepo,
		Hub:      hub,
		Handlers: h,
	}, &Config{})
	return router, hostRepo, logRepo
}

func createHTTPTestHost(t *testing.T, repo *sqlite.HostRepository, name string) *entities.Host {
	t.Helper()

	host, err := entities.NewHost(name, nil)
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	if err := repo.Create(host); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return host
}
