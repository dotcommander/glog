package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

func TestInboxRoutesLifecycle(t *testing.T) {
	t.Parallel()

	router := newInboxTestRouter(t)

	created := requestJSON(t, router, http.MethodPost, "/api/v1/inbox", map[string]any{
		"title":      "Investigate flaky checkout",
		"body":       "Look at error fingerprint abc123.",
		"kind":       "bug",
		"priority":   "high",
		"repo":       "dotcommander/glog",
		"session_id": "session-1",
		"tags":       []string{"agent", "checkout"},
	}, http.StatusCreated)

	id := int(created["id"].(float64))
	if got := created["status"]; got != "open" {
		t.Fatalf("created status = %v; want open", got)
	}
	if got := created["kind"]; got != "bug" {
		t.Fatalf("created kind = %v; want bug", got)
	}

	list := requestJSON(t, router, http.MethodGet, "/api/v1/inbox?status=open&repo=dotcommander/glog", nil, http.StatusOK)
	if got := int(list["total"].(float64)); got != 1 {
		t.Fatalf("list total = %d; want 1", got)
	}

	updated := requestJSON(t, router, http.MethodPatch, "/api/v1/inbox/"+itoa(id), map[string]any{
		"status":     "in_progress",
		"claimed_by": "codex",
	}, http.StatusOK)
	if got := updated["status"]; got != "in_progress" {
		t.Fatalf("updated status = %v; want in_progress", got)
	}
	if got := updated["claimed_by"]; got != "codex" {
		t.Fatalf("updated claimed_by = %v; want codex", got)
	}

	done := requestJSON(t, router, http.MethodPost, "/api/v1/inbox/"+itoa(id)+"/done", nil, http.StatusOK)
	if got := done["status"]; got != "done" {
		t.Fatalf("done status = %v; want done", got)
	}

	requestStatus(t, router, http.MethodDelete, "/api/v1/inbox/"+itoa(id), nil, http.StatusNoContent)
	requestStatus(t, router, http.MethodGet, "/api/v1/inbox/"+itoa(id), nil, http.StatusNotFound)
}

func TestInboxRoutesRejectInvalidPayloads(t *testing.T) {
	t.Parallel()

	router := newInboxTestRouter(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{name: "empty title", body: map[string]any{"title": ""}},
		{name: "invalid status", body: map[string]any{"title": "x", "status": "waiting"}},
		{name: "invalid kind", body: map[string]any{"title": "x", "kind": "memo"}},
		{name: "invalid priority", body: map[string]any{"title": "x", "priority": "urgent"}},
		{name: "invalid due_at", body: map[string]any{"title": "x", "due_at": "tomorrow"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			requestStatus(t, router, http.MethodPost, "/api/v1/inbox", tt.body, http.StatusBadRequest)
		})
	}
}

func TestInboxRoutesRejectUnknownJSONFields(t *testing.T) {
	t.Parallel()

	router := newInboxTestRouter(t)

	requestStatus(t, router, http.MethodPost, "/api/v1/inbox", map[string]any{
		"title":      "Investigate drift",
		"unexpected": true,
	}, http.StatusBadRequest)
}

func newInboxTestRouter(t *testing.T) http.Handler {
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
	inboxRepo := sqlite.NewInboxRepository(db)
	hub := sse.NewHub()
	h := handlers.NewHandlers(hostRepo, logRepo, hub)
	h.SetInboxRepo(inboxRepo)

	return NewRouter(&RouterDeps{
		DB:        db,
		HostRepo:  hostRepo,
		LogRepo:   logRepo,
		InboxRepo: inboxRepo,
		Hub:       hub,
		Handlers:  h,
	}, &Config{})
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body map[string]any, wantStatus int) map[string]any {
	t.Helper()

	rec := requestStatus(t, handler, method, path, body, wantStatus)
	var decoded map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&decoded); err != nil {
		t.Fatalf("Decode() error = %v; body = %q", err, rec.Body.String())
	}
	return decoded
}

func requestStatus(t *testing.T, handler http.Handler, method, path string, body map[string]any, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d; want %d; body = %q", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	return rec
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
