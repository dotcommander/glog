package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

func TestAuthenticatedActivityUpdatesFreshnessOnce(t *testing.T) {
	t.Parallel()
	db, err := sqlite.New(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
		t.Fatalf("MigrateFS: %v", err)
	}
	hostRepo := sqlite.NewHostRepository(db)
	logRepo := sqlite.NewLogRepository(db)
	host := createHTTPTestHost(t, hostRepo, "freshness")
	if _, err := db.Conn().Exec(`CREATE TABLE freshness_writes (id INTEGER PRIMARY KEY);`); err != nil {
		t.Fatalf("create audit table: %v", err)
	}
	if _, err := db.Conn().Exec(`CREATE TRIGGER audit_freshness AFTER UPDATE OF last_seen ON hosts
		BEGIN INSERT INTO freshness_writes (id) VALUES (NULL); END`); err != nil {
		t.Fatalf("create audit trigger: %v", err)
	}
	hub := sse.NewHub()
	t.Cleanup(hub.Close)
	router := NewRouter(&RouterDeps{DB: db, HostRepo: hostRepo, LogRepo: logRepo,
		Hub: hub, Handlers: handlers.NewHandlers(hostRepo, logRepo, hub)}, &Config{})

	requests := []struct {
		method, path string
		payload      any
		wantStatus   int
	}{
		{http.MethodPost, "/api/v1/logs", map[string]any{"message": "single"}, http.StatusCreated},
		{http.MethodPost, "/api/v1/logs/bulk", map[string]any{"logs": []map[string]any{{"message": "one"}, {"message": "two"}}}, http.StatusCreated},
		{http.MethodPost, "/api/v1/logs", map[string]any{"message": ""}, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/export/ndjson", nil, http.StatusOK},
	}
	for index, tc := range requests {
		var body []byte
		if tc.payload != nil {
			body, err = json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
		}
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+host.APIKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.wantStatus {
			t.Fatalf("%s %s: status %d, want %d: %s", tc.method, tc.path, rec.Code, tc.wantStatus, rec.Body.String())
		}
		var writes int
		if err := db.Conn().QueryRow("SELECT COUNT(*) FROM freshness_writes").Scan(&writes); err != nil {
			t.Fatalf("audit count: %v", err)
		}
		if writes != index+1 {
			t.Fatalf("after %s %s: freshness writes = %d, want %d", tc.method, tc.path, writes, index+1)
		}
	}
}
