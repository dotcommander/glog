package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
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

func TestLogListRoutesRejectInvalidLevelFilter(t *testing.T) {
	t.Parallel()

	router, _, _ := newLogRouteTestDeps(t)
	for _, path := range []string{
		"/api/v1/logs?level=nope",
		"/api/v1/logs/groups?level=nope",
	} {
		rec := requestStatus(t, router, stdhttp.MethodGet, path, nil, stdhttp.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Invalid level. Must be one of: trace, debug, info, warn, error, fatal") {
			t.Fatalf("GET %s response = %q; want invalid level error", path, rec.Body.String())
		}
	}
}

func TestLogListRoutesNormalizePagination(t *testing.T) {
	t.Parallel()

	router, _, _ := newLogRouteTestDeps(t)
	for _, route := range []string{"/api/v1/logs", "/api/v1/logs/groups"} {
		for _, tt := range []struct {
			name       string
			query      string
			wantLimit  int
			wantOffset int
		}{
			{name: "defaults", wantLimit: constants.DefaultLimit, wantOffset: 0},
			{name: "caps limit", query: "?limit=1001&offset=7", wantLimit: constants.MaxLimit, wantOffset: 7},
		} {
			t.Run(route+"/"+tt.name, func(t *testing.T) {
				response := requestJSON(t, router, stdhttp.MethodGet, route+tt.query, nil, stdhttp.StatusOK)
				if got := int(response["limit"].(float64)); got != tt.wantLimit {
					t.Fatalf("GET %s limit = %d; want %d", route+tt.query, got, tt.wantLimit)
				}
				if got := int(response["offset"].(float64)); got != tt.wantOffset {
					t.Fatalf("GET %s offset = %d; want %d", route+tt.query, got, tt.wantOffset)
				}
			})
		}
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

func TestHostDetailRoutesReturnHostAndStats(t *testing.T) {
	t.Parallel()

	router, hostRepo, logRepo := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "detail")
	now := time.Now().UTC()
	if err := logRepo.Create(&entities.Log{
		HostID: host.ID, Level: entities.LogLevelError, Message: "failed",
		Fields: entities.JSONMap{}, Timestamp: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for _, tt := range []struct {
		name  string
		path  string
		field string
		want  any
	}{
		{name: "details", path: fmt.Sprintf("/api/v1/hosts/%d", host.ID), field: "id", want: float64(host.ID)},
		{name: "stats", path: fmt.Sprintf("/api/v1/hosts/%d/stats", host.ID), field: "total_logs", want: float64(1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := requestJSON(t, router, stdhttp.MethodGet, tt.path, nil, stdhttp.StatusOK)
			if got := response[tt.field]; got != tt.want {
				t.Fatalf("GET %s %s = %v; want %v", tt.path, tt.field, got, tt.want)
			}
		})
	}
}

func TestHostDetailRoutesRejectMalformedAndMissingIDs(t *testing.T) {
	t.Parallel()

	router, _, _ := newLogRouteTestDeps(t)
	for _, tt := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "malformed details ID", path: "/api/v1/hosts/nope", wantStatus: stdhttp.StatusBadRequest, wantBody: "Invalid host ID"},
		{name: "malformed stats ID", path: "/api/v1/hosts/nope/stats", wantStatus: stdhttp.StatusBadRequest, wantBody: "Invalid host ID"},
		{name: "missing details ID", path: "/api/v1/hosts/999999", wantStatus: stdhttp.StatusNotFound, wantBody: "Host not found"},
		{name: "missing stats ID", path: "/api/v1/hosts/999999/stats", wantStatus: stdhttp.StatusNotFound, wantBody: "Host not found"},
		{name: "zero details ID", path: "/api/v1/hosts/0", wantStatus: stdhttp.StatusNotFound, wantBody: "Host not found"},
		{name: "negative stats ID", path: "/api/v1/hosts/-1/stats", wantStatus: stdhttp.StatusNotFound, wantBody: "Host not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := requestStatus(t, router, stdhttp.MethodGet, tt.path, nil, tt.wantStatus)
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("GET %s body = %q; want %q", tt.path, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRegisterHostRejectsUnknownJSONFields(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/hosts", bytes.NewBufferString(
		`{"name":"api","unexpected":true}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("POST /api/v1/hosts status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("unknown field")) {
		t.Fatalf("body = %q; want unknown field error", rec.Body.String())
	}
	assertNoRegisteredHosts(t, hostRepo)
}

func TestRegisterHostRejectsOversizedRequestBody(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	body := `{"name":"` + strings.Repeat("x", constants.MaxRequestBodyBytes) + `"}`
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/hosts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("POST /api/v1/hosts status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("request body too large")) {
		t.Fatalf("body = %q; want request body too large error", rec.Body.String())
	}
	assertNoRegisteredHosts(t, hostRepo)
}

func TestRegisterHostRejectsOversizedTrailingData(t *testing.T) {
	t.Parallel()

	router, hostRepo, _ := newLogRouteTestDeps(t)
	body := `{"name":"api"}` + strings.Repeat(" ", constants.MaxRequestBodyBytes)
	req := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/hosts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("POST /api/v1/hosts status = %d; want %d; body = %q", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("request body too large")) {
		t.Fatalf("body = %q; want request body too large error", rec.Body.String())
	}
	assertNoRegisteredHosts(t, hostRepo)
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

func TestStatsRoutesReturnEmptyBuckets(t *testing.T) {
	t.Parallel()

	router, _, _ := newLogRouteTestDeps(t)
	for _, path := range []string{
		"/api/v1/stats/logs",
		"/api/v1/stats/fingerprints",
	} {
		response := requestJSON(t, router, stdhttp.MethodGet, path, nil, stdhttp.StatusOK)
		buckets, ok := response["buckets"].([]any)
		if !ok || len(buckets) != 0 {
			t.Fatalf("GET %s buckets = %#v; want empty array", path, response["buckets"])
		}
		if total := int(response["total"].(float64)); total != 0 {
			t.Fatalf("GET %s total = %d; want 0", path, total)
		}
	}
}

func TestStatsRoutesRejectInvalidFilters(t *testing.T) {
	t.Parallel()

	router, _, _ := newLogRouteTestDeps(t)
	for _, path := range []string{
		"/api/v1/stats/logs?level=nope",
		"/api/v1/stats/fingerprints?level=nope",
	} {
		rec := requestStatus(t, router, stdhttp.MethodGet, path, nil, stdhttp.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Invalid level. Must be one of: trace, debug, info, warn, error, fatal") {
			t.Fatalf("GET %s response = %q; want invalid level error", path, rec.Body.String())
		}
	}
}

func TestStatsRoutesReturnPopulatedBuckets(t *testing.T) {
	t.Parallel()

	router, hostRepo, logRepo := newLogRouteTestDeps(t)
	host := createHTTPTestHost(t, hostRepo, "stats")
	now := time.Now().UTC()
	if err := logRepo.Create(&entities.Log{
		HostID: host.ID, Level: entities.LogLevelError, Message: "failed",
		Fields: entities.JSONMap{}, Timestamp: now, CreatedAt: now, Fingerprint: "failure",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for _, path := range []string{
		"/api/v1/stats/logs",
		"/api/v1/stats/fingerprints",
	} {
		response := requestJSON(t, router, stdhttp.MethodGet, path, nil, stdhttp.StatusOK)
		buckets, ok := response["buckets"].([]any)
		if !ok || len(buckets) != 1 {
			t.Fatalf("GET %s buckets = %#v; want one bucket", path, response["buckets"])
		}
		if total := int(response["total"].(float64)); total != 1 {
			t.Fatalf("GET %s total = %d; want 1", path, total)
		}
	}
}

type failingStatsLogRepository struct {
	ports.LogRepository
	err error
}

func (r failingStatsLogRepository) FindLogBuckets(ports.LogFilters) ([]ports.LogBucket, error) {
	return nil, r.err
}

func (r failingStatsLogRepository) FindFingerprintBuckets(ports.LogFilters) ([]ports.FingerprintBucket, error) {
	return nil, r.err
}

func TestStatsRoutesPreserveQueryErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("query unavailable")
	h := handlers.NewHandlers(nil, failingStatsLogRepository{err: wantErr}, sse.NewHub())
	for _, tt := range []struct {
		path    string
		handler stdhttp.Handler
		prefix  string
	}{
		{path: "/api/v1/stats/logs", handler: handlers.LogBucketsHandler(h), prefix: "Failed to fetch log stats: "},
		{path: "/api/v1/stats/fingerprints", handler: handlers.FingerprintBucketsHandler(h), prefix: "Failed to fetch fingerprint stats: "},
	} {
		req := httptest.NewRequest(stdhttp.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		tt.handler.ServeHTTP(rec, req)
		if rec.Code != stdhttp.StatusInternalServerError {
			t.Fatalf("GET %s status = %d; want %d", tt.path, rec.Code, stdhttp.StatusInternalServerError)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != tt.prefix+wantErr.Error() {
			t.Fatalf("GET %s body = %q; want %q", tt.path, got, tt.prefix+wantErr.Error())
		}
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

func assertNoRegisteredHosts(t *testing.T, repo *sqlite.HostRepository) {
	t.Helper()
	hosts, err := repo.List(false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %d; want 0", len(hosts))
	}
}
